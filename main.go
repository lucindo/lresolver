package main

import (
	"flag"
	"fmt"
	"net"
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
	fmt.Fprintln(os.Stderr, "OPTIONS:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Printf("%s version %s (runtime: %s)\n", os.Args[0], version, runtime.Version())
}

func main() {
	flag.Parse()

	viper.SetDefault("bind", "127.0.0.1:53")
	if config != "" {
		viper.SetConfigFile(config)
	} else {
		viper.SetConfigName("lresolver")
		viper.AddConfigPath("/etc/lresolver/")
		viper.AddConfigPath(".")
	}

	err := viper.ReadInConfig()
	if err != nil {
		glog.Errorf("Fatal error config file: %s \n", err)
		os.Exit(1)
	}

	listenAddr := viper.GetString("bind")
	glog.Infoln("will listen on address:", listenAddr)

	servers := make(map[string]*dns.Server)

	for _, net := range []string{"udp", "tcp"} {
		servers[net] = &dns.Server{Addr: listenAddr, Net: net}
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
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	for _, server := range servers {
		glog.Infoln("shuting down server", server.Addr, "-", server.Net)
		server.Shutdown()
	}
}

func resolve(w dns.ResponseWriter, req *dns.Msg) {
	transport := "udp"
	if _, ok := w.RemoteAddr().(*net.TCPAddr); ok {
		transport = "tcp"
	}
	transport = "tcp"
	glog.Infoln("request:", req, "transport", transport, "endpoint", viper.GetString("nameserver"))
	client := &dns.Client{Net: transport}
	resp, _, err := client.Exchange(req, viper.GetString("nameserver"))
	if err != nil {
		dns.HandleFailed(w, req)
		return
	}
	w.WriteMsg(resp)
}
