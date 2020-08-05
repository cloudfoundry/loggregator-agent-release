package syslog

import (
	"crypto/tls"
	"errors"
	"fmt"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type metricClient interface {
	NewCounter(name, helpText string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactory struct {
	internalTlsConfig *tls.Config
	externalTlsConfig *tls.Config
	egressMetric      metrics.Counter
	netConf           NetworkTimeoutConfig
}

func NewWriterFactory(internalTlsConfig *tls.Config, externalTlsConfig *tls.Config, netConf NetworkTimeoutConfig, m metricClient) WriterFactory {
	metric := m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
	)
	return WriterFactory{
		internalTlsConfig: internalTlsConfig,
		externalTlsConfig: externalTlsConfig,
		egressMetric:      metric,
		netConf:           netConf,
	}
}

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
) (egress.WriteCloser, error) {
	var o []ConverterOption
	if urlBinding.OmitMetadata == true {
		o = append(o, WithoutSyslogMetadata())
	}
	tlsConfig := f.externalTlsConfig
	if urlBinding.InternalTls == true {
		tlsConfig = f.internalTlsConfig
	}
	converter := NewConverter(o...)

	var err error
	var w egress.WriteCloser
	switch urlBinding.URL.Scheme {
	case "https":
		w, err = NewHTTPSWriter(
			urlBinding,
			f.netConf,
			tlsConfig,
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
			tlsConfig,
			f.egressMetric,
			converter,
		), nil
	}

	if w == nil {
		return nil, errors.New(fmt.Sprintf("unsupported protocol: %v", urlBinding.URL.Scheme))
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
