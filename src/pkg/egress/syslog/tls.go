package syslog

import (
	"crypto/tls"
	"net"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/applog"
)

// TLSWriter represents a syslog writer that connects over unencrypted TCP.
type TLSWriter struct {
	TCPWriter
}

// NetworkTimeoutConfig stores various timeout values.
type NetworkTimeoutConfig struct {
	Keepalive    time.Duration
	DialTimeout  time.Duration
	WriteTimeout time.Duration
}

func NewTLSWriter(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	tlsConf *tls.Config,
	egressMetric metrics.Counter,
	syslogConverter *Converter,
	emitter applog.LogEmitter,
) egress.WriteCloser {

	dialer := &net.Dialer{
		Timeout:   netConf.DialTimeout,
		KeepAlive: netConf.Keepalive,
	}

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
			emitter:         emitter,
		},
	}

	return w
}
