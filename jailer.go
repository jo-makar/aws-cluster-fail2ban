package main

import (
	"net"
)

// Using the fail2ban jail options terminology
// Ref: https://www.fail2ban.org/wiki/index.php/MANUAL_0_8#Jail_Options
const (
	MaxRetry = 3
	FindTime = 600 // In seconds
	BanTime = 1800 // In seconds
)

type Jailer interface {
	AddInfraction(ip net.IP) error

	Ban(ip net.IP) error
	Unban(ip net.IP) error

	Close() error
}
