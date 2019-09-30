package scraper

import (
	metrics "code.cloudfoundry.org/go-metric-registry"
	"fmt"
	"github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"code.cloudfoundry.org/go-loggregator"
)

type Scraper struct {
	targetProvider TargetProvider
	metricsEmitter MetricsEmitter
	metricsGetter  MetricsGetter
	urlsScraped    metrics.Gauge
	failedScrapes  metrics.Gauge
	scrapeDuration metrics.Gauge
	defaultID      string
}

type TargetProvider func() []Target

type ScrapeOption func(s *Scraper)

type Target struct {
	ID          string
	InstanceID  string
	MetricURL   string
	Headers     map[string]string
	DefaultTags map[string]string
}

type MetricsEmitter interface {
	EmitGauge(opts ...loggregator.EmitGaugeOption)
	EmitCounter(name string, opts ...loggregator.EmitCounterOption)
}

type metricsClient interface {
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
}

type MetricsGetter func(addr string, headers map[string]string) (*http.Response, error)

func New(
	t TargetProvider,
	e MetricsEmitter,
	sc MetricsGetter,
	defaultID string,
	opts ...ScrapeOption,
) *Scraper {
	scraper := &Scraper{
		targetProvider: t,
		metricsEmitter: e,
		metricsGetter:  sc,
		defaultID:      defaultID,
		urlsScraped:    &defaultGauge{},
		scrapeDuration: &defaultGauge{},
		failedScrapes:  &defaultGauge{},
	}

	for _, o := range opts {
		o(scraper)
	}

	return scraper
}

func WithMetricsClient(m metricsClient) ScrapeOption {
	return func(s *Scraper) {
		s.urlsScraped = m.NewGauge(
			"last_total_attempted_scrapes",
			"Count of attempted scrapes during last round of scraping.",
			metrics.WithMetricLabels(map[string]string{"unit": "total"}),
		)

		s.failedScrapes = m.NewGauge(
			"last_total_failed_scrapes",
			"Count of failed scrapes during last round of scraping.",
			metrics.WithMetricLabels(map[string]string{"unit": "total"}),
		)

		s.scrapeDuration = m.NewGauge(
			"last_total_scrape_duration",
			"Time in milliseconds to scrape all targets in last round of scraping.",
			metrics.WithMetricLabels(map[string]string{"unit": "ms"}),
		)
	}
}

func (s *Scraper) Scrape() error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		s.scrapeDuration.Set(float64(duration / time.Millisecond))
	}()

	targetList := s.targetProvider()
	errs := make(chan *ScrapeError, len(targetList))
	var wg sync.WaitGroup

	s.urlsScraped.Set(float64(len(targetList)))
	for _, a := range targetList {
		wg.Add(1)

		go func(target Target) {
			scrapeResult, err := s.scrape(target)
			if err != nil {
				errs <- &ScrapeError{
					ID:         target.ID,
					InstanceID: target.InstanceID,
					MetricURL:  target.MetricURL,
					Err:        err,
				}
			}

			s.emitMetrics(scrapeResult, target)
			wg.Done()
		}(a)
	}

	wg.Wait()
	close(errs)

	s.failedScrapes.Set(float64(len(errs)))
	if len(errs) > 0 {
		var errorsSlice []*ScrapeError
		for e := range errs {
			errorsSlice = append(errorsSlice, e)
		}
		return &ScraperError{Errors: errorsSlice}
	}

	return nil
}

func (s *Scraper) scrape(target Target) (map[string]*io_prometheus_client.MetricFamily, error) {
	resp, err := s.metricsGetter(target.MetricURL, target.Headers)
	if err != nil {
		return nil, err
	}

	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, body)
	}

	p := &expfmt.TextParser{}
	res, err := p.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, err
	}

	return res, err
}

func (s *Scraper) emitMetrics(res map[string]*io_prometheus_client.MetricFamily, t Target) {
	for _, family := range res {
		name := family.GetName()

		for _, metric := range family.GetMetric() {
			sourceID, tags := s.parseTags(metric, t)

			switch family.GetType() {
			case io_prometheus_client.MetricType_GAUGE:
				s.emitGauge(sourceID, t.InstanceID, name, tags, metric)
			case io_prometheus_client.MetricType_COUNTER:
				s.emitCounter(sourceID, t.InstanceID, name, tags, metric)
			case io_prometheus_client.MetricType_HISTOGRAM:
				s.emitHistogram(sourceID, t.InstanceID, name, tags, metric)
			case io_prometheus_client.MetricType_SUMMARY:
				s.emitSummary(sourceID, t.InstanceID, name, tags, metric)
			default:
				return
			}
		}
	}
}

func (s *Scraper) emitGauge(sourceID, instanceID, name string, tags map[string]string, metric *io_prometheus_client.Metric) {
	val := metric.GetGauge().GetValue()

	var unit string
	tagUnit, ok := tags["unit"]
	if ok {
		unit = tagUnit
		delete(tags, "unit")
	}

	s.metricsEmitter.EmitGauge(
		loggregator.WithGaugeValue(name, val, unit),
		loggregator.WithGaugeSourceInfo(sourceID, instanceID),
		loggregator.WithEnvelopeTags(tags),
	)
}

func (s *Scraper) emitCounter(sourceID, instanceID, name string, tags map[string]string, metric *io_prometheus_client.Metric) {
	val := metric.GetCounter().GetValue()
	if val != float64(uint64(val)) {
		return //the counter contains a fractional value
	}

	s.metricsEmitter.EmitCounter(
		name,
		loggregator.WithTotal(uint64(val)),
		loggregator.WithCounterSourceInfo(sourceID, instanceID),
		loggregator.WithEnvelopeTags(tags),
	)
}

func (s *Scraper) emitHistogram(sourceID, instanceID, name string, tags map[string]string, metric *io_prometheus_client.Metric) {
	histogram := metric.GetHistogram()

	s.metricsEmitter.EmitGauge(
		loggregator.WithGaugeValue(name+"_sum", histogram.GetSampleSum(), ""),
		loggregator.WithGaugeSourceInfo(sourceID, instanceID),
		loggregator.WithEnvelopeTags(tags),
	)
	s.metricsEmitter.EmitCounter(
		name+"_count",
		loggregator.WithTotal(histogram.GetSampleCount()),
		loggregator.WithCounterSourceInfo(sourceID, instanceID),
		loggregator.WithEnvelopeTags(tags),
	)
	for _, bucket := range histogram.GetBucket() {
		s.metricsEmitter.EmitCounter(
			name+"_bucket",
			loggregator.WithTotal(bucket.GetCumulativeCount()),
			loggregator.WithCounterSourceInfo(sourceID, instanceID),
			loggregator.WithEnvelopeTags(tags),
			loggregator.WithEnvelopeTag("le", strconv.FormatFloat(bucket.GetUpperBound(), 'g', -1, 64)),
		)
	}
}

func (s *Scraper) emitSummary(sourceID, instanceID, name string, tags map[string]string, metric *io_prometheus_client.Metric) {
	summary := metric.GetSummary()
	s.metricsEmitter.EmitGauge(
		loggregator.WithGaugeValue(name+"_sum", summary.GetSampleSum(), ""),
		loggregator.WithGaugeSourceInfo(sourceID, instanceID),
		loggregator.WithEnvelopeTags(tags),
	)
	s.metricsEmitter.EmitCounter(
		name+"_count",
		loggregator.WithTotal(summary.GetSampleCount()),
		loggregator.WithCounterSourceInfo(sourceID, instanceID),
		loggregator.WithEnvelopeTags(tags),
	)
	for _, quantile := range summary.GetQuantile() {
		s.metricsEmitter.EmitGauge(
			loggregator.WithGaugeValue(name, float64(quantile.GetValue()), ""),
			loggregator.WithGaugeSourceInfo(sourceID, instanceID),
			loggregator.WithEnvelopeTags(tags),
			loggregator.WithEnvelopeTag("quantile", strconv.FormatFloat(quantile.GetQuantile(), 'g', -1, 64)),
		)
	}
}

func (s *Scraper) parseTags(m *io_prometheus_client.Metric, t Target) (string, map[string]string) {
	tags := map[string]string{}
	for k, v := range t.DefaultTags {
		tags[k] = v
	}

	sourceID := t.ID
	for _, l := range m.GetLabel() {
		if l.GetName() == "source_id" {
			sourceID = l.GetValue()
			continue
		}
		tags[l.GetName()] = l.GetValue()
	}

	if sourceID == "" {
		return s.defaultID, tags
	}
	return sourceID, tags
}

type defaultGauge struct{}

func (g *defaultGauge) Set(float642 float64) {}
func (g *defaultGauge) Add(float642 float64) {}
