package syslog

import (
	"errors"

	"code.cloudfoundry.org/go-loggregator/metrics"

	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
)

type metricClient interface {
	NewCounter(name string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactory struct {
	egressMetric    metrics.Counter
	defaultMetadata bool
}

func NewWriterFactory(m metricClient, defaultMetadata bool) WriterFactory {
	metric := m.NewCounter("egress")
	return WriterFactory{
		egressMetric:    metric,
		defaultMetadata: defaultMetadata,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
	netConf NetworkTimeoutConfig,
	skipCertVerify bool,
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
			skipCertVerify,
			f.egressMetric,
			converter,
		), nil
	case "syslog":
		return NewTCPWriter(
			urlBinding,
			netConf,
			skipCertVerify,
			f.egressMetric,
			converter,
		), nil
	case "syslog-tls":
		return NewTLSWriter(
			urlBinding,
			netConf,
			skipCertVerify,
			f.egressMetric,
			converter,
		), nil
	default:
		return nil, errors.New("unsupported protocol")
	}
}
