package main

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
)

func start() {
	var yamlConfig = []byte(`
bind: 127.0.0.1:5300
cache: true
negative_cache: true
max_cache_ttl: 300
tcp: true
nameservers:
- 8.8.8.8
- 8.8.4.4
`)
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBuffer(yamlConfig))
	if err != nil {
		panic(err)
	}
	startServers(fixDNSAddress(viper.GetString("bind")))
}

func stop() {
	stopServers()
}

func BenchmarkCache(b *testing.B) {
	// TODO
}
