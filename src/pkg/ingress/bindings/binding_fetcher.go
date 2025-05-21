package bindings

import (
	"log"
	"math"
	"sort"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

// Metrics is the client used to expose gauge and counter metricsClient.
type Metrics interface {
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

//go:generate go tool counterfeiter -generate

// Getter is configured to fetch HTTP responses
//counterfeiter:generate . Getter
type Getter interface {
	Get() ([]binding.Binding, error)
}

// BindingFetcher uses a Getter to fetch and decode Bindings
type BindingFetcher struct {
	refreshCount metrics.Counter
	maxLatency   metrics.Gauge
	limit        int
	getter       Getter
	logger       *log.Logger
}

// NewBindingFetcher returns a new BindingFetcher
func NewBindingFetcher(limit int, g Getter, m Metrics, logger *log.Logger) *BindingFetcher {
	refreshCount := m.NewCounter(
		"binding_refresh_count",
		"Total number of binding refresh attempts made to the binding provider.",
	)

	//TODO change to histogram
	maxLatency := m.NewGauge(
		"latency_for_last_binding_refresh",
		"Latency in milliseconds of the last binding fetch made to the binding provider.",
		metrics.WithMetricLabels(map[string]string{"unit": "ms"}),
	)
	return &BindingFetcher{
		limit:        limit,
		getter:       g,
		refreshCount: refreshCount,
		maxLatency:   maxLatency,
		logger:       logger,
	}
}

// FetchBindings reaches out to the syslog drain binding provider via the Getter and decodes
// the response. If it does not get a 200, it returns an error.
func (f *BindingFetcher) FetchBindings() ([]syslog.Binding, error) {
	var latency int64
	defer func() {
		f.refreshCount.Add(1)
		f.maxLatency.Set(toMilliseconds(latency))
	}()

	start := time.Now()
	bindings, err := f.getter.Get()
	if err != nil {
		f.logger.Printf("fetching v2/bindings failed: %s", err)
		return nil, err
	}
	latency = time.Since(start).Nanoseconds()
	return f.toSyslogBindings(bindings, f.limit), nil
}

func (f *BindingFetcher) DrainLimit() int {
	return f.limit
}

type ByUrl []syslog.Drain

func (b ByUrl) Len() int           { return len(b) }
func (b ByUrl) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByUrl) Less(i, j int) bool { return b[i].Url < b[j].Url }

type mold struct {
	drains   []syslog.Drain
	hostname string
}

func (f *BindingFetcher) remodelBindings(bs []binding.Binding) map[string]mold {
	remodel := make(map[string]mold)
	for _, b := range bs {
		for _, c := range b.Credentials {
			for _, a := range c.Apps {
				if val, ok := remodel[a.AppID]; ok {
					drain := syslog.Drain{Url: b.Url, Credentials: syslog.Credentials{Cert: c.Cert, Key: c.Key, CA: c.CA}}
					remodel[a.AppID] = mold{drains: append(val.drains, drain), hostname: a.Hostname}
				} else {
					drain := syslog.Drain{Url: b.Url, Credentials: syslog.Credentials{Cert: c.Cert, Key: c.Key, CA: c.CA}}
					remodel[a.AppID] = mold{drains: []syslog.Drain{drain}, hostname: a.Hostname}
				}
			}
		}
	}
	return remodel
}

func (f *BindingFetcher) toSyslogBindings(bs []binding.Binding, perAppLimit int) []syslog.Binding {
	var bindings []syslog.Binding

	remodel := f.remodelBindings(bs)
	for appID, b := range remodel {

		drains := b.drains
		sort.Sort(ByUrl(drains))

		if perAppLimit < len(drains) {
			drains = drains[:perAppLimit]
		}

		for _, d := range drains {

			binding := syslog.Binding{
				AppId:    appID,
				Hostname: b.hostname,
				Drain:    d,
			}
			bindings = append(bindings, binding)
		}
	}

	return bindings
}

// toMilliseconds truncates the calculated milliseconds float to microsecond
// precision.
func toMilliseconds(num int64) float64 {
	millis := float64(num) / float64(time.Millisecond)
	microsPerMilli := 1000.0
	return roundFloat64(millis*microsPerMilli) / microsPerMilli
}

func roundFloat64(num float64) float64 {
	return float64(int(num + math.Copysign(0.5, num)))
}
