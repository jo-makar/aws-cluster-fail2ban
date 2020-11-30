package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type IpSet struct {
	Name, Id string
	mux      sync.Mutex
}

func NewIpSet(name string) (*IpSet, error) {
	cmd := []string{"aws", "wafv2", "list-ip-sets", "--scope", "REGIONAL"}

	out, err := exec.Command(cmd[0], cmd[1:]...).Output()
	if err != nil {
		return nil, err
	}

	var parsed struct {
		IpSets []struct {
			Name string `json:"Name"`
			Id   string `json:"Id"`
		} `json:"IPSets"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil, err
	}

	id := ""
	for _, ipset := range parsed.IpSets {
		if ipset.Name == name {
			if id == "" {
				id = ipset.Id
			} else {
				WarningLog("multiple %s ip sets found", name)
			}
		}
	}

	if id == "" {
		return nil, fmt.Errorf("no %s ip set found", name)
	}

	return &IpSet{ Name: name, Id: id }, nil
}

func (i *IpSet) Get() ([]net.IP, string, error) {
	cmd := []string{"aws", "wafv2", "get-ip-set",
	                "--name", i.Name, "--scope", "REGIONAL", "--id", i.Id}

	out, err := exec.Command(cmd[0], cmd[1:]...).Output()
	if err != nil {
		return nil, "", err
	}

	var parsed struct {
		IpSet struct {
			Name  string   `json:"Name"`
			Id    string   `json:"Id"`
			IpVer string   `json:"IPAddressVersion"`
			Addrs []string `json:"Addresses"`
		} `json:"IPSet"`

		LockToken string `json:"LockToken"`
	}

	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil, "", err
	}

	ips := []net.IP{}
	for _, addr := range parsed.IpSet.Addrs {
		t := strings.Split(addr, "/")
		if len(t) != 2 {
			WarningLog("unexpected address format %s", addr)
			continue
		}
		if t[1] != "32" {
			WarningLog("non address cidr %s", addr)
			continue
		}

		ip := net.ParseIP(t[0])
		if ip == nil {
			WarningLog("invalid address %s", addr)
			continue
		}
		ips = append(ips, ip)
	}

	return ips, parsed.LockToken, nil
}

// TODO Rather than adding/deleting one at a time using a queuing system and batch the operations?

func (i *IpSet) Add(ip net.IP) error {
	i.mux.Lock()
	defer i.mux.Unlock()

	limit := 3
	for n := 0; n < limit; n++ {
		ips, token, err := i.Get()
		if err != nil {
			return err
		}

		if len(ips) == 10000 {
			return fmt.Errorf("ipset at maximum capacity")
		}

		for _, i := range ips {
			if i.Equal(ip) {
				return nil
			}
		}

		cmd := []string{"aws", "wafv2", "update-ip-set",
				"--name", i.Name, "--scope", "REGIONAL", "--id", i.Id,
				"--lock-token", token, "--addresses"}

		for _, i := range ips {
			cmd = append(cmd, i.String() + "/32")
		}
		cmd = append(cmd, ip.String() + "/32")

		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			WarningLog("failed to update ipset to add %s attempt %d", ip.String(), n+1)
			if n < limit-1 {
				time.Sleep(time.Duration((n+1) * 5) * time.Second)
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("lock contention attempting to add %s", ip.String())
}

func (i *IpSet) Del(ip net.IP) error {
	i.mux.Lock()
	defer i.mux.Unlock()

	limit := 3
	for n := 0; n < limit; n++ {
		ips, token, err := i.Get()
		if err != nil {
			return err
		}

		found := false
		for _, i := range ips {
			if i.Equal(ip) {
				found = true
				break
			}
		}
		if !found {
			return nil
		}

		cmd := []string{"aws", "wafv2", "update-ip-set",
				"--name", i.Name, "--scope", "REGIONAL", "--id", i.Id,
				"--lock-token", token, "--addresses"}

		for _, i := range ips {
			if i.Equal(ip) {
				continue
			}
			cmd = append(cmd, i.String() + "/32")
		}

		if err := exec.Command(cmd[0], cmd[1:]...).Run(); err != nil {
			WarningLog("failed to update ipset to delete %s attempt %d", ip.String(), n+1)
			if n < limit-1 {
				time.Sleep(time.Duration((n+1) * 5) * time.Second)
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("lock contention attempting to delete %s", ip.String())
}
