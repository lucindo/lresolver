package main

import (
	"container/ring"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/miekg/dns"
	"github.com/spf13/viper"
)

type entry struct {
	response *dns.Msg
	expire   int64
}

type nameservers struct {
	cacheOn       bool
	negativeCache bool
	maxCacheTTL   int64
	canBroadcast  bool
	slist         []string // read-only list of servers

	cmu   sync.RWMutex
	cache map[string]entry

	rmu   sync.Mutex
	sring *ring.Ring
}

var (
	// TODO: make servers variable thread-safe to implement automatic config reload
	servers    *nameservers
	dnsServers map[string]*dns.Server
)

func parseConfig() int {
	nservers := viper.GetStringSlice("nameservers")
	servers = &nameservers{sring: ring.New(len(nservers)), slist: make([]string, len(nservers))}
	for i := 0; i < servers.sring.Len(); i++ {
		nameserver := fixDNSAddress(nservers[i])
		servers.sring.Value = nameserver
		servers.sring = servers.sring.Next()
		servers.slist[i] = nameserver
	}
	servers.cacheOn = viper.GetBool("cache")
	servers.negativeCache = viper.GetBool("negative_cache")
	servers.maxCacheTTL = viper.GetInt64("max_cache_ttl")
	servers.cache = make(map[string]entry)
	servers.canBroadcast = servers.sring.Len() > 1
	return servers.sring.Len()
}

func dumpConfig() {
	glog.Infoln("config: bind", viper.GetString("bind"))
	glog.Infoln("config: tcp", viper.GetBool("tcp"))
	glog.Infoln("config: nameservers", servers.slist)
	glog.Infoln("config: negative_cache", servers.negativeCache)
	glog.Infoln("config: max_cache_ttl", servers.maxCacheTTL)
}

func startServers(listenAddr string) {
	dnsServers := make(map[string]*dns.Server)

	for _, transport := range getTransports() {
		dnsServers[transport] = &dns.Server{Addr: listenAddr, Net: transport}
	}
	dns.HandleFunc(".", resolve)
	for _, server := range dnsServers {
		go func(s *dns.Server) {
			glog.Infoln("starting server", s.Addr, "-", s.Net)
			if err := s.ListenAndServe(); err != nil {
				glog.Fatalln("error starting dns server: ", err)
			}
		}(server)
	}
}

func stopServers() {
	for _, server := range dnsServers {
		glog.Infoln("shuting down server", server.Addr, "-", server.Net)
		if err := server.Shutdown(); err != nil {
			glog.Errorln("error shuting down server:", err)
		}
	}
}

func getNameServer() string {
	servers.rmu.Lock()
	defer servers.rmu.Unlock()

	server := servers.sring.Value.(string)
	servers.sring = servers.sring.Next()

	return server
}

func getResponseFromCache(question string) *dns.Msg {
	if !servers.cacheOn {
		return nil
	}
	servers.cmu.RLock()
	defer servers.cmu.RUnlock()
	if value, ok := servers.cache[question]; ok {
		if value.expire < time.Now().Unix() {
			// remove from cache now
			// TODO: return cached value and update cache on a goroutine
			glog.Infoln("expiring cache")
			delete(servers.cache, question)
			return nil
		}
		return value.response
	}
	return nil
}

func updateCache(question string, response *dns.Msg) {
	if !servers.cacheOn {
		return
	}
	// we respect TTL as long as it is lower than max_cache_ttl
	now := time.Now().Unix()
	exp := now + servers.maxCacheTTL
	if len(response.Answer) > 0 {
		ttlexp := now + int64(response.Answer[0].Header().Ttl)
		if ttlexp < exp {
			exp = ttlexp
		}
	}
	servers.cmu.Lock()
	defer servers.cmu.Unlock()
	servers.cache[question] = entry{response: response, expire: exp}
}

func clearCache() {
	servers.cmu.Lock()
	defer servers.cmu.Unlock()
	servers.cache = make(map[string]entry)
}

func getTransports() []string {
	if viper.GetBool("tcp") {
		return []string{"udp", "tcp"}
	}
	return []string{"udp"}
}

func directResolve(req *dns.Msg, transport string, nameserver string) (*dns.Msg, error) {
	client := &dns.Client{Net: transport}
	glog.Infoln("trying to resolv", req.Question, "using", nameserver)
	in, _, err := client.Exchange(req, nameserver)
	return in, err
}

func broadcastResolve(req *dns.Msg, transport string, usedns string) (*dns.Msg, error) {
	total := len(servers.slist) - 1
	resp := make([]*dns.Msg, total)
	errs := make([]error, total)
	var wg sync.WaitGroup
	actual := 0
	for _, nameserver := range servers.slist {
		if nameserver == usedns {
			// skip already used nameserver
			glog.Infoln("used nameserver", usedns, "skipping", nameserver)
			continue
		}
		wg.Add(1)
		go func(pos int, ns string) {
			defer wg.Done()
			in, err := directResolve(req, transport, ns)
			resp[pos] = in
			errs[pos] = err
		}(actual, nameserver)
		actual++
	}
	// wait all to finish
	wg.Wait()
	// return first valid no-error response
	erroridx := 0
	for i := 0; i < total; i++ {
		if errs[i] == nil && !isError(resp[i]) {
			return resp[i], nil
		}
		erroridx = i
	}
	return resp[erroridx], errs[erroridx]
}

func isError(msg *dns.Msg) bool {
	// NOERROR = 0 (NXDOMAIN = 3)
	return msg.MsgHdr.Rcode != 0
}

func resolve(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 0 {
		dns.HandleFailed(w, req)
		return
	}

	in := getResponseFromCache(dnsMsgToStr(req))

	if in == nil {
		// not found in cache, first attempt to resolv is
		// using a nameserver from all possibilites using
		// round-robin
		transport := "udp"
		if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
			transport = "tcp"
		}
		var err error
		var nameserver = getNameServer()
		in, err = directResolve(req, transport, nameserver)
		// check for connection error or NXDOMAIN
		if (err != nil || isError(in)) && servers.canBroadcast {
			// check all nameservers for
			in, err = broadcastResolve(req, transport, nameserver)
			if err != nil {
				// we got network error from all servers ()
				dns.HandleFailed(w, req)
				return
			}
		}
		// if response is NXDOMAIN we only cache it if
		// negative_cache is configured
		if !isError(in) || (isError(in) && servers.negativeCache) {
			glog.Infoln("updating cache: valid result")
			updateCache(dnsMsgToStr(req), in)
		}
	} else {
		glog.Infoln("returning cached result")
		in.MsgHdr.Id = req.MsgHdr.Id // huh
	}

	if err := w.WriteMsg(in); err != nil {
		glog.Errorln("error writing response to client:", err)
	}
}

func dnsMsgToStr(req *dns.Msg) string {
	// TODO: validate this
	return req.Question[0].String()
}

func fixDNSAddress(addr string) string {
	defaultPort := "53"
	if !strings.Contains(addr, ":") {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}
