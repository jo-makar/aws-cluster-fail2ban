package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

type StandaloneJailer struct {
	mux         sync.Mutex
	                                     // net.IP is a slice type and cannot be used to map keys
	infractions map[string]([]time.Time) // Unix timestamps of infractions by offending ip

	quitChans   []chan bool
}

func NewStandaloneJailer() (*StandaloneJailer, error) {
	jailer := &StandaloneJailer{
		infractions: make(map[string]([]time.Time)),
		  quitChans: []chan bool{make(chan bool)},
	}

	go func(quitChan <-chan bool) {
		for {
			select {
				case <-quitChan:
					break
				case <-time.After(2 * FindTime * time.Second):
					jailer.cleanup()
			}
		}
	}(jailer.quitChans[0])

	// FIXME periodically unban ips

	// FIXME periodically output stats via a goroutine? or just rely on /state/...
	/* 
	go func(quitChan <-chan bool) {
		for {
			select {
				case <-quitChan:
					break
				case <-time.After(2 * time.Minute):
					jailer.cleanup()
			}
		}
	}(jailer.quitChans[1])
	*/

	return jailer, nil
}

func (j StandaloneJailer) Close() error {
	for _, c := range j.quitChans {
		c <- true
	}
	return nil
}

func (j StandaloneJailer) AddInfraction(ip net.IP) error {
	j.mux.Lock()
	defer j.mux.Unlock()

	s := ip.String()

	if _, ok := j.infractions[s]; !ok {
		j.infractions[s] = []time.Time{}
	}

	var i int
	for i = 0; i < len(j.infractions[s]); i++ {
		if time.Now().Sub(j.infractions[s][i]).Seconds() < FindTime {
			break
		}
	}
	if i == len(j.infractions[s]) {
		j.infractions[s] = []time.Time{}
	} else if i > 0 {
		j.infractions[s] = j.infractions[s][i:]
	}

	j.infractions[s] = append(j.infractions[s], time.Now())

	var o strings.Builder
	o.WriteString("[")
	for i, v := range j.infractions[s] {
		if i > 0 {
			o.WriteString(" ")
		}
		o.WriteString(v.Format("15:04:05"))
	}
	o.WriteString("]")
	DebugLog("infractions[%s] = %s", s, o.String())

	if len(j.infractions[s]) >= MaxRetry {
		InfoLog("%s banned due to %d infractions", s, len(j.infractions[s]))
		return j.Ban(ip)
	}

	return nil
}

func (j StandaloneJailer) cleanup() {
	j.mux.Lock()
	defer j.mux.Unlock()

	ipsDeleted := 0
	ipsAffected := 0
	infractionsDeleted := 0

	for k := range j.infractions {
		var i int
		for i = 0; i < len(j.infractions[k]); i++ {
			if time.Now().Sub(j.infractions[k][i]).Seconds() < FindTime {
				break
			}
		}
		if i == len(j.infractions[k]) {
			delete(j.infractions, k)
			ipsDeleted++
		} else if i > 0 {
			j.infractions[k] = j.infractions[k][i:]
			ipsAffected++
			infractionsDeleted += i
		}
	}

	suffix := func(v int) string {
		if v > 1 {
			return "s"
		} else {
			return ""
		}
	}

	if ipsDeleted > 0 {
		InfoLog("cleanup: %d ip%s deleted", ipsDeleted, suffix(ipsDeleted))
	}
	if ipsAffected > 0 {
		InfoLog("cleanup: %d infraction%s deleted from %d ip%s",
		        infractionsDeleted, suffix(infractionsDeleted), ipsAffected, suffix(ipsAffected))
	}
}

func (j StandaloneJailer) Ban(ip net.IP) error {
	// FIXME need to maintain state (ie a variable) for when the ban ends
	return nil
}

func (j StandaloneJailer) Unban(ip net.IP) error {
	// FIXME
	return nil
}
