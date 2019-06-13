package scraper

import (
	"fmt"
	"strings"
)

type ScrapeError struct {
	ID         string
	InstanceID string
	MetricURL  string
	Err        error
}

func (e *ScrapeError) Error() string {
	return fmt.Sprintf("[id: %s, instance_id: %s, metric_url: %s]: %s", e.ID, e.InstanceID, e.MetricURL, e.Err)
}

type ScraperError struct {
	Errors []*ScrapeError
}

func (e *ScraperError) Error() string {
	var scrapeErrors []string
	for _, scrapeErr := range e.Errors {
		scrapeErrors = append(scrapeErrors, scrapeErr.Error())
	}

	return fmt.Sprintf("scrape errors:\n%s", strings.Join(scrapeErrors, "\n"))
}
