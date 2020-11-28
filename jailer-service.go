package main

import (
	"context"
	"fmt"
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

func (j ServiceJailer) Close() error {
	j.quitChan <- true

	if err := j.redisClient.Close(); err != nil {
		return err
	}

	return nil
}

func (j ServiceJailer) manageState() {
	var cursor uint64 = 0
	var count int64 = 100

	keysEvaluated := 0
	scanIterations := 0
	ipsUnbanned := 0

	for {
		keys, retCursor, err := j.redisClient.Scan(context.Background(), cursor, "aws-fail2ban-*", count).Result()
		if err != nil {
			ErrorLog(err.Error())
		}

		for _, key := range keys {
			llen, err := j.redisClient.LLen(context.Background(), key).Result()
			if err != nil {
				ErrorLog(err.Error())
				continue
			}

			if llen >= MaxRetry {
				val, err := j.redisClient.LIndex(context.Background(), key, -1).Result()
				if err != nil {
					ErrorLog(err.Error())
					continue
				}

				unixtime, err := strconv.ParseInt(val, 10, 64)
				if err != nil {
					ErrorLog(err.Error())
					continue
				}

				endtime := time.Unix(unixtime, 0)
				if endtime.Add(BanTime * time.Second).Before(time.Now().UTC()) {
					ip := keyToIp(key)
					if ip == nil {
						WarningLog("unable to parse ip from %s", key)
						continue
					}

					InfoLog("%s is unbanned", ip.String())
					if err := j.Unban(ip); err != nil {
						ErrorLog(err.Error())
					}
				}
			}
		}

		keysEvaluated += len(keys)
		scanIterations++

		if len(keys) == 0 {
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
}

func (j ServiceJailer) AddInfraction(ip net.IP) error {
	// FIXME lpush, ltrim, expire so that redis is self-maintaining
	// FIXME careful to explicitly make all time.Times in utc by using timeobj.UTC()
	return nil
}

func (j ServiceJailer) Ban(ip net.IP) error {
	// FIXME ipset.Add
	return nil
}

func (j ServiceJailer) Unban(ip net.IP) error {
	// FIXME lrange so that there are less than maxretry entries in redis
	//       then ipset.Del
	return nil
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

