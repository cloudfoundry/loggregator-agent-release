package syslog

import (
	"code.cloudfoundry.org/go-metric-registry"
	"crypto/tls"
	"errors"

	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
)

type metricClient interface {
	NewCounter(name, helpText string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactory struct {
	tlsConfig    *tls.Config

	egressMetric metrics.Counter
}

func NewWriterFactory(tlsConf *tls.Config, m metricClient) WriterFactory {
	metric := m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
	)
	return WriterFactory{
		tlsConfig:    tlsConf,
		egressMetric: metric,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
	netConf NetworkTimeoutConfig,
) (egress.WriteCloser, error) {
	switch urlBinding.URL.Scheme {
	case "https":
		return NewHTTPSWriter(
			urlBinding,
			netConf,
			f.tlsConfig,
			f.egressMetric,
		), nil
	case "syslog":
		return NewTCPWriter(
			urlBinding,
			netConf,
			f.egressMetric,
		), nil
	case "syslog-tls":
		return NewTLSWriter(
			urlBinding,
			netConf,
			f.tlsConfig,
			f.egressMetric,
		), nil
	default:
		return nil, errors.New("unsupported protocol")
	}
}
