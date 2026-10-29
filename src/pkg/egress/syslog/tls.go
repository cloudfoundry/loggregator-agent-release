package syslog

import (
	"crypto/tls"
	"net"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
)

// TLSWriter represents a syslog writer that connects over unencrypted TCP.
type TLSWriter struct {
	TCPWriter
}

type IPChecker interface {
	CheckBlacklist(ip net.IP) error
}

// NetworkTimeoutConfig stores various timeout values.
type NetworkTimeoutConfig struct {
	Keepalive    time.Duration
	DialTimeout  time.Duration
	WriteTimeout time.Duration
	IPChecker    IPChecker
}

func NewTLSWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
	syslogConverter *Converter,
	appLogClient v2.LogClient,
) egress.WriteCloser {

	dialer := checkedDialer(netConf)

	df := func(addr string) (net.Conn, error) {
		return tls.DialWithDialer(dialer, "tcp", addr, tlsConf)
	}

	w := &TLSWriter{
		TCPWriter{
			url:             binding.URL,
			appID:           binding.AppID,
			hostname:        binding.Hostname,
			writeTimeout:    netConf.WriteTimeout,
			dialFunc:        df,
			scheme:          "syslog-tls",
			egressMetric:    egressMetric,
			syslogConverter: syslogConverter,
			appLogClient:    appLogClient,
		},
	}

	return w
}
