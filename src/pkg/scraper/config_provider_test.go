package scraper_test

import (
	"fmt"
	"log"
	"os"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/scraper"

	. "github.com/onsi/ginkgo"
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
})

const (
	metricConfigTemplate = `---
port: 8080
source_id: some-id
instance_id: some-instance-id`

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
