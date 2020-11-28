package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

func ipToKey(ip net.IP) string {
	return fmt.Sprintf("aws-fail2ban-%s", ip.String())
}

func keyToIp(key string) net.IP {
	return net.ParseIP(key[13:])
}

type ServiceJailer struct {
	ipset       *IpSet

	// Concurrency-safe, ref: https://github.com/go-redis/redis/blob/master/redis.go
	redisClient *redis.Client

	quitChan    chan bool
}

func NewServiceJailer(ipsetName, redisAddr string) (*ServiceJailer, error) {
	ipset, err := NewIpSet(ipsetName)
	if err != nil {
		return nil, err
	}

	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	_, err = redisClient.Ping(context.Background()).Result()
	if err != nil {
		return nil, err
	}

	jailer := &ServiceJailer{
		      ipset: ipset,
		redisClient: redisClient,
		   quitChan: make(chan bool),
	}

	// Sleep a random amount of time should multiple containers be started simultaneously
	rand.Seed(time.Now().UnixNano())
	time.Sleep(time.Duration(rand.Intn(60)) * time.Second)

	// Ensure ip set contents are being managed
	if ips, _, err := ipset.Get(); err != nil {
		return nil, err
	} else {
		for _, ip := range ips {
			llen, err := redisClient.LLen(context.Background(), ipToKey(ip)).Result()
			if err != nil {
				ErrorLog(err.Error())
				continue
			}

			for i:=llen; i<MaxRetry; i++ {
				if err := jailer.AddInfraction(ip); err != nil {
					ErrorLog(err.Error())
				}
			}
		}
	}

	go func() {
		// Use random periods in a crude attempt to avoid overlap with other containers
		// TODO Ideally want something based on FindTime and BanTime
		period := time.Duration(rand.Intn(60) + 60) * time.Second

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

func (j ServiceJailer) Close() error {
	j.quitChan <- true

	if err := j.redisClient.Close(); err != nil {
		return err
	}

	return nil
}

func (j ServiceJailer) manageState() {
	ctx := context.Background()

	var cursor uint64 = 0
	var count int64 = 100

	keysEvaluated := 0
	scanIterations := 0
	ipsAffected := 0
	infractionsDeleted := 0
	ipsUnbanned := 0

	listToInfractions := func(redisList []string) []time.Time {
		var rv []time.Time
		for _, s := range redisList {
			unixtime, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				ErrorLog("unable to parse time %s", s)
				continue
			}
			t := time.Unix(unixtime, 0)

			rv = append(rv, t)
		}
		return rv
	}

	bannedUntil := func(infractions []time.Time) (time.Time, int) {
		if len(infractions) < MaxRetry {
			return time.Time{}, -1
		}

		now := time.Now().UTC()
		start := 0
		for ; start < len(infractions); start++ {
			if !infractions[start].Before(now.Add(-(FindTime * time.Second))) {
				break
			}
		}

		if len(infractions) - start >= MaxRetry {
			return infractions[len(infractions)-1].Add(BanTime * time.Second), start
		} else {
			return time.Time{}, start
		}
	}


	for {
		keys, retCursor, err := j.redisClient.Scan(ctx, cursor, "aws-fail2ban-*", count).Result()
		if err != nil {
			ErrorLog(err.Error())
		}

		for _, key := range keys {
			ip := keyToIp(key)
			if ip == nil {
				ErrorLog("unable to parse ip from %s", key)
				continue
			}

			redisList, err := j.redisClient.LRange(ctx, key, 0, -1).Result()
			if err != nil {
				ErrorLog(err.Error())
				continue
			}

			infractions := listToInfractions(redisList)
			endtime, start := bannedUntil(infractions)
			if !endtime.IsZero() && time.Now().UTC().Before(endtime) {
				DebugLog("%s banned until %s", ip.String(), endtime.Format("2006-01-02T15:04:05"))
				continue
			}

			if start > 0 {
				if _, err := j.redisClient.LTrim(ctx, key, int64(start), -1).Result(); err != nil {
					ErrorLog(err.Error())
				}

				if len(infractions) >= MaxRetry && len(infractions)-start < MaxRetry {
					InfoLog("%s is unbanned", ip.String())
					if err := j.Unban(ip); err != nil {
						ErrorLog(err.Error())
					}
					ipsUnbanned++
				}

				ipsAffected++
				infractionsDeleted += start
			}
		}

		keysEvaluated += len(keys)
		scanIterations++

		if retCursor == 0 {
			break
		}
		cursor = retCursor

		if count < 1000 {
			count *= 2
		}
	}

	suffix := func(v int) string {
		if v > 1 {
			return "s"
		} else {
			return ""
		}
	}

	InfoLog("manageState: %d key%s evaluated in %d scan iteration%s",
	        keysEvaluated, suffix(keysEvaluated), scanIterations, suffix(scanIterations))

	if ipsUnbanned > 0 {
		InfoLog("manageState: %d ip%s unbanned", ipsUnbanned, suffix(ipsUnbanned))
	}
	if ipsAffected > 0 {
		InfoLog("manageState: %d infractions%s deleted from %d ip%s",
		        infractionsDeleted, suffix(infractionsDeleted), ipsAffected, suffix(ipsAffected))
	}
}

func (j ServiceJailer) AddInfraction(ip net.IP) error {
	ctx := context.Background()

	now := time.Now().UTC()
	llen, err := j.redisClient.RPush(ctx, ipToKey(ip), now.Unix()).Result()
	if err != nil {
		return err
	}

	DebugLog("%s infraction at %s", ip.String(), now.Format("2006-01-02T15:04:05"))

	if llen >= MaxRetry {
		InfoLog("%s banned due to %d infractions", ip.String(), llen)
		if err := j.Ban(ip); err != nil {
			return err
		}

		if llen > MaxRetry {
			if _, err := j.redisClient.LTrim(ctx, ipToKey(ip), llen-MaxRetry, -1).Result(); err != nil {
				return err
			}
		}
	}

	if _, err := j.redisClient.Expire(ctx, ipToKey(ip), 2 * BanTime * time.Second).Result(); err != nil {
		return err
	}

	return nil
}

func (j ServiceJailer) Ban(ip net.IP) error {
	return j.ipset.Add(ip)
}

func (j ServiceJailer) Unban(ip net.IP) error {
	return j.ipset.Del(ip)
}

func (j ServiceJailer) WriteState(w *http.ResponseWriter) error {
	var err error = nil
	write := func(s string) {
		if err != nil {
			return
		}

		_, err = (*w).Write([]byte(s))
		if err != nil {
			ErrorLog(err.Error())
		}
	}

	write("<html><body>\n")
	write("not implemented, instead refer to:<br/>\n")
	write("<tt>redis-cli -h &lt;ip&gt; -p &lt;port&gt; --scan --pattern &lt;prefix&gt;-*</tt>\n")
	write("</body></html>\n")

	return err
}

