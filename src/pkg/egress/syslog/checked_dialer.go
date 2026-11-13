package syslog

import (
	"errors"
	"net"
	"net/netip"
	"syscall"
)

var ErrBlockedAddress = errors.New("syslog drain destination address is blacklisted")

func checkedDialer(netConf NetworkTimeoutConfig) *net.Dialer {
	dialer := &net.Dialer{
		Timeout:   netConf.DialTimeout,
		KeepAlive: netConf.Keepalive,
	}

	if netConf.IPChecker != nil {
		dialer.Control = func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlockedAddress
			}

			addr, err := netip.ParseAddr(host)
			if err != nil {
				return ErrBlockedAddress
			}

			a := addr.As16()
			ip := net.IP(a[:])
			if err := netConf.IPChecker.CheckBlacklist(ip); err != nil {
				return ErrBlockedAddress
			}

			return nil
		}
	}

	return dialer
}
