package syslog

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"errors"

	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
)

type metricClient interface {
	NewCounter(name string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactory struct {
	egressMetric metrics.Counter
}

func NewWriterFactory(m metricClient) WriterFactory {
	metric := m.NewCounter(
		"egress",
		metrics.WithHelpText("Total number of envelopes successfully egressed."),
	)
	return WriterFactory{
		egressMetric: metric,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
	netConf NetworkTimeoutConfig,
	skipCertVerify bool,
) (egress.WriteCloser, error) {
	switch urlBinding.URL.Scheme {
	case "https":
		return NewHTTPSWriter(
			urlBinding,
			netConf,
			skipCertVerify,
			f.egressMetric,
		), nil
	case "syslog":
		return NewTCPWriter(
			urlBinding,
			netConf,
			skipCertVerify,
			f.egressMetric,
		), nil
	case "syslog-tls":
		return NewTLSWriter(
			urlBinding,
			netConf,
			skipCertVerify,
			f.egressMetric,
		), nil
	default:
		return nil, errors.New("unsupported protocol")
	}
}
