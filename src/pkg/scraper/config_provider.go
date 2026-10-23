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
		fileTargets, err := p.parseConfig(f)
		if err != nil {
			return nil, err
		}
		for _, cfg := range fileTargets {
			portInt, err := strconv.Atoi(cfg.Port)
			if err != nil || portInt <= 0 || portInt > 65536 {
				p.log.Printf("Prom scraper config at %s does not have a valid port - skipping this config file\n", f)
			} else {
				targets = append(targets, cfg)
			}
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

func (p *ConfigProvider) parseConfig(file string) ([]PromScraperConfig, error) {
	yamlFile, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read file: %s", err)
	}

	var listConfigs []PromScraperConfig
	if err := yaml.Unmarshal(yamlFile, &listConfigs); err == nil && listConfigs != nil {
		for i := range listConfigs {
			applyConfigDefaults(&listConfigs[i], p.defaultScrapeInterval)
		}
		return listConfigs, nil
	}

	cfg := PromScraperConfig{
		Scheme:         "http",
		Path:           "/metrics",
		ScrapeInterval: p.defaultScrapeInterval,
	}
	if err := yaml.Unmarshal(yamlFile, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %v", err)
	}
	return []PromScraperConfig{cfg}, nil
}

func applyConfigDefaults(cfg *PromScraperConfig, defaultInterval time.Duration) {
	if cfg.Scheme == "" {
		cfg.Scheme = "http"
	}
	if cfg.Path == "" {
		cfg.Path = "/metrics"
	}
	if cfg.ScrapeInterval == 0 {
		cfg.ScrapeInterval = defaultInterval
	}
}
