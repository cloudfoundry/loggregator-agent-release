package prom

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent/pkg/egress/v2"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
)

const help = "Metrics Agent collected metric"

type Collector struct {
	counters map[string]prometheus.Metric

	sync.RWMutex
}

func NewCollector() *Collector {
	return &Collector{
		counters:        map[string]prometheus.Metric{},
	}
}

//TODO use /Users/pivotal/workspace/loggregator-agent-release/src/pkg/egress/v2/counter_aggregator.go
//TODO by using an envelope writer instead of reading from the diode

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
	switch env.GetMessage().(type) {
	case *loggregator_v2.Envelope_Counter:
		id, metric = convertCounter(env)
	default:
		return nil
	}

	c.Lock()
	c.counters[id] = metric
	c.Unlock()

	return nil
}

func convertCounter(env *loggregator_v2.Envelope) (string, prometheus.Metric) {
	name := env.GetCounter().GetName()
	labelNames, labelValues := convertTags(env.GetTags())

	desc := prometheus.NewDesc(name, help, labelNames, nil)
	metric, err := prometheus.NewConstMetric(desc, prometheus.CounterValue, float64(env.GetCounter().GetTotal()), labelValues...)
	if err != nil {
		//TODO
		panic(err)
	}

	return name + v2.HashTags(env.GetTags()), metric
}

func convertTags(tags map[string]string) ([]string, []string) {
	var labelNames, labelValues []string

	for name, value := range tags {
		labelNames = append(labelNames, name)
		labelValues = append(labelValues, value)
	}

	return labelNames, labelValues
}
