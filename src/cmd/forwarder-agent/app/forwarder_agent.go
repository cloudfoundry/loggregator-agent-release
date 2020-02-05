package app

import (
	"code.cloudfoundry.org/go-metric-registry"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"net/http"

	_ "net/http/pprof"

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/go-loggregator"
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/diodes"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/syslog"
	egress_v2 "code.cloudfoundry.org/loggregator-agent/pkg/egress/v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/ingress/v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"
	"code.cloudfoundry.org/loggregator-agent/pkg/timeoutwaitgroup"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
)

// ForwarderAgent manages starting the forwarder agent service.
type ForwarderAgent struct {
	pprofPort          uint16
	m                  Metrics
	grpc               GRPC
	downstreamPortsCfg string
	log                *log.Logger
	tags               map[string]string
}

type Metrics interface {
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
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
		pprofPort:          cfg.MetricsServer.Port,
		grpc:               cfg.GRPC,
		m:                  m,
		downstreamPortsCfg: cfg.DownstreamIngressPortCfg,
		log:                log,
		tags:               cfg.Tags,
	}
}

func (s ForwarderAgent) Run() {
	go http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", s.pprofPort), nil)

	ingressDropped := s.m.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "ingress"}),
	)
	diode := diodes.NewManyToOneEnvelopeV2(10000, gendiodes.AlertFunc(func(missed int) {
		ingressDropped.Add(float64(missed))
	}))

	downstreamAddrs := getDownstreamAddresses(s.downstreamPortsCfg, s.log)
	clients := ingressClients(downstreamAddrs, s.grpc, s.log)
	tagger := egress_v2.NewTagger(s.tags)
	ew := egress_v2.NewEnvelopeWriter(
		multiWriter{writers: clients},
		egress_v2.NewCounterAggregator(tagger.TagEnvelope),
	)
	go func() {
		for {
			e := diode.Next()
			ew.Write(e)
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

	srv := v2.NewServer(
		fmt.Sprintf("127.0.0.1:%d", s.grpc.Port),
		rx,
		grpc.Creds(serverCreds),
	)
	srv.Start()
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
		w.Write(e)
	}
	return nil
}

type portConfig struct {
	Ingress string `yaml:"ingress"`
}

func getDownstreamAddresses(glob string, l *log.Logger) []string {
	files, err := filepath.Glob(glob)
	if err != nil {
		l.Fatal("Unable to read downstream port location")
	}

	var addrs []string
	for _, f := range files {
		yamlFile, err := ioutil.ReadFile(f)
		if err != nil {
			l.Fatalf("cannot read file: %s", err)
		}

		var c portConfig
		err = yaml.Unmarshal(yamlFile, &c)
		if err != nil {
			l.Fatalf("Unmarshal: %v", err)
		}

		addrs = append(addrs, fmt.Sprintf("127.0.0.1:%s", c.Ingress))
	}

	return addrs
}

func ingressClients(downstreamAddrs []string,
	grpc GRPC,
	l *log.Logger) []Writer {

	var ingressClients []Writer
	for _, addr := range downstreamAddrs {
		clientCreds, err := loggregator.NewIngressTLSConfig(
			grpc.CAFile,
			grpc.CertFile,
			grpc.KeyFile,
		)
		if err != nil {
			l.Fatalf("failed to configure client TLS: %s", err)
		}

		il := log.New(os.Stderr, fmt.Sprintf("[INGRESS CLIENT] -> %s: ", addr), log.LstdFlags)
		ingressClient, err := loggregator.NewIngressClient(
			clientCreds,
			loggregator.WithLogger(il),
			loggregator.WithAddr(addr),
		)
		if err != nil {
			l.Fatalf("failed to create ingress client for %s: %s", addr, err)
		}

		ctx := context.Background()
		wc := clientWriter{ingressClient}
		dw := egress.NewDiodeWriter(ctx, wc, gendiodes.AlertFunc(func(missed int) {
			il.Printf("Dropped %d logs for url %s", missed, addr)
		}), timeoutwaitgroup.New(time.Minute))

		ingressClients = append(ingressClients, dw)
	}

	return ingressClients
}
