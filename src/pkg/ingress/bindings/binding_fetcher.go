package bindings

import (
	"math"
	"net/url"
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

// Getter is configured to fetch HTTP responses
type Getter interface {
	Get() ([]binding.Binding, error)
	LegacyGet() ([]binding.LegacyBinding, error)
}

// BindingFetcher uses a Getter to fetch and decode Bindings
type BindingFetcher struct {
	refreshCount    metrics.Counter
	maxLatency      metrics.Gauge
	limit           int
	getter          Getter
	legacyBehaviour bool
}

// NewBindingFetcher returns a new BindingFetcher
func NewBindingFetcher(limit int, g Getter, m Metrics, legacyBehaviour bool) *BindingFetcher {
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
		limit:           limit,
		getter:          g,
		refreshCount:    refreshCount,
		maxLatency:      maxLatency,
		legacyBehaviour: legacyBehaviour,
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
	var syslogBindings []syslog.Binding
	if f.legacyBehaviour {
		bindings, err := f.getter.LegacyGet()
		if err != nil {
			return nil, err
		}
		latency = time.Since(start).Nanoseconds()
		syslogBindings = f.legacyToSyslogBindings(bindings, f.limit)
	} else {
		bindings, err := f.getter.Get()
		if err != nil {
			return nil, err
		}
		latency = time.Since(start).Nanoseconds()
		syslogBindings = f.toSyslogBindings(bindings, f.limit)
	}

	return syslogBindings, nil
}

func (f *BindingFetcher) DrainLimit() int {
	return f.limit
}

type ByUrl []syslog.Drain

func (b ByUrl) Len() int           { return len(b) }
func (b ByUrl) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByUrl) Less(i, j int) bool { return b[i].Url < b[j].Url }

type mold struct {
	Drains   []syslog.Drain
	hostname string
}

func (f *BindingFetcher) RemodelBindings(bs []binding.Binding) map[string]mold {
	remodel := make(map[string]mold)
	for _, b := range bs {
		for _, a := range b.Apps {
			if val, ok := remodel[a.AppID]; ok {
				drain := syslog.Drain{Url: b.Url, Credentials: syslog.Credentials{Cert: b.Cert, Key: b.Key}}
				remodel[a.AppID] = mold{Drains: append(val.Drains, drain), hostname: a.Hostname}
			} else {
				drain := syslog.Drain{Url: b.Url, Credentials: syslog.Credentials{Cert: b.Cert, Key: b.Key}}
				remodel[a.AppID] = mold{Drains: []syslog.Drain{drain}, hostname: a.Hostname}
			}
		}
	}
	return remodel
}

func (f *BindingFetcher) toSyslogBindings(bs []binding.Binding, perAppLimit int) []syslog.Binding {
	var bindings []syslog.Binding

	remodel := f.RemodelBindings(bs)
	for appID, b := range remodel {

		drains := b.Drains
		sort.Sort(ByUrl(drains))

		if perAppLimit < len(drains) {
			drains = drains[:perAppLimit]
		}

		for _, d := range drains {
			u, err := url.Parse(d.Url)
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
				AppId:    appID,
				Hostname: b.hostname,
				Drain:    d,
				Type:     t,
			}
			bindings = append(bindings, binding)
		}
	}

	return bindings
}

func (f *BindingFetcher) legacyToSyslogBindings(bs []binding.LegacyBinding, perAppLimit int) []syslog.Binding {
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
				Drain:    syslog.Drain{Url: u.String(), Credentials: syslog.Credentials{Cert: "", Key: ""}},
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
