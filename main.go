package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
)

var (
	version = "devel"
)

func usage() {
	fmt.Printf("%s version %s (runtime: %s)\n", os.Args[0], version, runtime.Version())
}

func main() {
	flag.Usage = usage
	flag.Parse()
}
