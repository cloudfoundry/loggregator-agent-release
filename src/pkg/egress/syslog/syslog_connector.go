package syslog

import (
	"fmt"
	"log"

	"golang.org/x/net/context"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator/v9"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
)

type Binding struct {
	AppId        string      `json:"appId,omitempty"`
	Hostname     string      `json:"hostname,omitempty"`
	Drain        Drain       `json:"drain,omitempty"`
	Type         BindingType `json:"type,omitempty"`
	OmitMetadata bool
	InternalTls  bool
}

type Drain struct {
	Url         string      `json:"url"`
	Credentials Credentials `json:"credentials"`
}

type Credentials struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
	CA   string `json:"ca"`
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
	NewWriter(*URLBinding) (egress.WriteCloser, error)
}

// SyslogConnector creates the various egress syslog writers.
type SyslogConnector struct {
	skipCertVerify bool
	logClient      LogClient
	wg             egress.WaitGroup
	sourceIndex    string
	writerFactory  writerFactory

	metricClient  metricClient
	droppedMetric metrics.Counter
}

// NewSyslogConnector configures and returns a new SyslogConnector.
func NewSyslogConnector(
	skipCertVerify bool,
	wg egress.WaitGroup,
	f writerFactory,
	m metricClient,
	opts ...ConnectorOption,
) *SyslogConnector {
	droppedMetric := m.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "egress"}),
	)

	sc := &SyslogConnector{
		skipCertVerify: skipCertVerify,
		wg:             wg,
		logClient:      nullLogClient{},
		writerFactory:  f,

		metricClient:  m,
		droppedMetric: droppedMetric,
	}
	for _, o := range opts {
		o(sc)
	}
	return sc
}

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
		return nil, err
	}

	writer, err := w.writerFactory.NewWriter(urlBinding)
	if err != nil {
		return nil, err
	}

	anonymousUrl := *urlBinding.URL
	anonymousUrl.User = nil
	anonymousUrl.RawQuery = ""

	drainScope := "app"
	if b.AppId == "" {
		drainScope = "aggregate"
	}

	drainDroppedMetric := w.metricClient.NewCounter(
		"messages_dropped_per_drain",
		"Total number of dropped messages.",
		metrics.WithMetricLabels(map[string]string{
			"direction":   "egress",
			"drain_scope": drainScope,
			"drain_url":   anonymousUrl.String(),
		}),
	)

	dw := egress.NewDiodeWriter(ctx, writer, diodes.AlertFunc(func(missed int) {
		w.droppedMetric.Add(float64(missed))
		drainDroppedMetric.Add(float64(missed))

		w.emitLoggregatorErrorLog(b.AppId, fmt.Sprintf("%d messages lost for application %s in user provided syslog drain with url %s", missed, b.AppId, anonymousUrl.String()))
		w.emitStandardOutErrorLog(b.AppId, urlBinding.Scheme(), anonymousUrl.String(), missed)
	}), w.wg)

	filteredWriter, err := NewFilteringDrainWriter(b, dw)
	if err != nil {
		log.Printf("failed to create filtered writer: %s", err)
		return nil, err
	}

	return filteredWriter, nil
}

func (w *SyslogConnector) emitLoggregatorErrorLog(appID, message string) {
	if appID == "" {
		return
	}
	option := loggregator.WithAppInfo(appID, "LGR", "")
	w.logClient.EmitLog(message, option)

	option = loggregator.WithAppInfo(
		appID,
		"SYS",
		w.sourceIndex,
	)
	w.logClient.EmitLog(message, option)
}
func (w *SyslogConnector) emitStandardOutErrorLog(appID, scheme, url string, missed int) {
	errorAppOrAggregate := fmt.Sprintf("for %s's app drain", appID)
	if appID == "" {
		errorAppOrAggregate = "for aggregate drain"
	}
	log.Printf(
		"Dropped %d %s logs %s with url %s",
		missed, scheme, errorAppOrAggregate, url,
	)
}
