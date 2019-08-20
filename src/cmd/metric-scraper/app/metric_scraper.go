package app

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"code.cloudfoundry.org/tlsconfig"
	"log"
	"net"
	"net/http"
	"time"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator-agent/pkg/scraper"
)

type MetricScraper struct {
	cfg           Config
	log           *log.Logger
	scrapeTargets scraper.TargetProvider
	doneChan      chan struct{}
	stoppedChan   chan struct{}
	metrics       metricsClient
}

type metricsClient interface {
	NewCounter(name string, opts ...metrics.MetricOption) metrics.Counter
	NewGauge(name string, opts ...metrics.MetricOption) metrics.Gauge
}

func NewMetricScraper(cfg Config, l *log.Logger, m metricsClient) *MetricScraper {
	return &MetricScraper{
		cfg:           cfg,
		log:           l,
		scrapeTargets: scraper.NewDNSScrapeTargetProvider(cfg.DefaultSourceID, cfg.DNSFile, cfg.ScrapePort),
		doneChan:      make(chan struct{}),
		metrics:       m,
		stoppedChan:   make(chan struct{}),
	}
}

func (m *MetricScraper) Run() {
	m.scrape()
}

func (m *MetricScraper) scrape() {
	creds, err := loggregator.NewIngressTLSConfig(
		m.cfg.CACertPath,
		m.cfg.ClientCertPath,
		m.cfg.ClientKeyPath,
	)
	if err != nil {
		m.log.Fatal(err)
	}

	client, err := loggregator.NewIngressClient(
		creds,
		loggregator.WithAddr(m.cfg.LoggregatorIngressAddr),
		loggregator.WithLogger(m.log),
	)
	if err != nil {
		m.log.Fatal(err)
	}

	tlsClient := newTLSClient(m.cfg)
	s := scraper.New(
		m.scrapeTargets,
		client,
		func(addr string, _ map[string]string) (response *http.Response, e error) {
			return tlsClient.Get(addr)
		},
		m.cfg.DefaultSourceID,
		scraper.WithMetricsClient(m.metrics),
	)

	leadershipClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	numScrapes := m.metrics.NewCounter("num_scrapes")
	t := time.NewTicker(m.cfg.ScrapeInterval)
	for {
		select {
		case <-t.C:
			resp, err := leadershipClient.Get(m.cfg.LeadershipServerAddr)
			if err == nil && resp.StatusCode == http.StatusLocked {
				continue
			}

			if err := s.Scrape(); err != nil {
				m.log.Printf("failed to scrape: %s", err)
			}

			numScrapes.Add(1.0)
		case <-m.doneChan:
			close(m.stoppedChan)
			return
		}
	}
}

func (m *MetricScraper) Stop() {
	close(m.doneChan)
	<-m.stoppedChan
}

func newTLSClient(cfg Config) *http.Client {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(cfg.MetricsCertPath, cfg.MetricsKeyPath),
	).Client(
		tlsconfig.WithAuthorityFromFile(cfg.MetricsCACertPath),
		tlsconfig.WithServerName(cfg.MetricsCN),
	)

	if err != nil {
		log.Panicf("failed to load API client certificates: %s", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.ScrapeTimeout,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.ScrapeTimeout,
	}
}
