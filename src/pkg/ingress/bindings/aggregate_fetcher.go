package bindings

import (
	"net/url"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type CacheFetcher interface {
	GetAggregate() ([]binding.LegacyBinding, error)
}

type AggregateDrainFetcher struct {
	bindings []syslog.Binding
	cf       CacheFetcher
}

func NewAggregateDrainFetcher(bindings []string, cf CacheFetcher) *AggregateDrainFetcher {
	drainFetcher := &AggregateDrainFetcher{cf: cf}
	parsedDrains := parseBindings(bindings)
	drainFetcher.bindings = parsedDrains
	return drainFetcher
}

func (a *AggregateDrainFetcher) FetchBindings() ([]syslog.Binding, error) {
	if len(a.bindings) != 0 {
		var bindings []syslog.Binding
		bindings = append(bindings, a.bindings...)
		return bindings, nil
	} else if a.cf != nil {
		aggregate, err := a.cf.GetAggregate()
		if err != nil {
			return []syslog.Binding{}, err
		}
		syslogBindings := []syslog.Binding{}
		for _, i := range aggregate {
			syslogBindings = append(syslogBindings, parseBindings(i.Drains)...)
		}
		return syslogBindings, nil
	} else {
		return []syslog.Binding{}, nil
	}
}

func parseBindings(urls []string) []syslog.Binding {
	syslogBindings := []syslog.Binding{}
	for _, b := range urls {
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
			Drain: syslog.Drain{Url: b},
			Type:  bindingType,
		}
		syslogBindings = append(syslogBindings, binding)
	}
	return syslogBindings
}

func (a *AggregateDrainFetcher) DrainLimit() int {
	return -1
}
