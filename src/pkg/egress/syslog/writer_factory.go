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

type WriterKind int

const (
	Https WriterKind = iota
	Syslog
	SyslogTLS
	Unsupported
)

type WriterFactoryError struct {
	Kind    WriterKind
	Message string
	URL     *url.URL
}

func (e WriterFactoryError) StringKind() string {
	switch e.Kind {
	case Https:
		return "https"
	case Syslog:
		return "syslog"
	case SyslogTLS:
		return "syslogTLS"
	case Unsupported:
		return "unsupported protocol"
	}
	return "error"
}

func (e WriterFactoryError) Error() string {
	return fmt.Sprintf("%s: %s, binding: %q", e.StringKind(), e.Message, e.URL)
}

func NewWriterFactoryErrorf(kind WriterKind, bindingURL *url.URL, format string, a ...any) error {
	return WriterFactoryError{
		Kind:    kind,
		URL:     bindingURL,
		Message: fmt.Sprintf(format, a...),
	}
}

type WriterFactory struct {
	internalTlsConfig *tls.Config
	externalTlsConfig *tls.Config
	egressMetric      metrics.Counter
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

func (f WriterFactory) NewWriter(
	urlBinding *URLBinding,
) (egress.WriteCloser, error) {
	var o []ConverterOption
	if urlBinding.OmitMetadata {
		o = append(o, WithoutSyslogMetadata())
	}
	tlsConfig := f.externalTlsConfig
	if urlBinding.InternalTls {
		tlsConfig = f.internalTlsConfig
	}
	converter := NewConverter(o...)
	tlsClonedConfig := tlsConfig.Clone()
	if len(urlBinding.Certificate) > 0 && len(urlBinding.PrivateKey) > 0 {
		credentials, err := tls.X509KeyPair(urlBinding.Certificate, urlBinding.PrivateKey)
		if err != nil {
			err = NewWriterFactoryErrorf(SyslogTLS, urlBinding.URL, "failed to load certificate: %s", err.Error())
			return nil, err
		}
		tlsClonedConfig.Certificates = []tls.Certificate{credentials}
	}
	if len(urlBinding.CA) > 0 {
		ok := tlsClonedConfig.RootCAs.AppendCertsFromPEM(urlBinding.CA)
		if !ok {
			err := NewWriterFactoryErrorf(SyslogTLS, urlBinding.URL, "failed to load root ca")
			return nil, err
		}
	}

	drainScope := "app"
	if urlBinding.AppID == "" {
		drainScope = "aggregate"
	}

	f.egressMetric = f.m.NewCounter(
		"egress",
		"Total number of envelopes successfully egressed.",
		metrics.WithMetricLabels(map[string]string{
			"direction":   "egress",
			"drain_scope": drainScope,
			"drain_url":   urlBinding.URL.String(),
		}),
	)

	var w egress.WriteCloser
	switch urlBinding.URL.Scheme {
	case "https":
		w = NewHTTPSWriter(
			urlBinding,
			f.netConf,
			tlsClonedConfig,
			f.egressMetric,
			converter,
		)
	case "syslog":
		w = NewTCPWriter(
			urlBinding,
			f.netConf,
			f.egressMetric,
			converter,
		)
	case "syslog-tls":
		w = NewTLSWriter(
			urlBinding,
			f.netConf,
			tlsClonedConfig,
			f.egressMetric,
			converter,
		)
	}

	if w == nil {
		return nil, NewWriterFactoryErrorf(Unsupported, urlBinding.URL, "%q", urlBinding.URL.Scheme)
	}

	return NewRetryWriter(
		urlBinding,
		ExponentialDuration,
		maxRetries,
		w,
	)
}
