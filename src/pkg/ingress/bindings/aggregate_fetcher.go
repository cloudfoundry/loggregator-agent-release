package bindings

import (
	"errors"
	"net/url"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type CacheFetcher interface {
	GetAggregate() ([]binding.Binding, error)
	GetLegacyAggregate() ([]binding.LegacyBinding, error)
}

type AggregateDrainFetcher struct {
	bindings []syslog.Binding
	cf       CacheFetcher
}

func NewAggregateDrainFetcher(bindings []string, cf CacheFetcher) *AggregateDrainFetcher {
	drainFetcher := &AggregateDrainFetcher{cf: cf}
	parsedDrains := parseLegacyBindings(bindings)
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
			return a.FetchBindingsLegacyFallback()
		}
		syslogBindings := []syslog.Binding{}
		for _, i := range aggregate {
			b, err := parseUrl(i.Url)
			if err != nil {
				continue
			}
			if len(i.Credentials) > 0 {
				b.Drain.Credentials = syslog.Credentials{
					CA:   i.Credentials[0].CA,
					Cert: i.Credentials[0].Cert,
					Key:  i.Credentials[0].Key,
				}
			}
			syslogBindings = append(syslogBindings, b)
		}
		return syslogBindings, nil
	} else {
		return []syslog.Binding{}, nil
	}
}

func (a *AggregateDrainFetcher) FetchBindingsLegacyFallback() ([]syslog.Binding, error) {
	aggregateLegacy, err := a.cf.GetLegacyAggregate()
	if err != nil {
		return []syslog.Binding{}, err
	}
	syslogBindings := []syslog.Binding{}
	for _, i := range aggregateLegacy {
		if i.V2Available {
			return nil, errors.New("v2 is available")
		}
		syslogBindings = append(syslogBindings, parseLegacyBindings(i.Drains)...)
	}
	return syslogBindings, nil
}

func parseLegacyBindings(urls []string) []syslog.Binding {
	syslogBindings := []syslog.Binding{}
	for _, u := range urls {
		binding, err := parseUrl(u)
		if err != nil {
			continue
		}
		syslogBindings = append(syslogBindings, binding)
	}
	return syslogBindings
}

func parseUrl(u string) (syslog.Binding, error) {
	if u == "" {
		return syslog.Binding{}, errors.New("no url")
	}
	bindingType := syslog.BINDING_TYPE_LOG
	urlParsed, err := url.Parse(u)
	if err != nil {
		return syslog.Binding{}, err
	}
	if urlParsed.Query().Get("include-metrics-deprecated") != "" {
		bindingType = syslog.BINDING_TYPE_AGGREGATE
	}
	binding := syslog.Binding{
		AppId: "",
		Drain: syslog.Drain{Url: u},
		Type:  bindingType,
	}
	return binding, nil
}

func (a *AggregateDrainFetcher) DrainLimit() int {
	return -1
}
