package scraper_test

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/scraper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PromScraper", func() {
	var (
		metricConfigDir string
		configGlobs     string

		testLogger            = log.New(GinkgoWriter, "", log.LstdFlags)
		defaultScrapeInterval = 100 * time.Millisecond
	)

	BeforeEach(func() {
		metricConfigDir = metricPortConfigDir()
		configGlobs = fmt.Sprintf("%s/prom_scraper_config*", metricConfigDir)
	})

	AfterEach(func() {
		Eventually(func() error {
			return os.RemoveAll(metricConfigDir)
		}, 10).Should(Succeed())
	})

	It("parses multiple configs", func() {
		writeScrapeConfigFile(metricConfigDir, metricConfigTemplate, "prom_scraper_config.yml")
		writeScrapeConfigFile(metricConfigDir, metricConfigWithAllFieldsSpecifiedTemplate, "prom_scraper_config2.yml")

		ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
		Expect(err).ToNot(HaveOccurred())
		Expect(ps).To(ConsistOf(
			scraper.PromScraperConfig{
				Port:           "8080",
				SourceID:       "some-id",
				InstanceID:     "some-instance-id",
				Scheme:         "http",
				Path:           "/metrics",
				ScrapeInterval: 100 * time.Millisecond,
			},
			scraper.PromScraperConfig{
				Port:       "8081",
				SourceID:   "some-id",
				InstanceID: "some-instance-id",
				Scheme:     "https",
				ServerName: "some-server",
				Path:       "/other",
				Headers: map[string]string{
					"Header1": "value1",
					"Header2": "value2",
				},
				Labels: map[string]string{
					"label": "value",
				},
				ScrapeInterval: 10 * time.Second,
			},
		))
	})

	Context("defaults", func() {
		It("defaults path to /metrics", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigTemplate, "prom_scraper_config.yml")

			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(HaveLen(1))
			Expect(ps[0].Path).To(Equal("/metrics"))
		})

		It("defaults scheme to http", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigTemplate, "prom_scraper_config.yml")

			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(HaveLen(1))
			Expect(ps[0].Scheme).To(Equal("http"))
		})

		It("defaults scrape interval to defaultScrapeInterval", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigTemplate, "prom_scraper_config.yml")

			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(HaveLen(1))
			Expect(ps[0].ScrapeInterval).To(Equal(100 * time.Millisecond))
		})
	})

	It("returns a error if port is not set", func() {
		writeScrapeConfigFile(metricConfigDir, metricConfigEmpty, "prom_scraper_config.yml")

		var buffer bytes.Buffer
		assertableLogger := log.New(&buffer, "", log.LstdFlags)
		ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, assertableLogger).Configs()
		Expect(err).ToNot(HaveOccurred())
		Expect(ps).To(HaveLen(0))
		Expect(buffer.String()).To(MatchRegexp("Prom scraper config at /.*/prom_scraper_config.yml[0-9]* does not have a valid port - skipping this config file"))
	})

	It("returns a error if port is less than 1", func() {
		writeScrapeConfigFile(metricConfigDir, metricConfigPortTooSmall, "prom_scraper_config.yml")

		var buffer bytes.Buffer
		assertableLogger := log.New(&buffer, "", log.LstdFlags)
		ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, assertableLogger).Configs()
		Expect(err).ToNot(HaveOccurred())
		Expect(ps).To(HaveLen(0))
		Expect(buffer.String()).To(MatchRegexp("Prom scraper config at /.*/prom_scraper_config.yml[0-9]* does not have a valid port - skipping this config file"))
	})

	It("returns a error if port is greater than 65536", func() {
		writeScrapeConfigFile(metricConfigDir, metricConfigPortTooLarge, "prom_scraper_config.yml")

		var buffer bytes.Buffer
		assertableLogger := log.New(&buffer, "", log.LstdFlags)
		ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, assertableLogger).Configs()
		Expect(err).ToNot(HaveOccurred())
		Expect(ps).To(HaveLen(0))
		Expect(buffer.String()).To(MatchRegexp("Prom scraper config at /.*/prom_scraper_config.yml[0-9]* does not have a valid port - skipping this config file"))
	})

	It("returns a error if port is not a number", func() {
		writeScrapeConfigFile(metricConfigDir, metricConfigPortNotANumber, "prom_scraper_config.yml")

		var buffer bytes.Buffer
		assertableLogger := log.New(&buffer, "", log.LstdFlags)
		ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, assertableLogger).Configs()
		Expect(err).ToNot(HaveOccurred())
		Expect(ps).To(HaveLen(0))
		Expect(buffer.String()).To(MatchRegexp("Prom scraper config at /.*/prom_scraper_config.yml[0-9]* does not have a valid port - skipping this config file"))
	})

	Describe("list format (static targets)", func() {
		It("parses a file containing a list of targets", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigListTemplate, "prom_scraper_config.yml")

			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(ConsistOf(
				scraper.PromScraperConfig{
					Port:           "8080",
					SourceID:       "first",
					Scheme:         "http",
					Path:           "/metrics",
					ScrapeInterval: defaultScrapeInterval,
				},
				scraper.PromScraperConfig{
					Port:           "8081",
					SourceID:       "second",
					Scheme:         "https",
					ServerName:     "some-server",
					Path:           "/other",
					ScrapeInterval: 10 * time.Second,
				},
			))
		})

		It("applies defaults to list entries missing scheme, path, and scrape_interval", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigListMinimal, "prom_scraper_config.yml")

			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(HaveLen(1))
			Expect(ps[0].Scheme).To(Equal("http"))
			Expect(ps[0].Path).To(Equal("/metrics"))
			Expect(ps[0].ScrapeInterval).To(Equal(defaultScrapeInterval))
		})

		It("skips list entries with invalid ports and continues", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigListWithBadPort, "prom_scraper_config.yml")

			var buffer bytes.Buffer
			assertableLogger := log.New(&buffer, "", log.LstdFlags)
			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, assertableLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(HaveLen(1))
			Expect(ps[0].Port).To(Equal("8080"))
			Expect(buffer.String()).To(ContainSubstring("does not have a valid port - skipping this config file"))
		})

		It("returns no targets for an empty list", func() {
			writeScrapeConfigFile(metricConfigDir, metricConfigEmptyList, "prom_scraper_config.yml")

			ps, err := scraper.NewConfigProvider([]string{configGlobs}, defaultScrapeInterval, testLogger).Configs()
			Expect(err).ToNot(HaveOccurred())
			Expect(ps).To(BeEmpty())
		})
	})
})

const (
	metricConfigTemplate = `---
port: 8080
source_id: some-id
instance_id: some-instance-id`

	metricConfigEmpty        = `---`
	metricConfigPortTooSmall = `---
port: 0`
	metricConfigPortTooLarge = `---
port: 65537 `
	metricConfigPortNotANumber = `---
port: foo`

	metricConfigWithAllFieldsSpecifiedTemplate = `---
port: 8081
source_id: some-id
instance_id: some-instance-id
scrape_interval: 10s
path: /other
scheme: https
server_name: some-server
headers:
  Header1: value1
  Header2: value2
labels:
  label: value`

	metricConfigListTemplate = `---
- port: "8080"
  source_id: first
- port: "8081"
  source_id: second
  scheme: https
  server_name: some-server
  path: /other
  scrape_interval: 10s`

	metricConfigListMinimal = `---
- port: "9090"
  source_id: minimal`

	metricConfigListWithBadPort = `---
- port: "8080"
  source_id: good
- port: "bad"
  source_id: invalid`

	metricConfigEmptyList = `--- []`
)

func metricPortConfigDir() string {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		log.Fatal(err)
	}

	return dir
}

func writeScrapeConfigFile(metricConfigDir, config, fileName string) {
	f, err := os.CreateTemp(metricConfigDir, fileName)
	if err != nil {
		log.Fatal(err)
	}

	_, err = f.Write([]byte(config))
	if err != nil {
		log.Fatal(err)
	}
}
