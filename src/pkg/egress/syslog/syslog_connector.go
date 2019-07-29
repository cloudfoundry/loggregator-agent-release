package syslog

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"fmt"
	"log"
	"time"

	"golang.org/x/net/context"

	"code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
)

type Binding struct {
	AppId    string      `json:"appId,	omitempty"`
	Hostname string      `json:"hostname,omitempty"`
	Drain    string      `json:"drain,	omitempty"`
	Type     BindingType `json:"type,	omitempty"`
}

// LogClient is used to emit logs.
type LogClient interface {
	EmitLog(message string, opts ...loggregator.EmitLogOption)
}

// nullLogClient ensures that the LogClient is in fact optional.
type nullLogClient struct{}

// EmitLog drops all messages into /dev/null.
func (nullLogClient) EmitLog(message string, opts ...loggregator.EmitLogOption) {
}

type writerFactory interface {
	NewWriter(*URLBinding, NetworkTimeoutConfig, bool) (egress.WriteCloser, error)
}

// SyslogConnector creates the various egress syslog writers.
type SyslogConnector struct {
	skipCertVerify bool
	keepalive      time.Duration
	ioTimeout      time.Duration
	dialTimeout    time.Duration
	logClient      LogClient
	wg             egress.WaitGroup
	sourceIndex    string
	writerFactory  writerFactory
	droppedMetric  metrics.Counter
}

// NewSyslogConnector configures and returns a new SyslogConnector.
func NewSyslogConnector(
	netConf NetworkTimeoutConfig,
	skipCertVerify bool,
	wg egress.WaitGroup,
	f writerFactory,
	m metricClient,
	opts ...ConnectorOption,
) *SyslogConnector {
	metric := m.NewCounter("dropped", metrics.WithMetricTags(map[string]string{"direction": "egress"}))

	sc := &SyslogConnector{
		keepalive:      netConf.Keepalive,
		ioTimeout:      netConf.WriteTimeout,
		dialTimeout:    netConf.DialTimeout,
		skipCertVerify: skipCertVerify,
		wg:             wg,
		logClient:      nullLogClient{},
		writerFactory:  f,
		droppedMetric:  metric,
	}
	for _, o := range opts {
		o(sc)
	}
	return sc
}

// WriterConstructor creates syslog connections to https, syslog, and
// syslog-tls drains
type WriterConstructor func(
	binding *URLBinding,
	netConf NetworkTimeoutConfig,
	skipCertVerify bool,
	egressMetric func(uint64),
) egress.WriteCloser

// ConnectorOption allows a syslog connector to be customized.
type ConnectorOption func(*SyslogConnector)

// WithLogClient returns a ConnectorOption that will set up logging for any
// information about a binding.
func WithLogClient(logClient LogClient, sourceIndex string) ConnectorOption {
	return func(sc *SyslogConnector) {
		sc.logClient = logClient
		sc.sourceIndex = sourceIndex
	}
}

// Connect returns an egress writer based on the scheme of the binding drain
// URL.
func (w *SyslogConnector) Connect(ctx context.Context, b Binding) (egress.Writer, error) {
	urlBinding, err := buildBinding(ctx, b)
	if err != nil {
		// Note: the scheduler ensures the URL is valid. It is unlikely that
		// a binding with an invalid URL would make it this far. Nonetheless,
		// we handle the error case all the same.
		w.emitErrorLog(b.AppId, "Invalid syslog drain URL: parse failure")
		return nil, err
	}

	netConf := NetworkTimeoutConfig{
		Keepalive:    w.keepalive,
		DialTimeout:  w.dialTimeout,
		WriteTimeout: w.ioTimeout,
	}

	writer, err := w.writerFactory.NewWriter(urlBinding, netConf, w.skipCertVerify)
	if err != nil {
		return nil, err
	}

	anonymousUrl := *urlBinding.URL
	anonymousUrl.User = nil
	anonymousUrl.RawQuery = ""

	dw := egress.NewDiodeWriter(ctx, writer, diodes.AlertFunc(func(missed int) {
		w.droppedMetric.Add(float64(missed))

		w.emitErrorLog(b.AppId, fmt.Sprintf("%d messages lost in user provided syslog drain", missed))

		log.Printf(
			"Dropped %d %s logs for url %s in app %s",
			missed, urlBinding.Scheme(), anonymousUrl.String(), b.AppId,
		)
	}), w.wg)

	filteredWriter, err := NewFilteringDrainWriter(b, dw)
	if err != nil {
		log.Printf("failed to create filtered writer: %s", err)
		return nil, err
	}

	return filteredWriter, nil
}

func (w *SyslogConnector) emitErrorLog(appID, message string) {
	option := loggregator.WithAppInfo(appID, "LGR", "")
	w.logClient.EmitLog(message, option)

	option = loggregator.WithAppInfo(
		appID,
		"SYS",
		w.sourceIndex,
	)
	w.logClient.EmitLog(message, option)

}
