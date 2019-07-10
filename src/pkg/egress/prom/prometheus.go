package prom

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent/pkg/egress/v2"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"regexp"
	"strings"
	"sync"
	"time"
)

const help = "Metrics Agent collected metric"

var (
	buckets                  = []float64{0.01, 0.2, 1.0, 15.0, 60.0}
	invalidNameRegex         = regexp.MustCompile(`[^a-zA-Z0-9_:]`)
	invalidTagCharacterRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

type Collector struct {
	metrics map[string]prometheus.Metric

	sync.RWMutex
}

func NewCollector() *Collector {
	return &Collector{
		metrics: map[string]prometheus.Metric{},
	}
}

// Describe implements prometheus.Collector
// Unimplemented because metric descriptors should not be checked against other collectors
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {}

// Collect implements prometheus.Collector
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.RLock()
	defer c.RUnlock()

	for _, metric := range c.metrics {
		ch <- metric
	}
}

//Write implements v2.Writer
func (c *Collector) Write(env *loggregator_v2.Envelope) error {
	metrics, err := c.convertEnvelope(env)
	if err != nil {
		return err
	}

	c.Lock()
	for id, metric := range metrics {
		c.metrics[id] = metric
	}
	c.Unlock()

	return nil
}

func (c *Collector) convertEnvelope(env *loggregator_v2.Envelope) (map[string]prometheus.Metric, error) {
	switch env.GetMessage().(type) {
	case *loggregator_v2.Envelope_Counter:
		id, metric, err := c.convertCounter(env)
		return map[string]prometheus.Metric{id: metric}, err
	case *loggregator_v2.Envelope_Gauge:
		return c.convertGaugeEnvelope(env)
	case *loggregator_v2.Envelope_Timer:
		id, metric, err := c.convertTimer(env)
		return map[string]prometheus.Metric{id: metric}, err
	default:
		return nil, nil
	}
}

func (c *Collector) convertCounter(env *loggregator_v2.Envelope) (metricID string, metric prometheus.Metric, err error) {
	name := env.GetCounter().GetName()
	if invalidName(name) {
		return "", nil, fmt.Errorf("invalid metric name: %s", name)
	}

	labelNames, labelValues := convertTags(env)

	desc := prometheus.NewDesc(name, help, labelNames, nil)
	metric, err = prometheus.NewConstMetric(desc, prometheus.CounterValue, float64(env.GetCounter().GetTotal()), labelValues...)
	if err != nil {
		return "", nil, err
	}

	return buildMetricID(name, labelNames, labelValues), metric, nil
}

func (c *Collector) convertGaugeEnvelope(env *loggregator_v2.Envelope) (map[string]prometheus.Metric, error) {
	envelopeLabelNames, envelopeLabelValues := convertTags(env)

	metrics := map[string]prometheus.Metric{}
	for name, metric := range env.GetGauge().GetMetrics() {
		if invalidName(name) {
			return nil, fmt.Errorf("invalid metric name: %s", name)
		}

		id, metric := convertGaugeValue(name, metric, envelopeLabelNames, envelopeLabelValues)
		metrics[id] = metric
	}

	return metrics, nil
}

func convertGaugeValue(name string, gaugeValue *loggregator_v2.GaugeValue, envelopeLabelNames, envelopeLabelValues []string) (string, prometheus.Metric) {
	gaugeLabelNames, gaugeLabelValues := gaugeLabels(gaugeValue, envelopeLabelNames, envelopeLabelValues)

	desc := prometheus.NewDesc(name, help, gaugeLabelNames, nil)
	metric := prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, gaugeValue.Value, gaugeLabelValues...)

	return buildMetricID(name, envelopeLabelNames, envelopeLabelValues), metric
}

func buildMetricID(name string, envelopeLabelNames, envelopeLabelValues []string) string {
	labelTags := labelTags(envelopeLabelNames, envelopeLabelValues)

	return name + v2.HashTags(labelTags)
}

func labelTags(envelopeLabelNames, envelopeLabelValues []string) map[string]string {
	labelTags := map[string]string{}
	for i, labelName := range envelopeLabelNames {
		labelTags[labelName] = envelopeLabelValues[i]
	}
	return labelTags
}

func gaugeLabels(metric *loggregator_v2.GaugeValue, envelopeLabelNames, envelopeLabelValues []string) ([]string, []string) {
	if metric.Unit == "" {
		return envelopeLabelNames, envelopeLabelValues
	}

	return append(envelopeLabelNames, "unit"), append(envelopeLabelValues, metric.Unit)
}

func (c *Collector) convertTimer(env *loggregator_v2.Envelope) (metricID string, metric prometheus.Metric, err error) {
	timer := env.GetTimer()
	name := timer.GetName() + "_seconds"
	if invalidName(name) {
		return "", nil, fmt.Errorf("invalid metric name: %s", name)
	}

	labelNames, labelValues := convertTags(env)
	id := buildMetricID(name, labelNames, labelValues)

	c.RLock()
	metric, ok := c.metrics[id]
	c.RUnlock()
	if !ok {
		metric = prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        name,
			Help:        help,
			Buckets:     buckets,
			ConstLabels: labelTags(labelNames, labelValues),
		})
	}

	metric.(prometheus.Histogram).Observe(durationInSeconds(timer))

	return id, metric, nil
}

func durationInSeconds(timer *loggregator_v2.Timer) float64 {
	return float64(timer.GetStop()-timer.GetStart()) / float64(time.Second)
}

func convertTags(env *loggregator_v2.Envelope) ([]string, []string) {
	var labelNames, labelValues []string

	for name, value := range env.GetTags() {
		if invalidTag(name, value) {
			continue
		}

		labelNames = append(labelNames, name)
		labelValues = append(labelValues, value)
	}

	labelNames = append(labelNames, "source_id")
	labelValues = append(labelValues, env.GetSourceId())

	if env.GetInstanceId() != "" {
		labelNames = append(labelNames, "instance_id")
		labelValues = append(labelValues, env.GetInstanceId())
	}

	return labelNames, labelValues
}

func invalidName(name string) bool {
	return invalidTagCharacterRegex.MatchString(name)
}

func invalidTag(name, value string) bool {
	return strings.HasPrefix(name, "_") || invalidTagCharacterRegex.MatchString(name) || value == ""
}
