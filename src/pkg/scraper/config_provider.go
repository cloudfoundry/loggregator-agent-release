package scraper

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"
)

type PromScraperConfig struct {
	Port           string            `yaml:"port"`
	SourceID       string            `yaml:"source_id"`
	InstanceID     string            `yaml:"instance_id"`
	Scheme         string            `yaml:"scheme"`
	ServerName     string            `yaml:"server_name"`
	Path           string            `yaml:"path"`
	Headers        map[string]string `yaml:"headers"`
	Labels         map[string]string `yaml:"labels"`
	CaPath         string            `yaml:"ca_path"`
	ClientKeyPath  string            `yaml:"client_key_path"`
	ClientCertPath string            `yaml:"client_cert_path"`
	ScrapeInterval time.Duration     `yaml:"scrape_interval"`
}

type ConfigProvider struct {
	globs                 []string
	defaultScrapeInterval time.Duration
	log                   *log.Logger
}

func NewConfigProvider(globs []string, defaultScrapeInterval time.Duration, log *log.Logger) *ConfigProvider {
	return &ConfigProvider{
		globs:                 globs,
		defaultScrapeInterval: defaultScrapeInterval,
		log:                   log,
	}
}

func (p *ConfigProvider) Configs() ([]PromScraperConfig, error) {
	files := p.filesForGlobs()

	var targets []PromScraperConfig
	for _, f := range files {
		scraperConfig, err := p.parseConfig(f)
		if err != nil {
			return nil, err
		}
		portInt, err := strconv.Atoi(scraperConfig.Port)
		if err != nil || portInt <= 0 || portInt > 65536 {
			p.log.Println(fmt.Sprintf("Prom scraper config at %s does not have a valid port - skipping this config file\n", f))
		} else {
			targets = append(targets, scraperConfig)
		}
	}

	return targets, nil
}

func (p *ConfigProvider) filesForGlobs() []string {
	var files []string

	for _, glob := range p.globs {
		globFiles, err := filepath.Glob(glob)
		if err != nil {
			p.log.Println("unable to read config from glob:", glob)
		}

		files = append(files, globFiles...)
	}

	return files
}

func (p *ConfigProvider) parseConfig(file string) (PromScraperConfig, error) {
	yamlFile, err := os.ReadFile(file)
	if err != nil {
		return PromScraperConfig{}, fmt.Errorf("cannot read file: %s", err)
	}

	scraperConfig := PromScraperConfig{
		Scheme:         "http",
		Path:           "/metrics",
		ScrapeInterval: p.defaultScrapeInterval,
	}

	err = yaml.Unmarshal(yamlFile, &scraperConfig)
	if err != nil {
		return PromScraperConfig{}, fmt.Errorf("unmarshal: %v", err)
	}

	return scraperConfig, nil
}
