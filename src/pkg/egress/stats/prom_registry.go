package stats

import (
	"fmt"
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

type PromRegistry struct {
	Registerer prometheus.Registerer
	gauges     map[string]prometheus.Gauge
}

func NewPromRegistry(r prometheus.Registerer) *PromRegistry {
	return &PromRegistry{
		Registerer: r,
		gauges:     make(map[string]prometheus.Gauge),
	}
}

func (r *PromRegistry) Get(name, origin, unit string, tags map[string]string) Gauge {
	if tags == nil {
		tags = make(map[string]string)
	}
	tags["unit"] = unit

	gaugeName := gaugeName(name, origin, unit, tags)
	g, ok := r.gauges[gaugeName]
	if ok {
		return g
	}

	g = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        name,
			Help:        "vm metric",
			ConstLabels: tags,
		},
	)

	r.Registerer.Register(g)
	r.gauges[gaugeName] = g
	return g
}

func gaugeName(name, origin, unit string, tags map[string]string) string {
	return fmt.Sprintf("%s_%s_%s", name, origin, sortedTags(tags))
}

func sortedTags(tags map[string]string) string {
	var keys []string
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var t []string
	for _, k := range keys {
		t = append(t, k, tags[k])
	}
	return strings.Join(t, "_")
}
