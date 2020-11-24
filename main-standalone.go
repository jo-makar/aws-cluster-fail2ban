package main

import (
	"flag"
)

func main() {
	var logLevel int
	flag.IntVar(&logLevel, "loglevel", 2, "log level")
	flag.IntVar(&logLevel, "l", 2, "log level")

	flag.Parse()

	DefaultLogger.Level = logLevel

	// FIXME STOPPED
}
