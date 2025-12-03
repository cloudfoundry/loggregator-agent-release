package binding

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

type Poller struct {
	apiClient       client
	pollingInterval time.Duration
	store           Setter

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
	emitter                    syslog.AppLogEmitter
}

type client interface {
	Get(int) (*http.Response, error)
}

type Credentials struct {
	Cert string `json:"cert" yaml:"cert"`
	Key  string `json:"key" yaml:"key"`
	CA   string `json:"ca" yaml:"ca"`
	Apps []App  `json:"apps"`
}

type App struct {
	Hostname string `json:"hostname"`
	AppID    string `json:"app_id"`
}

type Binding struct {
	Url         string        `json:"url" yaml:"url"`
	Credentials []Credentials `json:"credentials" yaml:"credentials"`
}

type AggBinding struct {
	Url  string `yaml:"url"`
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
	CA   string `yaml:"ca"`
}

type Setter interface {
	Set(bindings []Binding, bindingCount int)
}

func NewPoller(ac client, pi time.Duration, s Setter, m Metrics, logger *log.Logger, emitter syslog.AppLogEmitter) *Poller {
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
		emitter: emitter,
	}
	p.poll()
	return p
}

func (p *Poller) Poll() {
	for {
		p.poll()
		time.Sleep(p.pollingInterval)
	}
}

func (p *Poller) poll() {
	nextID := 0
	var bindings []Binding
	for {
		resp, err := p.apiClient.Get(nextID)
		if err != nil {
			p.bindingRefreshErrorCounter.Add(1)
			p.logger.Printf("failed to get page %d from internal bindings endpoint: %s", nextID, err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			p.logger.Printf("unexpected response from internal bindings endpoint. status code: %d", resp.StatusCode)
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

	checkBindings(bindings, p.emitter, p.logger)

	bindingCount := CalculateBindingCount(bindings)
	p.lastBindingCount.Set(float64(bindingCount))
	p.store.Set(bindings, bindingCount)
}

func checkBindings(bindings []Binding, emitter syslog.AppLogEmitter, logger *log.Logger) {
	logger.Printf("checking bindings - found %d bindings", len(bindings))
	for _, b := range bindings {
		u, err := url.Parse(b.Url)
		if err != nil {
			logger.Printf("Cannot parse syslog drain url %s", b.Url)
			sendAppLogMessage(fmt.Sprintf("Cannot parse syslog drain url %s", b.Url), b.Credentials[0].Apps, emitter)
			continue
		}

		anonymousUrl := u
		anonymousUrl.User = nil
		anonymousUrl.RawQuery = ""

		if invalidScheme(u.Scheme) {
			// todo what about multiple credentials?
			logger.Printf("Invalid Scheme for syslog drain url %s", b.Url)
			sendAppLogMessage(fmt.Sprintf("Invalid Scheme for syslog drain url %s", b.Url), b.Credentials[0].Apps, emitter)
			continue
		}

		if len(u.Host) == 0 {
			logger.Printf("No hostname found in syslog drain url %s", b.Url)
			sendAppLogMessage(fmt.Sprintf("No hostname found in syslog drain url %s", b.Url), b.Credentials[0].Apps, emitter)
			continue
		}

		if len(b.Credentials) == 0 {
			logger.Printf("no credentials for %s", b.Url)
			continue
		}

		// use BlacklistRanges as ipChecker

		// todo how to get failed hosts cache?
		// _, exists := f.failedHostsCache.Get(u.Host)
		// if exists {
		// 	invalidDrains += 1
		// 	f.printWarning("Skipped resolve ip address for syslog drain with url %s for application %s due to prior failure", anonymousUrl.String(), b.AppId)
		// 	continue
		// }

		// todo how to resolve addr?
		// ip, err := f.ipChecker.ResolveAddr(u.Host)
		// if err != nil {
		// 	invalidDrains += 1
		// 	f.failedHostsCache.Set(u.Host, true)
		// 	f.printWarning("Cannot resolve ip address for syslog drain with url %s for application %s", anonymousUrl.String(), b.AppId)
		// 	continue
		// }

		// todo how to get blacklist? -> needs config adjustment

		// err = f.ipChecker.CheckBlacklist(ip)
		// if err != nil {
		// 	invalidDrains += 1
		// 	blacklistedDrains += 1
		// 	f.printWarning("Resolved ip address for syslog drain with url %s for application %s is blacklisted", anonymousUrl.String(), b.AppId)
		// 	continue
		// }

		// todo validate certificates for mtls
		//PrivateKey: []byte(b.Drain.Credentials.Key),
		//if len(b.Credentials[0].Cert) > 0 && len(b.Credentials[0].Key) > 0 {
		//	_, err := tls.X509KeyPair([]byte(b.Credentials[0].Cert), []byte(b.Credentials[0].Key))
		//	if err != nil {
		//		errorMessage := err.Error()
		//		sendAppLogMessage(fmt.Sprintf("failed to load certificate: %s", errorMessage), b.Credentials[0].Apps, emitter)
		//		continue
		//	}
		//}
		//if len(b.Credentials[0].CA) > 0 {
		//	certPool := x509.NewCertPool()
		//	ok := certPool.AppendCertsFromPEM([]byte(b.Credentials[0].CA))
		//	if !ok {
		//		sendAppLogMessage("failed to load root CA", b.Credentials[0].Apps, emitter)
		//		continue
		//	}
		//}

	}
}

func sendAppLogMessage(msg string, apps []App, emitter syslog.AppLogEmitter) {
	for _, app := range apps {
		appId := app.AppID
		if appId == "" {
			continue
		}
		emitter.EmitLog(appId, msg)
	}
}

var allowedSchemes = []string{"syslog", "syslog-tls", "https", "https-batch"}

func invalidScheme(scheme string) bool {
	for _, s := range allowedSchemes {
		if s == scheme {
			return false
		}
	}

	return true
}

func CalculateBindingCount(bindings []Binding) int {
	apps := make(map[string]bool)
	for _, b := range bindings {
		for _, c := range b.Credentials {
			for _, a := range c.Apps {
				apps[a.AppID] = true
			}
		}
	}
	return len(apps)
}

type apiResponse struct {
	Results []Binding
	NextID  int `json:"next_id"`
}
