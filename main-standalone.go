package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	var logLevel int
	flag.IntVar(&logLevel, "loglevel", 2, "log level")
	flag.IntVar(&logLevel, "l", 2, "log level")

	var port int
	flag.IntVar(&port, "port", 8000, "port")
	flag.IntVar(&port, "p", 8000, "port")

	flag.Parse()

	DefaultLogger.Level = logLevel

	jailer, err := NewStandaloneJailer()
	if err != nil {
		PanicLog(err.Error())
	}
	defer func() {
		if err := jailer.Close(); err != nil {
			PanicLog(err.Error())
		}
	}()

	handler, err := NewHandler(*jailer)
	if err != nil {
		PanicLog(err.Error())
	}
	defer func() {
		if err := handler.Close(); err != nil {
			PanicLog(err.Error())
		}
	}()

	http.Handle("/infraction/", handler)

	if logLevel <= 1 {
		http.Handle("/state/infractions", handler)
		http.Handle("/state/requests", handler)
	}

	InfoLog("listening on port %d", port)
	server := http.Server{Addr:fmt.Sprintf(":%d", port)}
	if logLevel <= 1 {
		server.ErrorLog = log.New(os.Stderr, "http.Server: ", log.LstdFlags)
	}

	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		s := <-sigchan
		InfoLog("terminating signal received (%d)", s)
		server.Close()
	}()

	if err := server.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			PanicLog(err.Error())
		}
	}
}
