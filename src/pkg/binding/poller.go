package binding

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/applog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/simplecache"
)

type Poller struct {
	apiClient       client
	pollingInterval time.Duration
	store           Setter

	logger                     *log.Logger
	bindingRefreshErrorCounter metrics.Counter
	lastBindingCount           metrics.Gauge
	emitter                    applog.LogEmitter
	checker                    IPChecker
	failedHostsCache           *simplecache.SimpleCache[string, bool]
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . IPChecker
type IPChecker interface {
	ResolveAddr(host string) (net.IP, error)
	CheckBlacklist(ip net.IP) error
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

func NewPoller(ac client, pi time.Duration, s Setter, m Metrics, logger *log.Logger, emitter applog.LogEmitter, checker IPChecker) *Poller {
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
		emitter:          emitter,
		checker:          checker,
		failedHostsCache: simplecache.New[string, bool](120 * time.Second),
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

	checkBindings(bindings, p.emitter, p.checker, p.logger, p.failedHostsCache)

	bindingCount := CalculateBindingCount(bindings)
	p.lastBindingCount.Set(float64(bindingCount))
	p.store.Set(bindings, bindingCount)
}

func checkBindings(bindings []Binding, emitter applog.LogEmitter, checker IPChecker, logger *log.Logger, failedHostsCache *simplecache.SimpleCache[string, bool]) {
	logger.Printf("checking bindings - found %d bindings", len(bindings))
	for _, b := range bindings {
		if len(b.Credentials) == 0 {
			logger.Printf("no credentials for %s", b.Url)
			continue
		}

		//todo provide Prometheus metrics for invalid/blacklisted drains
		u, err := url.Parse(b.Url)

		for _, cred := range b.Credentials {
			if err != nil {
				sendAppLogMessage(fmt.Sprintf("Cannot parse syslog drain url %s", b.Url), cred.Apps, emitter, logger)
				continue
			}

			anonymousUrl := u
			anonymousUrl.User = nil
			anonymousUrl.RawQuery = ""

			if invalidScheme(u.Scheme) {
				sendAppLogMessage(fmt.Sprintf("Invalid Scheme for syslog drain url %s", b.Url), cred.Apps, emitter, logger)
				continue
			}

			if len(u.Host) == 0 {
				sendAppLogMessage(fmt.Sprintf("No hostname found in syslog drain url %s", b.Url), cred.Apps, emitter, logger)
				continue
			}

			_, exists := failedHostsCache.Get(u.Host)
			if exists {
				//invalidDrains += 1
				sendAppLogMessage(fmt.Sprintf("Skipped resolve ip address for syslog drain with url %s due to prior failure", anonymousUrl.String()), cred.Apps, emitter, logger)
				continue
			}

			ip, err := checker.ResolveAddr(u.Host)
			if err != nil {
				//invalidDrains += 1
				failedHostsCache.Set(u.Host, true)
				sendAppLogMessage(fmt.Sprintf("Cannot resolve ip address for syslog drain with url %s", anonymousUrl.String()), cred.Apps, emitter, logger)
				continue
			}

			err = checker.CheckBlacklist(ip)
			if err != nil {
				//invalidDrains += 1
				//blacklistedDrains += 1
				sendAppLogMessage(fmt.Sprintf("Resolved ip address for syslog drain with url %s is blacklisted", anonymousUrl.String()), cred.Apps, emitter, logger)
				continue
			}

			if len(cred.Cert) > 0 && len(cred.Key) > 0 {
				_, err := tls.X509KeyPair([]byte(cred.Cert), []byte(cred.Key))
				if err != nil {
					errorMessage := err.Error()
					sendAppLogMessage(fmt.Sprintf("failed to load certificate: %s", errorMessage), cred.Apps, emitter, logger)
					continue
				}
			}

			if len(cred.CA) > 0 {
				certPool := x509.NewCertPool()
				ok := certPool.AppendCertsFromPEM([]byte(cred.CA))
				if !ok {
					sendAppLogMessage("failed to load root CA", cred.Apps, emitter, logger)
					continue
				}
			}
		}
	}
}

func sendAppLogMessage(msg string, apps []App, emitter applog.LogEmitter, logger *log.Logger) {
	for _, app := range apps {
		appId := app.AppID
		if appId == "" {
			continue
		}
		emitter.EmitAppLog(appId, msg)
		emitter.EmitPlatformLog(fmt.Sprintf("%s for app %s", msg, appId))
		logger.Printf("%s for app %s", msg, appId)
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
