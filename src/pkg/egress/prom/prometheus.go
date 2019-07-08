package prom

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent/pkg/egress/v2"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"regexp"
	"strings"
	"sync"
)

const help = "Metrics Agent collected metric"

var invalidTagCharacterRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)

type Collector struct {
	counters map[string]prometheus.Metric

	sync.RWMutex
}

func NewCollector() *Collector {
	return &Collector{
		counters: map[string]prometheus.Metric{},
	}
}

// Describe implements prometheus.Collector
// Unimplemented because metric descriptors should not be checked against other collectors
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {}

// Collect implements prometheus.Collector
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.RLock()
	defer c.RUnlock()

	for _, counter := range c.counters {
		ch <- counter
	}
}

//Write implements v2.Writer
func (c *Collector) Write(env *loggregator_v2.Envelope) error {
	var id string
	var metric prometheus.Metric
	var err error

	switch env.GetMessage().(type) {
	case *loggregator_v2.Envelope_Counter:
		id, metric, err = c.convertCounter(env)
	default:
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to convert envelope to Prometheus metric: %s", err)
	}

	c.Lock()
	c.counters[id] = metric
	c.Unlock()

	return nil
}

func (c *Collector) convertCounter(env *loggregator_v2.Envelope) (string, prometheus.Metric, error) {
	name := env.GetCounter().GetName()
	labelNames, labelValues := convertTags(env)

	desc := prometheus.NewDesc(name, help, labelNames, nil)
	metric, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, float64(env.GetCounter().GetTotal()), labelValues...)
	if err != nil {
		return "", nil, err
	}

	return name + v2.HashTags(env.GetTags()), metric, nil
}

func convertTags(env *loggregator_v2.Envelope) ([]string, []string) {
	var labelNames, labelValues []string

	for name, value := range env.Tags {
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

func invalidTag(name, value string) bool {
	return strings.HasPrefix(name, "_") || invalidTagCharacterRegex.MatchString(name) || value == ""
}
