package app

import (
	"code.cloudfoundry.org/tlsconfig"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator-agent/pkg/scraper"
	"gopkg.in/yaml.v2"
)

type promScraperConfig struct {
	Port           string            `yaml:"port"`
	SourceID       string            `yaml:"source_id"`
	InstanceID     string            `yaml:"instance_id"`
	Scheme         string            `yaml:"scheme"`
	Path           string            `yaml:"path"`
	Headers        map[string]string `yaml:"headers"`
	ScrapeInterval time.Duration     `yaml:"scrape_interval"`
}

type PromScraper struct {
	cfg  Config
	log  *log.Logger
	stop chan struct{}
	wg   sync.WaitGroup
}

func NewPromScraper(cfg Config, log *log.Logger) *PromScraper {
	return &PromScraper{
		cfg:  cfg,
		log:  log,
		stop: make(chan struct{}),
	}
}

func (p *PromScraper) Run() {
	promScraperConfigs := p.scrapeConfigsFromFiles(p.cfg.ConfigGlobs)
	client := p.buildIngressClient()

	p.startScrapers(promScraperConfigs, client)

	p.wg.Wait()
}

func (p *PromScraper) scrapeConfigsFromFiles(globs []string) []promScraperConfig {
	files := p.filesForGlobs(globs)

	var targets []promScraperConfig
	for _, f := range files {
		targets = append(targets, p.parseConfig(f))
	}

	return targets
}

func (p *PromScraper) filesForGlobs(globs []string) []string {
	var files []string

	for _, glob := range globs {
		globFiles, err := filepath.Glob(glob)
		if err != nil {
			p.log.Println("unable to read config from glob:", glob)
		}

		files = append(files, globFiles...)
	}

	return files
}

func (p *PromScraper) parseConfig(file string) promScraperConfig {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		p.log.Fatalf("cannot read file: %s", err)
	}

	scraperConfig := promScraperConfig{
		Scheme:         "http",
		Path:           "/metrics",
		ScrapeInterval: p.cfg.DefaultScrapeInterval,
	}

	err = yaml.Unmarshal(yamlFile, &scraperConfig)
	if err != nil {
		p.log.Fatalf("Unmarshal: %v", err)
	}

	return scraperConfig
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

func (p *PromScraper) startScrapers(promScraperConfigs []promScraperConfig, client *loggregator.IngressClient) {
	for _, scrapeConfig := range promScraperConfigs {
		p.wg.Add(1)
		go p.startScraper(scrapeConfig, client)
	}
}

func (p *PromScraper) startScraper(scrapeConfig promScraperConfig, client *loggregator.IngressClient) {
	defer p.wg.Done()

	s := p.buildScraper(scrapeConfig, client)
	ticker := time.Tick(scrapeConfig.ScrapeInterval)

	for {
		select {
		case <-ticker:
			if err := s.Scrape(); err != nil {
				p.log.Printf("failed to scrape: %s", err)
			}
		case <-p.stop:
			return
		}
	}
}

func (p *PromScraper) buildScraper(scrapeConfig promScraperConfig, client *loggregator.IngressClient) *scraper.Scraper {
	scrapeTarget := scraper.Target{
		ID:         scrapeConfig.SourceID,
		InstanceID: scrapeConfig.InstanceID,
		MetricURL:  fmt.Sprintf("%s://127.0.0.1:%s/%s", scrapeConfig.Scheme, scrapeConfig.Port, strings.TrimPrefix(scrapeConfig.Path, "/")),
		Headers:    scrapeConfig.Headers,
	}

	return scraper.New(
		func() []scraper.Target {
			return []scraper.Target{scrapeTarget}
		},
		client,
		p.scrape,
	)
}

// Stops cancel future scrapes and wait for any current scrapes to complete
func (p *PromScraper) Stop() {
	close(p.stop)
	p.wg.Wait()
}

func (p *PromScraper) scrape(addr string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, addr, nil)
	if err != nil {
		return nil, err
	}

	requestHeader := http.Header{}
	for k, v := range headers {
		requestHeader[k] = []string{v}
	}
	req.Header = requestHeader

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
	).Client(
		withSkipSSLValidation(p.cfg.SkipSSLValidation),
	)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return client.Do(req)
}

func withSkipSSLValidation(skipSSLValidation bool) tlsconfig.ClientOption {
	return func(tlsConfig *tls.Config) error {
		tlsConfig.InsecureSkipVerify = skipSSLValidation
		return nil
	}
}
