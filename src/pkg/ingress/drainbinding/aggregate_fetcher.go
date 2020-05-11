package drainbinding

import (
	"fmt"
	"net/url"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type AggregateDrainFetcher struct {
	bindings []syslog.Binding
}

func NewAggregateDrainFetcher(bindings []string) *AggregateDrainFetcher {
	drainFetcher := &AggregateDrainFetcher{}
	for _, b := range bindings {
		if b == "" {
			continue
		}
		bindingType := syslog.BINDING_TYPE_LOG
		urlParsed, err := url.Parse(b)
		if err != nil {
			continue
		}
		if urlParsed.Query().Get("include-metrics-deprecated") != "" {
			bindingType = syslog.BINDING_TYPE_AGGREGATE
		}
		binding := syslog.Binding{
			AppId: "",
			Drain: fmt.Sprintf("%s://%s", urlParsed.Scheme, urlParsed.Host),
			Type:  bindingType,
		}
		drainFetcher.bindings = append(drainFetcher.bindings, binding)
	}
	return drainFetcher
}

func (a *AggregateDrainFetcher) FetchBindings() ([]syslog.Binding, error) {
	var bindingSlice []syslog.Binding
	bindingSlice = append(bindingSlice, a.bindings...)
	return bindingSlice, nil
}

func (a *AggregateDrainFetcher) DrainLimit() int {
	return -1
}
