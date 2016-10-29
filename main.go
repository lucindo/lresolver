package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/golang/glog"
	"github.com/miekg/dns"
	"github.com/spf13/viper"
)

var (
	config  string
	version = "devel"
)

func init() {
	flag.StringVar(&config, "config", "", "Config file")
	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "OPTIONS (none required):")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Unless specified otherwise configuration file will be lresolver.{yml,yaml,json,toml,hcl}")
	fmt.Fprintln(os.Stderr, "Path search for configuration file: \"/etc/lresolver/:.\"")
	fmt.Fprintln(os.Stderr, "Sample configuration (YAML format):")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "# lresolver configuration begin")
	fmt.Fprintln(os.Stderr, "bind: 127.0.0.1")
	fmt.Fprintln(os.Stderr, "negative_cache: true")
	fmt.Fprintln(os.Stderr, "max_cache_ttl: 300")
	fmt.Fprintln(os.Stderr, "tcp: true")
	fmt.Fprintln(os.Stderr, "nameservers:")
	fmt.Fprintln(os.Stderr, "- 8.8.8.8")
	fmt.Fprintln(os.Stderr, "- 8.8.4.4")
	fmt.Fprintln(os.Stderr, "# lresolver configuration end")
	fmt.Fprintln(os.Stderr, "")
	fmt.Printf("%s version %s (runtime: %s)\n", os.Args[0], version, runtime.Version())
}

func main() {
	flag.Parse()

	// defaults
	viper.SetDefault("bind", "127.0.0.1")
	viper.SetDefault("tcp", "true")
	viper.SetDefault("negative_cache", "true")
	viper.SetDefault("max_cache_ttl", 300)

	if config != "" {
		viper.SetConfigFile(config)
	} else {
		viper.SetConfigName("lresolver")
		viper.AddConfigPath("/etc/lresolver/")
		viper.AddConfigPath(".")
	}

	glog.Infoln("using configuration file:", viper.ConfigFileUsed())

	err := viper.ReadInConfig()
	if err != nil {
		glog.Errorln("Fatal error reading config file", err)
		os.Exit(1)
	}

	if parseConfig() < 1 {
		glog.Errorln("no DNS servers configured, exiting")
		os.Exit(2)
	}

	dumpConfig()

	listenAddr := fixDNSAddress(viper.GetString("bind"))
	glog.Infoln("will listen on address:", listenAddr)

	servers := make(map[string]*dns.Server)

	for _, transport := range getTransports() {
		servers[transport] = &dns.Server{Addr: listenAddr, Net: transport}
	}
	dns.HandleFunc(".", resolve)
	for _, server := range servers {
		go func(s *dns.Server) {
			glog.Infoln("starting server", s.Addr, "-", s.Net)
			if err := s.ListenAndServe(); err != nil {
				glog.Fatalln("error starting dns server: ", err)
			}
		}(server)
	}

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1)
	go func() {
		for sig := range sigs {
			switch sig {
			case syscall.SIGUSR1:
				glog.Infoln("cleaning up cache...")
				clearCache()
			default:
				glog.Info("exiting...")
				done <- true
			}
		}
	}()

	<-done
	for _, server := range servers {
		glog.Infoln("shuting down server", server.Addr, "-", server.Net)
		if err := server.Shutdown(); err != nil {
			glog.Errorln("error shuting down server:", err)
		}
	}
}
