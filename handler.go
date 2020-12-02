package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Handler struct {
	jailer       Jailer

	responsesMux sync.Mutex
	responses    map[string](map[int]int) // Http response code counts

	quitChan     chan bool
}

func NewHandler(jailer Jailer) (*Handler, error) {
	handler := &Handler{
		   jailer: jailer,
		responses: make(map[string](map[int]int)),
		 quitChan: make(chan bool),
	}

	go func() {
		for {
			select {
				case <-handler.quitChan:
					break
				case <-time.After(10 * time.Minute):
					handler.responsesMux.Lock()

					for uri, stats := range handler.responses {
						pretty := ""
						for k, v := range stats {
							pretty += fmt.Sprintf(" %d:%d", k, v)
						}
						InfoLog("stats: %s:%s", uri, pretty)
					}

					handler.responsesMux.Unlock()
			}
		}
	}()

	return handler, nil
}

func (h *Handler) Close() error {
	h.quitChan <- true
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	respond := func(code int) {
		w.WriteHeader(code)

		h.responsesMux.Lock()
		defer h.responsesMux.Unlock()

		uri := r.RequestURI
		if strings.HasPrefix(uri, "/infraction/") {
			uri = "/infraction/*"
		}

		if _, ok := h.responses[uri]; !ok {
			h.responses[uri] = make(map[int]int)
		}
		if _, ok := h.responses[uri][code]; !ok {
			h.responses[uri][code] = 0
		}
		h.responses[uri][code]++
	}

	if strings.HasPrefix(r.RequestURI, "/infraction/") {
		s := r.RequestURI[len("/infraction/"):]
		ip :=  net.ParseIP(s)
		if ip == nil {
			WarningLog("%q is not a valid ip", s)
			respond(http.StatusBadRequest)
			return
		}

		if err := h.jailer.AddInfraction(ip); err != nil {
			ErrorLog(err.Error())
			respond(http.StatusServiceUnavailable)
			return
		}
		respond(http.StatusOK)

	} else if r.RequestURI == "/" {
		respond(http.StatusOK)

	} else if r.RequestURI == "/state/infractions" {
		respond(http.StatusOK)
		if err := h.jailer.WriteState(&w); err != nil {
			ErrorLog(err.Error())
		}

	} else if r.RequestURI == "/state/requests" {
		respond(http.StatusOK)

		h.responsesMux.Lock()
		defer h.responsesMux.Unlock()

		table := make(map[string]string)
		for uri, stats:= range h.responses {
			pretty := ""
			for k, v := range stats {
				pretty += fmt.Sprintf(" %d:%d", k, v)
			}
			table[uri] = pretty
		}

		if err := WriteTable(&w, table); err != nil {
			ErrorLog(err.Error())
		}

	} else {
		WarningLog("unsupported uri: %s", r.RequestURI)
		respond(http.StatusNotFound)
	}
}
