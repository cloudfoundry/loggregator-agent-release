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
	"time"

	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/loggregator-agent/pkg/scraper"
	"gopkg.in/yaml.v2"
)

type PromScraper struct {
	cfg  Config
	log  *log.Logger
	stop chan struct{}
	done chan struct{}
}

func NewPromScraper(cfg Config, log *log.Logger) *PromScraper {
	return &PromScraper{
		cfg: cfg,
		log: log,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (p *PromScraper) Run() {
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

	scrapeTargetProvider := func() []scraper.Target {
		return p.scrapeTargetsFromFiles(p.cfg.ConfigGlobs)
	}

	s := scraper.New(
		scrapeTargetProvider,
		client,
		p.scrape,
	)

	ticker := time.Tick(p.cfg.ScrapeInterval)
	for {
		select {
		case <-ticker:
			if err := s.Scrape(); err != nil {
				p.log.Printf("failed to scrape: %s", err)
			}
		case <-p.stop:
			close(p.done)
			return
		}
	}
}

// Stops cancel future scrapes and wait for any current scrapes to complete
func (p *PromScraper) Stop() {
	close(p.stop)
	<-p.done
}

func (p *PromScraper) scrapeTargetsFromFiles(globs []string) []scraper.Target {
	files := p.filesForGlobs(globs)

	var targets []scraper.Target
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

type promScraperConfig struct {
	Port       string            `yaml:"port"`
	SourceID   string            `yaml:"source_id"`
	InstanceID string            `yaml:"instance_id"`
	Scheme     string            `yaml:"scheme"`
	Path       string            `yaml:"path"`
	Headers    map[string]string `yaml:"headers"`
}

func (p *PromScraper) parseConfig(file string) scraper.Target {
	yamlFile, err := ioutil.ReadFile(file)
	if err != nil {
		p.log.Fatalf("cannot read file: %s", err)
	}

	c := promScraperConfig{
		Scheme: "http",
		Path:   "/metrics",
	}

	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		p.log.Fatalf("Unmarshal: %v", err)
	}

	return scraper.Target{
		ID:         c.SourceID,
		InstanceID: c.InstanceID,
		MetricURL:  fmt.Sprintf("%s://127.0.0.1:%s/%s", c.Scheme, c.Port, strings.TrimPrefix(c.Path, "/")),
		Headers:    c.Headers,
	}
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
