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
		b.LogFilter = getLogFilter(urlParsed)

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

// Parse HTML query parameter into a Set of LogTypes
func NewLogTypeSet(logTypeList string, isExclude bool) *syslog.LogTypeSet {
	if logTypeList == "" {
		set := make(syslog.LogTypeSet)
		return &set
	}

	logTypes := strings.Split(logTypeList, ",")
	set := make(syslog.LogTypeSet, len(logTypes))

	for _, logType := range logTypes {
		logType = strings.ToLower(strings.TrimSpace(logType))
		var t syslog.LogType
		switch logType {
		case "app":
			t = syslog.APP
		case "stg":
			t = syslog.STG
		case "rtr":
			t = syslog.RTR
		case "api":
			t = syslog.API
		case "lgr":
			t = syslog.LGR
		case "ssh":
			t = syslog.SSH
		case "cell":
			t = syslog.CELL
		default:
			// TODO Log unknown log type
			continue
		}
		set[t] = struct{}{}
	}

	if isExclude {
		// Invert the set
		fullSet := make(syslog.LogTypeSet)
		allLogTypes := []syslog.LogType{syslog.API, syslog.STG, syslog.RTR, syslog.LGR, syslog.APP, syslog.SSH, syslog.CELL}
		for _, t := range allLogTypes {
			fullSet[t] = struct{}{}
		}

		for t := range set {
			delete(fullSet, t)
		}
		return &fullSet
	}

	return &set
}

func getLogFilter(u *url.URL) *syslog.LogTypeSet {
	includeLogTypes := u.Query().Get("include-log-types")
	excludeLogTypes := u.Query().Get("exclude-log-types")

	if excludeLogTypes != "" && includeLogTypes != "" {
		// TODO return errors.New("include-log-types and exclude-log-types can not be used at the same time")
	} else if excludeLogTypes != "" {
		return NewLogTypeSet(excludeLogTypes, true)
	} else if includeLogTypes != "" {
		return NewLogTypeSet(includeLogTypes, false)
	}
	return NewLogTypeSet("", false)
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
