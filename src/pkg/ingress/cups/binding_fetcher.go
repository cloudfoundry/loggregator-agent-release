package cups

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"math"
	"net/url"
	"sort"
	"time"

	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/syslog"
)

// Metrics is the client used to expose gauge and counter metricsClient.
type Metrics interface {
	NewGauge(name string, opts ...metrics.MetricOption) metrics.Gauge
	NewCounter(name string, opts ...metrics.MetricOption) metrics.Counter
}

// Getter is configured to fetch HTTP responses
type Getter interface {
	Get() ([]binding.Binding, error)
}

// BindingFetcher uses a Getter to fetch and decode Bindings
type BindingFetcher struct {
	refreshCount metrics.Counter
	maxLatency   metrics.Gauge
	limit        int
	getter       Getter
}

// NewBindingFetcher returns a new BindingFetcher
func NewBindingFetcher(limit int, g Getter, m Metrics) *BindingFetcher {
	refreshCount := m.NewCounter(
		"binding_refresh_count",
		metrics.WithHelpText("Total number of binding refresh attempts made to the binding provider."),
	)

	//TODO change to histogram
	maxLatency := m.NewGauge(
		"latency_for_last_binding_refresh",
		metrics.WithHelpText("Latency in milliseconds of the last binding fetch made to the binding provider."),
		metrics.WithMetricTags(map[string]string{"unit": "ms"}),
	)
	return &BindingFetcher{
		limit:        limit,
		getter:       g,
		refreshCount: refreshCount,
		maxLatency:   maxLatency,
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
		return nil, err
	}
	latency = time.Since(start).Nanoseconds()
	syslogBindings := f.toSyslogBindings(bindings, f.limit)

	return syslogBindings, nil
}

func (f *BindingFetcher) DrainLimit() int {
	return f.limit
}

func (f *BindingFetcher) toSyslogBindings(bs []binding.Binding, perAppLimit int) []syslog.Binding {
	var bindings []syslog.Binding
	for _, b := range bs {
		drains := b.Drains
		sort.Strings(drains)

		if perAppLimit < len(drains) {
			drains = drains[:perAppLimit]
		}

		for _, d := range drains {
			u, err := url.Parse(d)
			if err != nil {
				continue
			}

			var t syslog.BindingType
			drainType := u.Query().Get("drain-type")

			switch drainType {
			case "metrics":
				t = syslog.BINDING_TYPE_METRIC
			case "all":
				t = syslog.BINDING_TYPE_ALL
			default:
				t = syslog.BINDING_TYPE_LOG
			}

			binding := syslog.Binding{
				AppId:    b.AppID,
				Hostname: b.Hostname,
				Drain:    u.String(),
				Type:     t,
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
