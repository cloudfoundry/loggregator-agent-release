package syslog

import (
	"crypto/tls"
	"fmt"

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
	GenericError
)

type WriterFactoryError struct {
	Kind    WriterKind
	Message string
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
	return fmt.Sprintf("%s: %s", e.StringKind(), e.Message)
}

func NewWriterFactoryError(kind WriterKind, message string) error {
	return WriterFactoryError{
		Kind:    kind,
		Message: message,
	}
}

func NewWriterFactoryErrorf(kind WriterKind, format string, a ...any) error {
	return WriterFactoryError{
		Kind:    kind,
		Message: fmt.Sprintf(format, a...),
	}
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
			err = NewWriterFactoryErrorf(SyslogTLS, "failed to load certificate: %s", err.Error())
			return nil, err
		}
		tlsClonedConfig.Certificates = []tls.Certificate{credentials}
	}
	if len(urlBinding.CA) > 0 {
		ok := tlsClonedConfig.RootCAs.AppendCertsFromPEM(urlBinding.CA)
		if !ok {
			err := NewWriterFactoryErrorf(SyslogTLS, "failed to load root ca for binding")
			return nil, err
		}
	}
	var err error
	var w egress.WriteCloser
	switch urlBinding.URL.Scheme {
	case "https":
		w, err = NewHTTPSWriter(
			urlBinding,
			f.netConf,
			tlsClonedConfig,
			f.egressMetric,
			converter,
		), nil
		if err != nil {
			err = NewWriterFactoryError(Https, err.Error())
		}
	case "syslog":
		w, err = NewTCPWriter(
			urlBinding,
			f.netConf,
			f.egressMetric,
			converter,
		), nil
		if err != nil {
			err = NewWriterFactoryError(Syslog, err.Error())
		}
	case "syslog-tls":
		w, err = NewTLSWriter(
			urlBinding,
			f.netConf,
			tlsClonedConfig,
			f.egressMetric,
			converter,
		), nil
	}

	if w == nil {
		err = NewWriterFactoryError(Unsupported, urlBinding.URL.Scheme)
		return nil, err
	}

	if err != nil {
		err = NewWriterFactoryError(GenericError, err.Error())
		return nil, err
	}

	return NewRetryWriter(
		urlBinding,
		ExponentialDuration,
		maxRetries,
		w,
	)
}
