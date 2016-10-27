package main

import (
	"container/ring"
	"net"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/miekg/dns"
	"github.com/spf13/viper"
)

type entry struct {
	response *dns.Msg
	ttl      uint32
}

type nameservers struct {
	respectTTL    bool
	negativeCache bool
	maxCacheTTL   int

	cmu   sync.RWMutex
	cache map[string]entry

	rmu   sync.Mutex
	sring *ring.Ring
}

// TODO: make servers variable thread-safe to implement automatic config reload
var (
	servers *nameservers
)

func readConfig() int {
	nservers := viper.GetStringSlice("nameservers")
	servers = &nameservers{sring: ring.New(len(nservers))}
	for i := 0; i < servers.sring.Len(); i++ {
		servers.sring.Value = fixDNSAddress(nservers[i])
		servers.sring = servers.sring.Next()
	}
	servers.respectTTL = viper.GetBool("respect_ttl")
	servers.negativeCache = viper.GetBool("negative_cache")
	servers.maxCacheTTL = viper.GetInt("max_cache_ttl")
	servers.cache = make(map[string]entry)
	return servers.sring.Len()
}

func getNameServer() string {
	servers.rmu.Lock()
	defer servers.rmu.Unlock()

	server := servers.sring.Value.(string)
	servers.sring = servers.sring.Next()

	return server
}

func getResponseFromCache(question string) *dns.Msg {
	servers.cmu.RLock()
	defer servers.cmu.RUnlock()
	if value, ok := servers.cache[question]; ok {
		// TODO: check for TTL
		return value.response
	}
	return nil
}

func updateCache(question string, response *dns.Msg) {
	var ttl uint32
	if len(response.Answer) > 0 {
		ttl = response.Answer[0].Header().Ttl
	}
	servers.cmu.Lock()
	defer servers.cmu.Unlock()
	servers.cache[question] = entry{response: response, ttl: ttl}
}

func getTransports() []string {
	if viper.GetBool("tcp") {
		return []string{"udp", "tcp"}
	}
	return []string{"udp"}
}

func directResolv(req *dns.Msg, transport string, nameserver string) (*dns.Msg, error) {
	client := &dns.Client{Net: transport}
	in, _, err := client.Exchange(req, nameserver)
	return in, err
}

func broadcastResolv(req *dns.Msg, transport string, usedns string) (*dns.Msg, error) {
	return nil, nil
}

func isError(msg *dns.Msg) bool {
	// NOERROR = 0 (NXDOMAIN = 3)
	return msg.MsgHdr.Rcode != 0
}

func resolve(w dns.ResponseWriter, req *dns.Msg) {
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
		in, err = directResolv(req, transport, nameserver)
		// check for connection error or NXDOMAIN
		if err != nil || isError(in) {
			// check all nameservers for
			in, err = broadcastResolv(req, transport, nameserver)
			if err != nil {
				// we got network error from all servers
				dns.HandleFailed(w, in)
				return
			}
		}
		// if response is NXDOMAIN we only cache it if
		// negative_cache is configured
		if !isError(in) || (isError(in) && servers.negativeCache) {
			updateCache(dnsMsgToStr(req), in)
		}
	} else {
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
