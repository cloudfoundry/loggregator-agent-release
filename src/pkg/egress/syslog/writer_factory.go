package syslog

import (
	"crypto/tls"
	"fmt"
	"net/url"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type metricClient interface {
	NewCounter(name, helpText string, o ...metrics.MetricOption) metrics.Counter
}

type WriterFactoryError struct {
	Message string
	URL     *url.URL
}

func NewWriterFactoryErrorf(u *url.URL, format string, a ...any) error {
	return WriterFactoryError{
		URL:     u,
		Message: fmt.Sprintf(format, a...),
	}
}

func (e WriterFactoryError) anonymizedURL() *url.URL {
	u := *e.URL
	u.User = nil
	u.RawQuery = ""
	return &u
}

func (e WriterFactoryError) Error() string {
	return fmt.Sprintf("%q: %s", e.anonymizedURL(), e.Message)
}

type WriterFactory struct {
	internalTlsConfig *tls.Config
	externalTlsConfig *tls.Config
	netConf           NetworkTimeoutConfig
	m                 metricClient
}

func NewWriterFactory(internalTlsConfig *tls.Config, externalTlsConfig *tls.Config, netConf NetworkTimeoutConfig, m metricClient) WriterFactory {
	return WriterFactory{
		internalTlsConfig: internalTlsConfig,
		externalTlsConfig: externalTlsConfig,
		netConf:           netConf,
		m:                 m,
	}
}

func (f WriterFactory) NewWriter(ub *URLBinding) (egress.WriteCloser, error) {
	tlsCfg := f.externalTlsConfig.Clone()
	if ub.InternalTls {
		tlsCfg = f.internalTlsConfig.Clone()
	}
	if len(ub.Certificate) > 0 && len(ub.PrivateKey) > 0 {
		cert, err := tls.X509KeyPair(ub.Certificate, ub.PrivateKey)
		if err != nil {
			err = NewWriterFactoryErrorf(ub.URL, "failed to load certificate: %s", err.Error())
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	if len(ub.CA) > 0 {
		ok := tlsCfg.RootCAs.AppendCertsFromPEM(ub.CA)
		if !ok {
			err := NewWriterFactoryErrorf(ub.URL, "failed to load root CA")
			return nil, err
		}
	}

	drainScope := "app"
	if ub.AppID == "" {
		drainScope = "aggregate"
	}
	anonymousURL := *ub.URL
	anonymousURL.User = nil
	anonymousURL.RawQuery = ""
	egressMetric := f.m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
		metrics.WithMetricLabels(map[string]string{
			"direction":   "egress",
			"drain_scope": drainScope,
			"drain_url":   anonymousURL.String(),
		}),
	)

	var o []ConverterOption
	if ub.OmitMetadata {
		o = append(o, WithoutSyslogMetadata())
	}
	converter := NewConverter(o...)

	var w egress.WriteCloser
	switch ub.URL.Scheme {
	case "https":
		w = NewHTTPSWriter(
			ub,
			f.netConf,
			tlsCfg,
			egressMetric,
			converter,
		)
	case "syslog":
		w = NewTCPWriter(
			ub,
			f.netConf,
			egressMetric,
			converter,
		)
	case "syslog-tls":
		w = NewTLSWriter(
			ub,
			f.netConf,
			tlsCfg,
			egressMetric,
			converter,
		)
	}

	if w == nil {
		return nil, NewWriterFactoryErrorf(ub.URL, "unsupported protocol: %q", ub.URL.Scheme)
	}

	return NewRetryWriter(
		ub,
		ExponentialDuration,
		maxRetries,
		w,
	)
}
