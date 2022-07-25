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

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
}

type client interface {
	Get(int) (*http.Response, error)
}

type App struct {
	Hostname string `json:"hostname"`
	AppID    string `json:"appid"`
}

type Binding struct {
	Url  string `json:"url"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
	Apps []App  `json:"apps"`
}

type Setter interface {
	Set([]Binding)
}

func NewPoller(ac client, pi time.Duration, s Setter, m Metrics, logger *log.Logger) *Poller {
	p := &Poller{
		apiClient:       ac,
		pollingInterval: pi,
		store:           s,
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
	p.lastBindingCount.Set(CalculateBindingCount(bindings))
	p.store.Set(bindings)
}

func CalculateBindingCount(bindings []Binding) float64 {
	apps := make(map[string]bool)
	for _, b := range bindings {
		for _, a := range b.Apps {
			if _, ok := apps[a.AppID]; ok {
				continue
			}
			apps[a.AppID] = true
		}
	}
	return float64(len(apps))
}

type apiResponse struct {
	Results []Binding
	NextID  int `json:"next_id"`
}
