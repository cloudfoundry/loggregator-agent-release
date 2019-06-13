package cups

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type BlacklistRange struct {
	Start string
	End   string
}

type BlacklistRanges struct {
	Ranges []BlacklistRange
}

func NewBlacklistRanges(ranges ...BlacklistRange) (*BlacklistRanges, error) {
	r := &BlacklistRanges{Ranges: ranges}

	err := r.validate()
	if err != nil {
		return nil, err
	}

	return r, nil
}

// UnmarshalEnv implements envstruct.Unmarshaller.
// Example input:
// 10.0.0.5-10.0.0.9,123.4.5.6-123.4.5.7
func (i *BlacklistRanges) UnmarshalEnv(v string) error {
	if v == "" {
		return nil
	}

	for _, ipRange := range strings.Split(v, ",") {
		ips := strings.Split(ipRange, "-")
		if len(ips) != 2 {
			return fmt.Errorf("invalid BlacklistRange: %s", ipRange)
		}

		i.Ranges = append(i.Ranges, BlacklistRange{
			Start: ips[0],
			End:   ips[1],
		})
	}

	return i.validate()
}

func (i *BlacklistRanges) validate() error {
	for _, ipRange := range i.Ranges {
		startIP := net.ParseIP(ipRange.Start)
		endIP := net.ParseIP(ipRange.End)
		if startIP == nil {
			return fmt.Errorf("invalid IP Address for Blacklist IP Range: %s", ipRange.Start)
		}
		if endIP == nil {
			return fmt.Errorf("invalid IP Address for Blacklist IP Range: %s", ipRange.End)
		}
		if bytes.Compare(startIP, endIP) > 0 {
			return fmt.Errorf("invalid Blacklist IP Range: Start %s has to be before End %s", ipRange.Start, ipRange.End)
		}
	}

	return nil
}

func (i *BlacklistRanges) CheckBlacklist(ip net.IP) error {
	for _, ipRange := range i.Ranges {
		if bytes.Compare(ip, net.ParseIP(ipRange.Start)) >= 0 && bytes.Compare(ip, net.ParseIP(ipRange.End)) <= 0 {
			return fmt.Errorf("syslog drain blacklisted: %s", ip)
		}
	}

	return nil
}

func (i *BlacklistRanges) ResolveAddr(host string) (net.IP, error) {
	ipAddress := net.ParseIP(host)
	if ipAddress == nil {
		ipAddr, err := net.ResolveIPAddr("ip", host)
		if err != nil {
			return nil, fmt.Errorf("unable to resolve DNS entry: %s", host)
		}
		ipAddress = net.ParseIP(ipAddr.String())
	}

	return ipAddress, nil
}

func (i *BlacklistRanges) ParseHost(drainURL string) (string, string, error) {
	testURL, err := url.Parse(drainURL)
	if err != nil {
		return "", "", err
	}

	if len(testURL.Host) == 0 {
		return "", "", errors.New("invalid URL, detected no host")
	}

	host := strings.Split(testURL.Host, ":")[0]

	return testURL.Scheme, host, nil
}
