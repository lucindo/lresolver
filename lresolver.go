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

func resolve(w dns.ResponseWriter, req *dns.Msg) {
	in := getResponseFromCache(req.Question[0].String())
	if in == nil {
		transport := "udp"
		if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
			transport = "tcp"
		}
		nameserver := getNameServer()
		glog.Infoln("request for", req.Question, "transport", transport, "endpoint", nameserver)
		client := &dns.Client{Net: transport}
		var err error
		var rtt time.Duration
		in, rtt, err = client.Exchange(req, nameserver)
		if err != nil {
			dns.HandleFailed(w, in)
			return
		}
		glog.Infoln("response:", dns.RcodeToString[in.MsgHdr.Rcode])
		glog.Infoln("response rtt:", rtt)
		if len(in.Answer) > 0 {
			glog.Infoln("response ttl:", in.Answer[0].Header().Ttl)
		}
		updateCache(req.Question[0].String(), in)
	} else {
		glog.Infoln("response from cache")
		in.MsgHdr.Id = req.MsgHdr.Id // huh
	}
	if err := w.WriteMsg(in); err != nil {
		glog.Errorln("error writing response to client:", err)
	}
}

func fixDNSAddress(addr string) string {
	defaultPort := "53"
	if !strings.Contains(addr, ":") {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}
