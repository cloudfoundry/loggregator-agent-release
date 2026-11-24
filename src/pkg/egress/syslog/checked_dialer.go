package syslog

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"syscall"

	"github.com/valyala/fasthttp"
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

// blacklistResolver is a fasthttp.Resolver that drops blacklisted addresses
// from every lookup. fasthttp.TCPDialer only dials addresses this resolver
// returns (including IP literals), so a blocked address is never connected to,
// while the dialer keeps its DNS cache, round-robin and concurrency limit.
type blacklistResolver struct {
	checker IPChecker
}

func (r blacklistResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	allowed := addrs[:0]
	for _, a := range addrs {
		ip := a.IP.To16()
		if ip == nil {
			continue
		}
		if r.checker.CheckBlacklist(ip) == nil {
			allowed = append(allowed, a)
		}
	}
	if len(allowed) == 0 {
		return nil, ErrBlockedAddress
	}
	return allowed, nil
}

// checkedTCPDialer returns a fasthttp.TCPDialer that only dials addresses
// accepted by netConf.IPChecker. Blacklisting is applied before the DNS
// cache, so cached entries are always allowed addresses.
func checkedTCPDialer(netConf NetworkTimeoutConfig) *fasthttp.TCPDialer {
	return &fasthttp.TCPDialer{
		Concurrency: 1000, // same as fasthttp's default dialer
		Resolver:    blacklistResolver{checker: netConf.IPChecker},
	}
}
