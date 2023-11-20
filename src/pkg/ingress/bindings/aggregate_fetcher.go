package bindings

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type CacheFetcher interface {
	GetAggregate() ([]binding.Binding, error)
}

type AggregateDrainFetcher struct {
	bindings []syslog.Binding
	cf       CacheFetcher
}

func NewAggregateDrainFetcher(bindings []string, cf CacheFetcher) *AggregateDrainFetcher {
	drainFetcher := &AggregateDrainFetcher{cf: cf}
	parsedDrains := constructBindings(bindings)
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
			if i.Url == "" {
				continue
			}
			b := syslog.Binding{
				AppId: "",
				Drain: syslog.Drain{Url: i.Url},
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

func constructBindings(urls []string) []syslog.Binding {
	syslogBindings := []syslog.Binding{}
	for _, u := range urls {
		if u == "" {
			continue
		}
		binding := syslog.Binding{
			AppId: "",
			Drain: syslog.Drain{Url: u},
		}
		syslogBindings = append(syslogBindings, binding)
	}
	return syslogBindings
}

func (a *AggregateDrainFetcher) DrainLimit() int {
	return -1
}
