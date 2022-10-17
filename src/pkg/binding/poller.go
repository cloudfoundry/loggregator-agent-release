package binding

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
)

type Poller struct {
	apiClient       client
	pollingInterval time.Duration
	store           Setter
	legacyStore     LegacySetter

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
}

type client interface {
	Get(int) (*http.Response, error)
}

type Credentials struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
	Apps []App  `json:"apps"`
}

type App struct {
	Hostname string `json:"hostname"`
	AppID    string `json:"app_id"`
}

type Binding struct {
	Url         string        `json:"url"`
	Credentials []Credentials `json:"credentials"`
}

type LegacyBinding struct {
	AppID    string   `json:"app_id"`
	Drains   []string `json:"drains"`
	Hostname string   `json:"hostname"`
}

type Setter interface {
	Set(bindings []Binding, bindingCount int)
}

type LegacySetter interface {
	Set([]LegacyBinding)
}

func NewPoller(ac client, pi time.Duration, s Setter, legacyStore LegacySetter, m Metrics, logger *log.Logger) *Poller {
	p := &Poller{
		apiClient:       ac,
		pollingInterval: pi,
		store:           s,
		legacyStore:     legacyStore,
		logger:          logger,
		bindingRefreshErrorCounter: m.NewCounter(
			"binding_refresh_error",
			"Total number of failed requests to the binding provider.",
		),
		lastBindingCount: m.NewGauge(
			"last_binding_refresh_count",
			"Current number of bindings received from binding provider during last refresh.",
		),
	}
	p.poll()
	return p
}

func (p *Poller) Poll() {
	t := time.NewTicker(p.pollingInterval)

	for range t.C {
		p.poll()
	}
}

func (p *Poller) poll() {
	nextID := 0
	var bindings []Binding
	for {
		resp, err := p.apiClient.Get(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			p.logger.Printf("failed to decode JSON: %s", err)
			return
		}

		bindings = append(bindings, aResp.Results...)
		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}

	bindingCount := CalculateBindingCount(bindings)
	p.lastBindingCount.Set(float64(bindingCount))
	p.store.Set(bindings, bindingCount)
	p.legacyStore.Set(ToLegacyBindings(bindings))
}

func CalculateBindingCount(bindings []Binding) int {
	apps := make(map[string]bool)
	for _, b := range bindings {
		for _, c := range b.Credentials {
			for _, a := range c.Apps {
				if _, ok := apps[a.AppID]; ok {
					continue
				}
				apps[a.AppID] = true
			}
		}
	}
	return len(apps)
}

type legacyMold struct {
	Drains   []string
	hostname string
}

func ToLegacyBindings(bindings []Binding) []LegacyBinding {
	var legacyBindings []LegacyBinding
	remodel := make(map[string]legacyMold)
	for _, b := range bindings {
		drain := b.Url
		for _, c := range b.Credentials {
			for _, a := range c.Apps {
				if val, ok := remodel[a.AppID]; ok {
					// This logic prevents duplicate URLs for the same application
					drainExists := false
					for _, existingDrain := range remodel[a.AppID].Drains {
						if drain == existingDrain {
							drainExists = true
							break
						}
					}
					if !drainExists {
						remodel[a.AppID] = legacyMold{Drains: append(val.Drains, drain), hostname: a.Hostname}
					}
				} else {
					remodel[a.AppID] = legacyMold{Drains: []string{drain}, hostname: a.Hostname}
				}
			}
		}
	}

	for appID, app := range remodel {
		legacyBinding := LegacyBinding{appID, app.Drains, app.hostname}
		legacyBindings = append(legacyBindings, legacyBinding)
	}
	return legacyBindings
}

type apiResponse struct {
	Results []Binding
	NextID  int `json:"next_id"`
}
