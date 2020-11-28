package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type StandaloneJailer struct {
	infractionsMux sync.Mutex
	                                        // net.IP is a slice type and cannot be used to map keys
	infractions    map[string]([]time.Time) // Unix timestamps of infractions by offending ip

	ipset          *IpSet

	quitChan       chan bool
}

func NewStandaloneJailer(ipsetName string) (*StandaloneJailer, error) {
	ipset, err := NewIpSet(ipsetName)
	if err != nil {
		return nil, err
	}

	jailer := &StandaloneJailer{
		infractions: make(map[string]([]time.Time)),
		      ipset: ipset,
		   quitChan: make(chan bool),
	}

	// Ensure ip set contents are being managed
	if ips, _, err := ipset.Get(); err != nil {
		return nil, err
	} else {
		for _, ip := range ips {
			for i:=0; i<MaxRetry; i++ {
				if err := jailer.AddInfraction(ip); err != nil {
					ErrorLog(err.Error())
				}
			}
		}
	}

	go func() {
		// TODO Ideally want something based on FindTime and BanTime
		period := 1 * time.Minute

		for {
			select {
				case <-jailer.quitChan:
					break
				case <-time.After(period):
					jailer.manageState()
			}
		}
	}()

	return jailer, nil
}

func (j StandaloneJailer) Close() error {
	j.quitChan <- true
	return nil
}

func (j StandaloneJailer) AddInfraction(ip net.IP) error {
	j.infractionsMux.Lock()
	defer j.infractionsMux.Unlock()

	s := ip.String()

	if _, ok := j.infractions[s]; !ok {
		j.infractions[s] = []time.Time{}
	}

	j.infractions[s] = append(j.infractions[s], time.Now())

	var o strings.Builder
	o.WriteString("[")
	for i, v := range j.infractions[s] {
		if i > 0 {
			o.WriteString(" ")
		}
		o.WriteString(v.Format("2006-01-02T15:04:05"))
	}
	o.WriteString("]")
	DebugLog("infractions[%s] = %s", s, o.String())

	if len(j.infractions[s]) >= MaxRetry {
		InfoLog("%s banned due to %d infractions", s, len(j.infractions[s]))
		return j.Ban(ip)
	}

	return nil
}

func bannedUntil(infractions []time.Time) time.Time {
	if len(infractions) < MaxRetry {
		return time.Time{}
	}

	return infractions[len(infractions)-1].Add(BanTime * time.Second)
}

func (j StandaloneJailer) manageState() {
	j.infractionsMux.Lock()
	defer j.infractionsMux.Unlock()

	ipsDeleted := 0
	ipsAffected := 0
	infractionsDeleted := 0
	ipsUnbanned := 0

	unban := func(s string) {
		ipsUnbanned++

		ip := net.ParseIP(s)
		if ip == nil { // Should never happen
			ErrorLog("could not parse %s as an ip", s)
			return
		}

		InfoLog("%s is unbanned", s)
		if err := j.Unban(ip); err != nil {
			ErrorLog(err.Error())
		}
	}

	for ip := range j.infractions {
		origlen := len(j.infractions[ip])
		limit := len(j.infractions[ip])

		endtime := bannedUntil(j.infractions[ip])
		if !endtime.IsZero() && time.Now().Before(endtime) {
			limit = len(j.infractions[ip]) - MaxRetry
			DebugLog("%s banned until %s", ip, endtime.Format("2006-01-02T15:04:05"))
		}

		var i int
		for i = 0; i < limit; i++ {
			if time.Now().Sub(j.infractions[ip][i]).Seconds() < FindTime {
				break
			}
		}
		if i == len(j.infractions[ip]) {
			if origlen >= MaxRetry {
				unban(ip)
			}

			delete(j.infractions, ip)
			ipsDeleted++

		} else if i > 0 {
			if origlen >= MaxRetry && origlen-i < MaxRetry {
				unban(ip)
			}

			j.infractions[ip] = j.infractions[ip][i:]
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

	if ipsUnbanned > 0 {
		InfoLog("manageState: %d ip%s unbanned", ipsUnbanned, suffix(ipsUnbanned))
	}
	if ipsDeleted > 0 {
		InfoLog("manageState: %d ip%s deleted", ipsDeleted, suffix(ipsDeleted))
	}
	if ipsAffected > 0 {
		InfoLog("manageState: %d infraction%s deleted from %d ip%s",
		        infractionsDeleted, suffix(infractionsDeleted), ipsAffected, suffix(ipsAffected))
	}
}

func (j StandaloneJailer) Ban(ip net.IP) error {
	return j.ipset.Add(ip)
}

func (j StandaloneJailer) Unban(ip net.IP) error {
	return j.ipset.Del(ip)
}

func (j StandaloneJailer) WriteState(w *http.ResponseWriter) error {
	j.infractionsMux.Lock()
	defer j.infractionsMux.Unlock()

	table := make(map[string]string)
	for ip, times:= range j.infractions {
		pretty := ""
		for _, t := range times {
			pretty += t.Format(" 2006-01-02T15:04:05")
		}

		if endtime := bannedUntil(times); !endtime.IsZero() {
			pretty += endtime.Format(" (banned until 2006-01-02T15:04:05)")
		}

		table[ip] = pretty
	}

	return WriteTable(w, table)
}
