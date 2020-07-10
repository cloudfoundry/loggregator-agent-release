package bindings

import (
	"net/url"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type DrainParamParser struct {
	fetcher binding.Fetcher
}

func NewDrainParamParser(f binding.Fetcher) *DrainParamParser {
	return &DrainParamParser{
		fetcher: f,
	}
}

func (d *DrainParamParser) FetchBindings() ([]syslog.Binding, error) {
	var processed []syslog.Binding
	bs, err := d.fetcher.FetchBindings()
	if err != nil {
		return nil, err
	}

	for _, b := range bs {
		urlParsed, err := url.Parse(b.Drain)
		if err != nil {
			continue
		}

		if urlParsed.Query().Get("disable-metadata") == "true" {
			b.OmitMetadata = true
		}
		processed = append(processed, b)
	}

	return processed, nil
}

func (d *DrainParamParser) DrainLimit() int {
	return -1
}
