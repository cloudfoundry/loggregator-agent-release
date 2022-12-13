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
	LegacyGet(int) (*http.Response, error)
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
	AppID       string   `json:"app_id"`
	Drains      []string `json:"drains"`
	Hostname    string   `json:"hostname"`
	V2Available bool     `json:"v2_available"`
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
		statusCode := resp.StatusCode

		if statusCode != http.StatusOK || err != nil {
			p.logger.Printf("failed to decode JSON: %s, statusCode: %d, falling back to legacy endpoint", err, statusCode)
			p.pollLegacyFallback()
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

func (p *Poller) pollLegacyFallback() {
	nextID := 0
	var legacyBindings []LegacyBinding

	for {
		resp, err := p.apiClient.LegacyGet(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get id %d from legacy CUPS Provider: %s", nextID, err)
			return
		}
		var aRespLegacy legacyApiResponse

		err = json.NewDecoder(resp.Body).Decode(&aRespLegacy)
		if err != nil {
			p.logger.Printf("failed to decode legacy JSON: %s", err)
			return
		}
		if aRespLegacy.V5Available {
			p.logger.Printf("V4 endpoint is deprecated, skipping v4 result parsing.")
			return
		}
		for k, v := range aRespLegacy.Results {
			legacyBindings = append(legacyBindings, LegacyBinding{
				AppID:       k,
				Drains:      v.Drains,
				Hostname:    v.Hostname,
				V2Available: true,
			})
		}
		nextID = aRespLegacy.NextID

		if nextID == 0 {
			break
		}
	}
	bindings := ToBindings(legacyBindings)
	bindingCount := CalculateBindingCount(bindings)
	p.lastBindingCount.Set(float64(bindingCount))
	p.store.Set(bindings, bindingCount)
	p.legacyStore.Set(legacyBindings)
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

func ToBindings(legacyBindings []LegacyBinding) []Binding {
	var bindings []Binding
	var remodel = make(map[string]Credentials)
	for _, lb := range legacyBindings {
		for _, d := range lb.Drains {
			if val, ok := remodel[d]; ok {
				appExists := false
				for _, a := range val.Apps {
					if a.AppID == lb.AppID {
						appExists = true
						break
					}
				}
				if !appExists {
					app := App{AppID: lb.AppID, Hostname: lb.Hostname}
					remodel[d] = Credentials{Cert: "", Key: "", Apps: append(val.Apps, app)}
				}
			} else {
				app := App{AppID: lb.AppID, Hostname: lb.Hostname}
				remodel[d] = Credentials{Cert: "", Key: "", Apps: []App{app}}
			}
		}
	}

	for url, credentials := range remodel {
		binding := Binding{Url: url, Credentials: []Credentials{credentials}}
		bindings = append(bindings, binding)
	}
	return bindings
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
		legacyBinding := LegacyBinding{appID, app.Drains, app.hostname, true}
		legacyBindings = append(legacyBindings, legacyBinding)
	}
	return legacyBindings
}

type apiResponse struct {
	Results []Binding
	NextID  int `json:"next_id"`
}

type legacyApiResponse struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID      int  `json:"next_id"`
	V5Available bool `json:"v5_available"`
}
