package app

import (
	"code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/tlsconfig"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator/v8"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/scraper"
)

type PromScraper struct {
	scrapeConfigProvider ConfigProvider
	cfg                  Config
	log                  *log.Logger
	stop                 chan struct{}
	wg                   sync.WaitGroup
	m                    promRegistry
	scrapeTargetTotals   metrics.Counter
}

type ConfigProvider func() ([]scraper.PromScraperConfig, error)

type promRegistry interface {
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

func NewPromScraper(cfg Config, configProvider ConfigProvider, m promRegistry, log *log.Logger) *PromScraper {
	return &PromScraper{
		scrapeConfigProvider: configProvider,
		cfg:                  cfg,
		log:                  log,
		stop:                 make(chan struct{}),

		m: m,
		scrapeTargetTotals: m.NewCounter(
			"scrape_targets_total",
			"Total number of scrape targets identified from prom scraper config files.",
		),
	}
}

func (p *PromScraper) Run() {
	promScraperConfigs, err := p.scrapeConfigProvider()
	if err != nil {
		p.log.Fatal(err)
	}

	p.validateConfigs(promScraperConfigs)

	client := p.buildIngressClient()

	p.startScrapers(promScraperConfigs, client)
	p.scrapeTargetTotals.Add(float64(len(promScraperConfigs)))
	p.wg.Wait()
}

func (p *PromScraper) validateConfigs(scrapeConfigs []scraper.PromScraperConfig) {
	for _, scrapeConfig := range scrapeConfigs {
		if p.isMTLSTargetMissingServerName(scrapeConfig) {
			p.log.Panicf("server_name is missing from mTLS scrape config (%s)", scrapeConfig.SourceID)
		}
	}
}

func (p *PromScraper) isMTLSTargetMissingServerName(scraperConfig scraper.PromScraperConfig) bool {
	return p.cfg.ScrapeCertPath != "" && scraperConfig.Scheme == "https" && scraperConfig.ServerName == ""
}

func (p *PromScraper) buildIngressClient() *loggregator.IngressClient {
	creds, err := loggregator.NewIngressTLSConfig(
		p.cfg.CACertPath,
		p.cfg.ClientCertPath,
		p.cfg.ClientKeyPath,
	)
	if err != nil {
		p.log.Fatal(err)
	}

	client, err := loggregator.NewIngressClient(
		creds,
		loggregator.WithAddr(p.cfg.LoggregatorIngressAddr),
		loggregator.WithLogger(p.log),
	)
	if err != nil {
		p.log.Fatal(err)
	}

	return client
}

func (p *PromScraper) startScrapers(promScraperConfigs []scraper.PromScraperConfig, ingressClient *loggregator.IngressClient) {
	for _, scrapeConfig := range promScraperConfigs {
		p.wg.Add(1)
		go p.startScraper(scrapeConfig, ingressClient)
	}
}

func (p *PromScraper) startScraper(scrapeConfig scraper.PromScraperConfig, ingressClient *loggregator.IngressClient) {
	defer p.wg.Done()

	s := p.buildScraper(scrapeConfig, ingressClient)
	ticker := time.Tick(scrapeConfig.ScrapeInterval)

	failedScrapesTotal := p.m.NewCounter(
		"failed_scrapes_total",
		"Total number of failed scrapes for the target source_id.",
		metrics.WithMetricLabels(map[string]string{"scrape_target_source_id": scrapeConfig.SourceID}),
	)

	hadError := false
	for {
		select {
		case <-ticker:
			if err := s.Scrape(); err != nil {
				hadError=true
				failedScrapesTotal.Add(1)
				p.log.Printf("failed to scrape: %s", err)
			} else if hadError {
				hadError=false
				p.log.Printf("%s has recovered", scrapeConfig.InstanceID)
			}
		case <-p.stop:
			return
		}
	}
}

func (p *PromScraper) buildScraper(scrapeConfig scraper.PromScraperConfig, client *loggregator.IngressClient) *scraper.Scraper {
	scrapeTarget := scraper.Target{
		ID:          scrapeConfig.SourceID,
		InstanceID:  scrapeConfig.InstanceID,
		MetricURL:   fmt.Sprintf("%s://127.0.0.1:%s/%s", scrapeConfig.Scheme, scrapeConfig.Port, strings.TrimPrefix(scrapeConfig.Path, "/")),
		Headers:     scrapeConfig.Headers,
		DefaultTags: scrapeConfig.Labels,
	}

	httpClient := p.buildHttpClient(scrapeConfig.ScrapeInterval, scrapeConfig.ServerName)

	return scraper.New(
		func() []scraper.Target {
			return []scraper.Target{scrapeTarget}
		},
		client,
		p.scrape(httpClient),
		p.cfg.DefaultSourceID,
	)
}

func (p *PromScraper) buildHttpClient(idleTimeout time.Duration, serverName string) *http.Client {
	tlsOptions := []tlsconfig.TLSOption{tlsconfig.WithInternalServiceDefaults()}
	clientOptions := []tlsconfig.ClientOption{withSkipSSLValidation(p.cfg.SkipSSLValidation)}

	if p.cfg.ScrapeCertPath != "" && p.cfg.ScrapeKeyPath != "" {
		tlsOptions = append(tlsOptions, tlsconfig.WithIdentityFromFile(p.cfg.ScrapeCertPath, p.cfg.ScrapeKeyPath))
		clientOptions = append(clientOptions, tlsconfig.WithServerName(serverName))
	}

	if p.cfg.ScrapeCACertPath != "" {
		clientOptions = append(clientOptions, tlsconfig.WithAuthorityFromFile(p.cfg.ScrapeCACertPath))
	}

	tlsConfig, err := tlsconfig.Build(tlsOptions...).Client(clientOptions...)
	if err != nil {
		p.log.Fatal(err)
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			MaxIdleConns:    1,
			IdleConnTimeout: idleTimeout,
		},
	}
}

// Stops cancel future scrapes and wait for any current scrapes to complete
func (p *PromScraper) Stop() {
	close(p.stop)
	p.wg.Wait()
}

func (p *PromScraper) scrape(client *http.Client) scraper.MetricsGetter {
	return func(addr string, headers map[string]string) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, addr, nil)
		if err != nil {
			return nil, err
		}

		requestHeader := http.Header{}
		for k, v := range headers {
			requestHeader[k] = []string{v}
		}
		req.Header = requestHeader

		return client.Do(req)
	}
}

func withSkipSSLValidation(skipSSLValidation bool) tlsconfig.ClientOption {
	return func(tlsConfig *tls.Config) error {
		tlsConfig.InsecureSkipVerify = skipSSLValidation
		return nil
	}
}
