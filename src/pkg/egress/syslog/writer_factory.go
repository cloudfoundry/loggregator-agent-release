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
	tlsConfig    *tls.Config
	egressMetric metrics.Counter
	netConf      NetworkTimeoutConfig
}

func NewWriterFactory(tlsConf *tls.Config, netConf NetworkTimeoutConfig, m metricClient) WriterFactory {

	metric := m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
	)
	return WriterFactory{
		tlsConfig:    tlsConf,
		egressMetric: metric,
		netConf:      netConf,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
) (egress.WriteCloser, error) {
	var o []ConverterOption
	if urlBinding.OmitMetadata == true {
		o = append(o, WithoutSyslogMetadata())
	}
	converter := NewConverter(o...)

	var w egress.WriteCloser
	var err error
	switch urlBinding.URL.Scheme {
	case "https":
		w, err = NewHTTPSWriter(
			urlBinding,
			f.netConf,
			f.tlsConfig,
			f.egressMetric,
			converter,
		), nil
	case "syslog":
		w, err = NewTCPWriter(
			urlBinding,
			f.netConf,
			f.egressMetric,
			converter,
		), nil
	case "syslog-tls":
		w, err = NewTLSWriter(
			urlBinding,
			f.netConf,
			f.tlsConfig,
			f.egressMetric,
			converter,
		), nil
	}

	if w == nil {
		return nil, errors.New("unsupported protocol")
	}

	if err != nil {
		return nil, err
	}

	return NewRetryWriter(
		urlBinding,
		ExponentialDuration,
		maxRetries,
		w,
	)
}
