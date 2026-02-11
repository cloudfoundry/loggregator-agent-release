package bindings

import (
	"net/url"
	"strings"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type DrainParamParser struct {
	fetcher              binding.Fetcher
	defaultDrainMetadata bool
}

func NewDrainParamParser(f binding.Fetcher, defaultDrainMetadata bool) *DrainParamParser {
	return &DrainParamParser{
		fetcher:              f,
		defaultDrainMetadata: defaultDrainMetadata,
	}
}

func (d *DrainParamParser) FetchBindings() ([]syslog.Binding, error) {
	var processed []syslog.Binding
	bs, err := d.fetcher.FetchBindings()
	if err != nil {
		return nil, err
	}

	for _, b := range bs {
		urlParsed, err := url.Parse(b.Drain.Url)
		if err != nil {
			continue
		}

		b.OmitMetadata = getOmitMetadata(urlParsed, d.defaultDrainMetadata)
		b.InternalTls = getInternalTLS(urlParsed)
		b.DrainData = getBindingType(urlParsed)
		b.LogFilter = d.getLogFilter(urlParsed)

		processed = append(processed, b)
	}

	return processed, nil
}

func getInternalTLS(url *url.URL) bool {
	return url.Query().Get("ssl-strict-internal") == "true"
}

func getOmitMetadata(url *url.URL, defaultDrainMetadata bool) bool {
	if defaultDrainMetadata && getRemoveMetadataQuery(url) == "true" {
		return true
	} else if !defaultDrainMetadata && getRemoveMetadataQuery(url) == "false" {
		return false
	} else {
		return !defaultDrainMetadata
	}
}

func getBindingType(u *url.URL) syslog.DrainData {
	drainData := syslog.LOGS
	switch u.Query().Get("drain-type") {
	case "logs":
		drainData = syslog.LOGS_NO_EVENTS
	case "metrics":
		drainData = syslog.METRICS
	case "all":
		drainData = syslog.LOGS_AND_METRICS
	}

	switch u.Query().Get("drain-data") {
	case "logs":
		drainData = syslog.LOGS
	case "metrics":
		drainData = syslog.METRICS
	case "traces":
		drainData = syslog.TRACES
	case "all":
		drainData = syslog.ALL
	}

	if u.Query().Get("include-metrics-deprecated") != "" {
		drainData = syslog.ALL
	}
	return drainData
}

func (d *DrainParamParser) getLogFilter(u *url.URL) *syslog.LogFilter {
	includeSourceTypes := u.Query().Get("include-source-types")
	excludeSourceTypes := u.Query().Get("exclude-source-types")

	if excludeSourceTypes != "" {
		return d.newLogFilter(excludeSourceTypes, syslog.LogFilterModeExclude)
	} else if includeSourceTypes != "" {
		return d.newLogFilter(includeSourceTypes, syslog.LogFilterModeInclude)
	}
	return nil
}

// newLogFilter parses a URL query parameter into a LogFilter.
// sourceTypeList is assumed to be a comma-separated list of valid source types.
func (d *DrainParamParser) newLogFilter(sourceTypeList string, mode syslog.LogFilterMode) *syslog.LogFilter {
	if sourceTypeList == "" {
		return nil
	}

	sourceTypes := strings.Split(sourceTypeList, ",")
	set := make(syslog.SourceTypeSet, len(sourceTypes))

	for _, sourceType := range sourceTypes {
		sourceType = strings.TrimSpace(sourceType)
		t, _ := syslog.ParseSourceType(sourceType)
		set.Add(t)
	}

	return syslog.NewLogFilter(set, mode)
}

func getRemoveMetadataQuery(u *url.URL) string {
	q := u.Query().Get("disable-metadata")
	if q == "" {
		q = u.Query().Get("omit-metadata")
	}
	return q
}

func (d *DrainParamParser) DrainLimit() int {
	return -1
}
