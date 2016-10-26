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

type nameservers struct {
	respectTTL    bool
	negativeCache bool
	maxCacheTTL   int

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
	return servers.sring.Len()
}

func getNameServer() string {
	servers.rmu.Lock()
	defer servers.rmu.Unlock()

	server := servers.sring.Value.(string)
	servers.sring = servers.sring.Next()

	return server
}

func getTransports() []string {
	if viper.GetBool("tcp") {
		return []string{"udp", "tcp"}
	}
	return []string{"udp"}
}

func resolve(w dns.ResponseWriter, req *dns.Msg) {
	transport := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		transport = "tcp"
	}
	nameserver := getNameServer()
	glog.Infoln("request for", req.Question, "transport", transport, "endpoint", nameserver)
	client := &dns.Client{Net: transport}
	in, rtt, err := client.Exchange(req, nameserver)
	if err != nil {
		dns.HandleFailed(w, in)
		return
	}
	glog.Infoln("response:", dns.RcodeToString[in.MsgHdr.Rcode])
	glog.Infoln("response rtt:", rtt)
	if len(in.Answer) > 0 {
		glog.Infoln("response ttl:", in.Answer[0].Header().Ttl)
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
