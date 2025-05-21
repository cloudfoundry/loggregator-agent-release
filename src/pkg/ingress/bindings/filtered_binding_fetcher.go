package bindings

import (
	"log"
	"net"
	"net/url"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/simplecache"
)

var allowedSchemes = []string{"syslog", "syslog-tls", "https", "https-batch"}

//counterfeiter:generate . IPChecker
type IPChecker interface {
	ResolveAddr(host string) (net.IP, error)
	CheckBlacklist(ip net.IP) error
}

// Metrics is the client used to expose gauge and counter metricsClient.
type metricsClient interface {
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
}

type FilteredBindingFetcher struct {
	ipChecker         IPChecker
	br                binding.Fetcher
	warn              bool
	logger            *log.Logger
	invalidDrains     metrics.Gauge
	blacklistedDrains metrics.Gauge
	failedHostsCache  *simplecache.SimpleCache[string, bool]
}

func NewFilteredBindingFetcher(c IPChecker, b binding.Fetcher, m metricsClient, warn bool, lc *log.Logger) *FilteredBindingFetcher {
	opt := metrics.WithMetricLabels(map[string]string{"unit": "total"})

	invalidDrains := m.NewGauge(
		"invalid_drains",
		"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
		opt,
	)
	blacklistedDrains := m.NewGauge(
		"blacklisted_drains",
		"Count of blacklisted drains encountered in last binding fetch.",
		opt,
	)
	return &FilteredBindingFetcher{
		ipChecker:         c,
		br:                b,
		warn:              warn,
		logger:            lc,
		invalidDrains:     invalidDrains,
		blacklistedDrains: blacklistedDrains,
		failedHostsCache:  simplecache.New[string, bool](120 * time.Second),
	}
}

func (f FilteredBindingFetcher) DrainLimit() int {
	return f.br.DrainLimit()
}

func (f *FilteredBindingFetcher) FetchBindings() ([]syslog.Binding, error) {
	sourceBindings, err := f.br.FetchBindings()
	if err != nil {
		return nil, err
	}
	newBindings := []syslog.Binding{}

	var invalidDrains float64
	var blacklistedDrains float64
	for _, b := range sourceBindings {
		u, err := url.Parse(b.Drain.Url)
		if err != nil {
			invalidDrains += 1
			f.printWarning("Cannot parse syslog drain url for application %s", b.AppId)
			continue
		}

		anonymousUrl := u
		anonymousUrl.User = nil
		anonymousUrl.RawQuery = ""

		if invalidScheme(u.Scheme) {
			f.printWarning("Invalid scheme %s in syslog drain url %s for application %s", u.Scheme, anonymousUrl.String(), b.AppId)
			continue
		}

		if len(u.Host) == 0 {
			invalidDrains += 1
			f.printWarning("No hostname found in syslog drain url %s for application %s", anonymousUrl.String(), b.AppId)
			continue
		}

		_, exists := f.failedHostsCache.Get(u.Host)
		if exists {
			invalidDrains += 1
			f.printWarning("Skipped resolve ip address for syslog drain with url %s for application %s due to prior failure", anonymousUrl.String(), b.AppId)
			continue
		}

		ip, err := f.ipChecker.ResolveAddr(u.Host)
		if err != nil {
			invalidDrains += 1
			f.failedHostsCache.Set(u.Host, true)
			f.printWarning("Cannot resolve ip address for syslog drain with url %s for application %s", anonymousUrl.String(), b.AppId)
			continue
		}

		err = f.ipChecker.CheckBlacklist(ip)
		if err != nil {
			invalidDrains += 1
			blacklistedDrains += 1
			f.printWarning("Resolved ip address for syslog drain with url %s for application %s is blacklisted", anonymousUrl.String(), b.AppId)
			continue
		}

		newBindings = append(newBindings, b)
	}

	f.blacklistedDrains.Set(blacklistedDrains)
	f.invalidDrains.Set(invalidDrains)
	return newBindings, nil
}

func (f FilteredBindingFetcher) printWarning(format string, v ...any) {
	if f.warn {
		f.logger.Printf(format, v...)
	}
}

func invalidScheme(scheme string) bool {
	for _, s := range allowedSchemes {
		if s == scheme {
			return false
		}
	}

	return true
}
