package binding

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Poller struct {
	apiClient       client
	pollingInterval time.Duration
	store           Setter
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

func NewPoller(ac client, pi time.Duration, s Setter) *Poller {
	p := &Poller{
		apiClient:       ac,
		pollingInterval: pi,
		store:           s,
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
			log.Printf("failed to get id %d from CUPS Provider: %s", nextID, err)
			return
		}
		var aResp apiResponse
		err = json.NewDecoder(resp.Body).Decode(&aResp)
		if err != nil {
			log.Printf("failed to decode JSON: %s", err)
			return
		}

		bindings = append(bindings, p.toBindings(aResp)...)
		nextID = aResp.NextID

		if nextID == 0 {
			break
		}
	}
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
