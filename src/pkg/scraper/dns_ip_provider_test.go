package scraper_test

import (
	"os"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/scraper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DnsIpProvider", func() {
	It("returns metrics urls from the ips returned from the lookup", func() {
		dnsFile := writeScrapeConfig(genericConfig)
		scrapeTargets := scraper.NewDNSScrapeTargetProvider("default-source", dnsFile, 9100)
		targets := scrapeTargets()

		var urls []string
		for _, t := range targets {
			urls = append(urls, t.MetricURL)
		}

		Expect(urls).To(ConsistOf(
			"https://10.0.16.26:9100/metrics",
			"https://10.0.16.27:9100/metrics",
		))
	})
})

func writeScrapeConfig(config string) string {
	f, err := os.CreateTemp("", "records.json")
	Expect(err).ToNot(HaveOccurred())

	_, err = f.Write([]byte(config))
	Expect(err).ToNot(HaveOccurred())

	return f.Name()
}

var genericConfig = `
{
    "records": [
      [
        "10.0.16.27",
        "hostname-1"
      ],
      [
        "10.0.16.26",
        "hostname-2"
      ]
    ]
}`
