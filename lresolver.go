package main

import (
	"container/ring"
	"net"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

type nameservers struct {
	mu    sync.Mutex
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
		nameserver := nservers[i]
		if !strings.Contains(nameserver, ":") {
			nameserver = net.JoinHostPort(nameserver, "53")
		}
		servers.sring.Value = nameserver
		servers.sring = servers.sring.Next()
	}
	return servers.sring.Len()
}

func getNameserver() string {
	servers.mu.Lock()
	defer servers.mu.Unlock()

	server := servers.sring.Value.(string)
	servers.sring = servers.sring.Next()

	return server
}
