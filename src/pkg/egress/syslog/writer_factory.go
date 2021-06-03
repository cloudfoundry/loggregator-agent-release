package syslog

import (
	"crypto/tls"
	"errors"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type metricClient interface {
	NewCounter(name, helpText string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactory struct {
	tlsConfig *tls.Config

	egressMetric    metrics.Counter
	defaultMetadata bool
}

func NewWriterFactory(tlsConf *tls.Config, defaultMetadata bool, m metricClient) WriterFactory {
	metric := m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
	)
	return WriterFactory{
		tlsConfig:       tlsConf,
		egressMetric:    metric,
		defaultMetadata: defaultMetadata,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
	netConf NetworkTimeoutConfig,
) (egress.WriteCloser, error) {
	var o []ConverterOption
	if !f.defaultMetadata == true {
		o = append(o, WithoutSyslogMetadata())
	}
	converter := NewConverter(o...)

	switch urlBinding.URL.Scheme {
	case "https":
		return NewHTTPSWriter(
			urlBinding,
			netConf,
			f.tlsConfig,
			f.egressMetric,
			converter,
		), nil
	case "syslog":
		return NewTCPWriter(
			urlBinding,
			netConf,
			f.egressMetric,
			converter,
		), nil
	case "syslog-tls":
		return NewTLSWriter(
			urlBinding,
			netConf,
			f.tlsConfig,
			f.egressMetric,
			converter,
		), nil
	default:
		return nil, errors.New("unsupported protocol")
	}
}
