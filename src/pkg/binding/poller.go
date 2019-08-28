package binding

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"encoding/json"
	"log"
	"net/http"
	"time"
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

type Binding struct {
	AppID    string   `json:"app_id"`
	Drains   []string `json:"drains"`
	Hostname string   `json:"hostname"`
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
			metrics.WithHelpText("Total number of failed requests to the binding provider."),
		),
		lastBindingCount: m.NewGauge(
			"last_binding_refresh_count",
			metrics.WithHelpText("Current number of bindings received from binding provider during last refresh."),
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

		bindings = append(bindings, p.toBindings(aResp)...)
		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}

	p.lastBindingCount.Set(float64(len(bindings)))
	p.store.Set(bindings)
}

func (p *Poller) toBindings(aResp apiResponse) []Binding {
	var bindings []Binding
	for k, v := range aResp.Results {
		bindings = append(bindings, Binding{
			AppID:    k,
			Drains:   v.Drains,
			Hostname: v.Hostname,
		})
	}
	return bindings
}

type apiResponse struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}
