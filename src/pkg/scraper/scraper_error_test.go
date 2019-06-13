package scraper_test

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/scraper"
	"errors"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ScraperError", func() {
	It("prints all the errors", func() {
		var err error
		err = &scraper.ScraperError{
			Errors: []*scraper.ScrapeError{
				{ID: "1", InstanceID: "instance-1", MetricURL: "https://1", Err: errors.New("one")},
				{ID: "2", InstanceID: "instance-2", MetricURL: "https://2", Err: errors.New("two")},
			},
		}

		Expect(err).To(MatchError("scrape errors:\n[id: 1, instance_id: instance-1, metric_url: https://1]: one" +
			"\n[id: 2, instance_id: instance-2, metric_url: https://2]: two"))
	})
})
