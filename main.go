package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	_ "github.com/miekg/dns"
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

	viper.SetDefault("bind", map[string]interface{}{"address": "127.0.0.1", "port": 53})
	if config != "" {
		viper.SetConfigFile(config)
	} else {
		viper.SetConfigName("lresolver")
		viper.AddConfigPath("/etc/lresolver/")
		viper.AddConfigPath(".")
	}

	err := viper.ReadInConfig()
	if err != nil {
		fmt.Errorf("Fatal error config file: %s \n", err)
		os.Exit(1)
	}

	fmt.Println(viper.Get("bind"))
}
