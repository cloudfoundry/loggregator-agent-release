package bindings

import (
	"log"
	"net/url"
	"strings"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/simplecache"
)

var allowedSchemes = []string{"syslog", "syslog-tls", "https", "https-batch"}

type FilteredBindingFetcher struct {
	ipChecker        binding.IPChecker
	br               binding.Fetcher
	warn             bool
	logger           *log.Logger
	failedHostsCache *simplecache.SimpleCache[string, bool]
}

func NewFilteredBindingFetcher(c binding.IPChecker, b binding.Fetcher, warn bool, lc *log.Logger) *FilteredBindingFetcher {
	return &FilteredBindingFetcher{
		ipChecker:        c,
		br:               b,
		warn:             warn,
		logger:           lc,
		failedHostsCache: simplecache.New[string, bool](120 * time.Second),
	}
}

func (f FilteredBindingFetcher) DrainLimit() int {
	return f.br.DrainLimit()
}

// Fetches bindings from the Syslog Binding Cache
// The validation of the binding URL is moved to the poller in the Syslog Binding Cache.
// The validation here is solely to protect the Syslog Agent from tempering of the
// Syslog Binding Cache's API. The Syslog Agent should always get valid bindings from the
// Syslog Binding Cache, but if it happens that a binding is invalid it will write an error
// message to inform the operators that the Syslog Binding Cache API should be checked
func (f *FilteredBindingFetcher) FetchBindings() ([]syslog.Binding, error) {
	sourceBindings, err := f.br.FetchBindings()
	if err != nil {
		return nil, err
	}
	newBindings := []syslog.Binding{}

	var invalidDrains float64
	for _, b := range sourceBindings {
		u, err := url.Parse(b.Drain.Url)
		if err != nil {
			invalidDrains += 1
			continue
		}

		anonymousUrl := *u
		anonymousUrl.User = nil
		anonymousUrl.RawQuery = ""

		if invalidScheme(u.Scheme) {
			invalidDrains += 1
			continue
		}

		if len(u.Host) == 0 {
			invalidDrains += 1
			continue
		}

		if invalidLogFilter(u) {
			invalidDrains += 1
			f.printWarning("include-source-types and exclude-source-types cannot be used at the same time in syslog drain url %s for application %s", anonymousUrl.String(), b.AppId)
			continue
		}

		sourceTypes := getUnknownSourceTypes(u.Query())
		if sourceTypes != nil {
			invalidDrains += 1
			f.printWarning("Unknown source types '%s' in source type filter in syslog drain url %s for application %s", strings.Join(sourceTypes, ", "), anonymousUrl.String(), b.AppId)
			continue
		}

		_, exists := f.failedHostsCache.Get(u.Host)
		if exists {
			invalidDrains += 1
			continue
		}

		ip, err := f.ipChecker.ResolveAddr(u.Host)
		if err != nil {
			invalidDrains += 1
			f.failedHostsCache.Set(u.Host, true)
			continue
		}

		err = f.ipChecker.CheckBlacklist(ip)
		if err != nil {
			invalidDrains += 1
			continue
		}

		newBindings = append(newBindings, b)
	}

	if invalidDrains > 0 {
		f.printWarning("Invalid drains detected in the Syslog Agent. This should not happen. Check your Syslog Binding Cache and its API")
	}

	return newBindings, nil

}

// invalidLogFilter checks if both include-source-types and exclude-source-types
func invalidLogFilter(u *url.URL) bool {
	includeSourceTypes := u.Query().Get("include-source-types")
	excludeSourceTypes := u.Query().Get("exclude-source-types")
	if excludeSourceTypes != "" && includeSourceTypes != "" {
		return true
	}
	return false
}

// assumes only one of include-source-types or exclude-source-types is set
func getUnknownSourceTypes(u url.Values) []string {
	var sourceTypeList string
	includeSourceTypes := u.Get("include-source-types")
	excludeSourceTypes := u.Get("exclude-source-types")

	if includeSourceTypes != "" {
		sourceTypeList = includeSourceTypes
	} else if excludeSourceTypes != "" {
		sourceTypeList = excludeSourceTypes
	} else {
		return nil
	}

	sourceTypes := strings.Split(sourceTypeList, ",")
	var unknownTypes []string

	for _, sourceType := range sourceTypes {
		sourceType = strings.TrimSpace(sourceType)
		_, ok := syslog.ParseSourceType(sourceType)
		if !ok {
			unknownTypes = append(unknownTypes, sourceType)
			continue
		}
	}

	return unknownTypes
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
