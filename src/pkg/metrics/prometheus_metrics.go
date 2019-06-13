package metrics

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type PromRegistry struct {
	port        string
	registry    *prometheus.Registry
	defaultTags map[string]string
	loggr       *log.Logger
}

type Counter interface {
	Add(float64)
}

type Gauge interface {
	Add(float64)
	Set(float64)
}

//By default, the prom registry will register the metrics route with the default
//http mux but will not start a http server. This is intentional so that we can
//combine metrics with other things like pprof into one server. To start a server
//just for metrics, the WithServer RegistryOption
func NewPromRegistry(defaultSourceID string, logger *log.Logger, opts ...RegistryOption) *PromRegistry {
	registry := prometheus.NewRegistry()

	pr := &PromRegistry{
		registry:    registry,
		defaultTags: map[string]string{"source_id": defaultSourceID, "origin": defaultSourceID},
		loggr:       logger,
	}

	for _, o := range opts {
		o(pr)
	}

	httpHandler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	http.Handle("/metrics", httpHandler)

	return pr
}

type RegistryOption func(r *PromRegistry)

func WithDefaultTags(tags map[string]string) RegistryOption {
	return func(r *PromRegistry) {
		for k, v := range tags {
			r.defaultTags[k] = v
		}
	}
}

func WithServer(port int) RegistryOption {
	return func(r *PromRegistry) {
		r.start(port)
	}
}

func (p *PromRegistry) start(port int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	s := http.Server{
		Addr:         addr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		p.loggr.Fatalf("Unable to setup metrics endpoint (%s): %s", addr, err)
	}
	p.loggr.Printf("Metrics endpoint is listening on %s", lis.Addr().String())

	parts := strings.Split(lis.Addr().String(), ":")
	p.port = parts[len(parts)-1]

	go s.Serve(lis)
}

func (p *PromRegistry) NewCounter(name string, opts ...MetricOption) Counter {
	opt := p.newMetricOpt(name, "counter metric", opts...)
	counter := prometheus.NewCounter(prometheus.CounterOpts(opt))

	collector := p.registerCollector(name, counter)
	return collector.(Counter)
}

func (p *PromRegistry) NewGauge(name string, opts ...MetricOption) Gauge {
	opt := p.newMetricOpt(name, "gauge metric", opts...)
	gauge := prometheus.NewGauge(prometheus.GaugeOpts(opt))

	collector := p.registerCollector(name, gauge)
	return collector.(Gauge)
}

func (p *PromRegistry) registerCollector(name string, c prometheus.Collector) prometheus.Collector {
	err := p.registry.Register(c)
	if err != nil {
		typ, ok := err.(prometheus.AlreadyRegisteredError)
		if !ok {
			p.loggr.Panicf("unable to create %s: %s", name, err)
		}

		return typ.ExistingCollector
	}

	return c
}

func (p *PromRegistry) Port() string {
	return fmt.Sprint(p.port)
}

func (p *PromRegistry) newMetricOpt(name, helpText string, mOpts ...MetricOption) prometheus.Opts {
	opt := prometheus.Opts{
		Name:        name,
		Help:        helpText,
		ConstLabels: make(map[string]string),
	}

	for _, o := range mOpts {
		o(&opt)
	}

	for k, v := range p.defaultTags {
		opt.ConstLabels[k] = v
	}

	return opt
}

func WithMetricTags(tags map[string]string) MetricOption {
	return func(o *prometheus.Opts) {
		for k, v := range tags {
			o.ConstLabels[k] = v
		}
	}
}

func WithHelpText(helpText string) MetricOption {
	return func(o *prometheus.Opts) {
		o.Help = helpText
	}
}

type MetricOption func(o *prometheus.Opts)
