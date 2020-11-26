package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

type Handler struct {
	jailer   Jailer

	mux      sync.Mutex
	                                  // net.IP is a slice type and cannot be used to map keys
	requests map[string](map[int]int) // Http response code counts by requesting ip
}

func NewHandler(jailer Jailer) (*Handler, error) {
	return &Handler{
		  jailer: jailer,
		requests: make(map[string](map[int]int)),
	}, nil

	// FIXME periodically output stats via a goroutine? or just rely on /state/...
}

func (h *Handler) Close() error {
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.RequestURI, "/infraction/") {
		s := r.RequestURI[len("/infraction/"):]
		ip :=  net.ParseIP(s)
		if ip == nil {
			WarningLog("%q is not a valid ip", s)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// FIXME skip link local, etc.  also check error
		h.jailer.AddInfraction(ip)
		// FIXME STOPPED use jailer to standalone vs service handling



	} else if r.RequestURI == "/state/infractions" {
		// FIXME

	} else if r.RequestURI == "/state/requests" {
		// FIXME

	} else {
		WarningLog("unsupported uri: %s", r.RequestURI)
		w.WriteHeader(http.StatusNotFound)
	}
}
