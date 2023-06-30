package bindings

import (
	"net/url"

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

		if d.defaultDrainMetadata && getRemoveMetadataQuery(urlParsed) == "true" {
			b.OmitMetadata = true
		} else if !d.defaultDrainMetadata && getRemoveMetadataQuery(urlParsed) == "false" {
			b.OmitMetadata = false
		} else {
			b.OmitMetadata = !d.defaultDrainMetadata
		}
		if urlParsed.Query().Get("ssl-strict-internal") == "true" {
			b.InternalTls = true
		}

		b.Type = getBindingType(urlParsed)

		processed = append(processed, b)
	}

	return processed, nil
}

func getBindingType(u *url.URL) syslog.BindingType {
	bindingType := syslog.BINDING_TYPE_LOG
	switch u.Query().Get("drain-type") {
	case "metrics":
		bindingType = syslog.BINDING_TYPE_METRIC
	case "all":
		bindingType = syslog.BINDING_TYPE_ALL
	case "allWithTimers":
		bindingType = syslog.BINDING_TYPE_AGGREGATE
	}

	if u.Query().Get("include-metrics-deprecated") != "" {
		bindingType = syslog.BINDING_TYPE_AGGREGATE
	}
	return bindingType
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
