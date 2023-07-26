package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"

	"net/http"

	_ "net/http/pprof" //nolint:gosec

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator/v9"
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/forwarder-agent/app/otelcolclient"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/diodes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	egress_v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/timeoutwaitgroup"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
)

// ForwarderAgent manages starting the forwarder agent service.
type ForwarderAgent struct {
	pprofPort             uint16
	pprofServer           *http.Server
	m                     Metrics
	grpc                  GRPC
	v2srv                 *v2.Server
	downstreamFilePattern string
	log                   *log.Logger
	tags                  map[string]string
	debugMetrics          bool
}

type Metrics interface {
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
	RegisterDebugMetrics()
}

type BindingFetcher interface {
	FetchBindings() ([]syslog.Binding, error)
}

type Writer interface {
	Write(*loggregator_v2.Envelope) error
}

// NewForwarderAgent intializes and returns a new forwarder agent.
func NewForwarderAgent(
	cfg Config,
	m Metrics,
	log *log.Logger,
) *ForwarderAgent {
	return &ForwarderAgent{
		pprofPort:             cfg.MetricsServer.PprofPort,
		grpc:                  cfg.GRPC,
		m:                     m,
		downstreamFilePattern: cfg.DownstreamIngressPortCfg,
		log:                   log,
		tags:                  cfg.Tags,
		debugMetrics:          cfg.MetricsServer.DebugMetrics,
	}
}

func (s *ForwarderAgent) Run() {
	if s.debugMetrics {
		s.m.RegisterDebugMetrics()
		s.pprofServer = &http.Server{
			Addr:              fmt.Sprintf("127.0.0.1:%d", s.pprofPort),
			Handler:           http.DefaultServeMux,
			ReadHeaderTimeout: 2 * time.Second,
		}
		go func() { s.log.Println("PPROF SERVER STOPPED " + s.pprofServer.ListenAndServe().Error()) }()
	}
	ingressDropped := s.m.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "ingress"}),
	)
	diode := diodes.NewManyToOneEnvelopeV2(10000, gendiodes.AlertFunc(func(missed int) {
		ingressDropped.Add(float64(missed))
	}))

	dests := downstreamDestinations(s.downstreamFilePattern, s.log)
	writers := downstreamWriters(dests, s.grpc, s.log)
	tagger := egress_v2.NewTagger(s.tags)
	ew := egress_v2.NewEnvelopeWriter(
		multiWriter{writers: writers},
		egress_v2.NewCounterAggregator(tagger.TagEnvelope),
	)
	go func() {
		for {
			e := diode.Next()
			ew.Write(e) //nolint:errcheck
		}
	}()

	var opts []plumbing.ConfigOption
	if len(s.grpc.CipherSuites) > 0 {
		opts = append(opts, plumbing.WithCipherSuites(s.grpc.CipherSuites))
	}

	serverCreds, err := plumbing.NewServerCredentials(
		s.grpc.CertFile,
		s.grpc.KeyFile,
		s.grpc.CAFile,
		opts...,
	)
	if err != nil {
		s.log.Fatalf("failed to configure server TLS: %s", err)
	}

	im := s.m.NewCounter(
		"ingress",
		"Total number of envelopes ingressed by the agent.",
	)
	omm := s.m.NewCounter(
		"origin_mappings",
		"Total number of envelopes where the origin tag is used as the source_id.",
	)
	rx := v2.NewReceiver(diode, im, omm)

	s.v2srv = v2.NewServer(
		fmt.Sprintf("127.0.0.1:%d", s.grpc.Port),
		rx,
		grpc.Creds(serverCreds),
		grpc.MaxRecvMsgSize(10*1024*1024),
	)
	s.v2srv.Start()
}

func (s *ForwarderAgent) Stop() {
	if s.pprofServer != nil {
		s.pprofServer.Close()
	}
	s.v2srv.Stop()
}

type clientWriter struct {
	c *loggregator.IngressClient
}

func (c clientWriter) Write(e *loggregator_v2.Envelope) error {
	c.c.Emit(e)
	return nil
}

func (c clientWriter) Close() error {
	return c.c.CloseSend()
}

type multiWriter struct {
	writers []Writer
}

func (mw multiWriter) Write(e *loggregator_v2.Envelope) error {
	for _, w := range mw.writers {
		w.Write(e) //nolint:errcheck
	}
	return nil
}

type destination struct {
	Ingress  string `yaml:"ingress"`
	Protocol string `yaml:"protocol"`
}

func downstreamDestinations(pattern string, l *log.Logger) []destination {
	files, err := filepath.Glob(pattern)
	if err != nil {
		l.Fatal("Unable to read downstream port location")
	}

	var dests []destination
	for _, f := range files {
		yamlFile, err := os.ReadFile(f)
		if err != nil {
			l.Fatalf("cannot read file: %s", err)
		}

		var d destination
		err = yaml.Unmarshal(yamlFile, &d)
		if err != nil {
			l.Fatalf("Unmarshal: %v", err)
		}

		d.Ingress = fmt.Sprintf("127.0.0.1:%s", d.Ingress)

		dests = append(dests, d)
	}

	return dests
}

func downstreamWriters(dests []destination, grpc GRPC, l *log.Logger) []Writer {
	var writers []Writer
	for _, d := range dests {
		var w Writer
		switch d.Protocol {
		case "otelcol":
			w = otelCollectorClient(d, grpc, l)
		default:
			w = loggregatorClient(d, grpc, l)
		}
		writers = append(writers, w)
	}
	return writers
}

func otelCollectorClient(dest destination, grpc GRPC, l *log.Logger) Writer {
	occl := log.New(l.Writer(), fmt.Sprintf("[OTEL COLLECTOR CLIENT] -> %s: ", dest.Ingress), l.Flags())
	c, err := otelcolclient.New(dest.Ingress, occl)
	if err != nil {
		l.Fatalf("Failed to create OTel Collector client for %s: %s", dest.Ingress, err)
	}

	dw := egress.NewDiodeWriter(context.Background(), c, gendiodes.AlertFunc(func(missed int) {
		occl.Printf("Dropped %d envelopes for url %s", missed, dest.Ingress)
	}), timeoutwaitgroup.New(time.Minute))

	return dw
}

func loggregatorClient(dest destination, grpc GRPC, l *log.Logger) Writer {
	clientCreds, err := loggregator.NewIngressTLSConfig(
		grpc.CAFile,
		grpc.CertFile,
		grpc.KeyFile,
	)
	if err != nil {
		l.Fatalf("failed to configure client TLS: %s", err)
	}

	il := log.New(l.Writer(), fmt.Sprintf("[INGRESS CLIENT] -> %s: ", dest.Ingress), l.Flags())
	ingressClient, err := loggregator.NewIngressClient(
		clientCreds,
		loggregator.WithLogger(il),
		loggregator.WithAddr(dest.Ingress),
	)
	if err != nil {
		l.Fatalf("failed to create ingress client for %s: %s", dest.Ingress, err)
	}

	ctx := context.Background()
	wc := clientWriter{ingressClient}
	dw := egress.NewDiodeWriter(ctx, wc, gendiodes.AlertFunc(func(missed int) {
		il.Printf("Dropped %d logs for url %s", missed, dest.Ingress)
	}), timeoutwaitgroup.New(time.Minute))
	return dw
}
