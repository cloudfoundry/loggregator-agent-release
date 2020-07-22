package app

import (
	"code.cloudfoundry.org/go-metric-registry"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v1"
	"fmt"
	"log"
	"net/http"

	"code.cloudfoundry.org/go-loggregator/v8"
	"code.cloudfoundry.org/go-loggregator/v8/conversion"
	ingress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v1"
	"github.com/cloudfoundry/sonde-go/events"
)

type Metrics interface {
	NewGauge(name, helpText string,  opts ...metrics.MetricOption) metrics.Gauge
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
}

type UDPForwarder struct {
	grpc       GRPC
	udpPort    int
	debugPort  int
	log        *log.Logger
	metrics    Metrics
	deployment string
	job        string
	index      string
	ip         string
}

func NewUDPForwarder(cfg Config, l *log.Logger, m Metrics) *UDPForwarder {
	return &UDPForwarder{
		grpc:       cfg.LoggregatorAgentGRPC,
		udpPort:    cfg.UDPPort,
		debugPort:  cfg.DebugPort,
		log:        l,
		metrics:    m,
		deployment: cfg.Deployment,
		job:        cfg.Job,
		index:      cfg.Index,
		ip:         cfg.IP,
	}
}

func (u *UDPForwarder) Run() {
	tlsConfig, err := loggregator.NewIngressTLSConfig(
		u.grpc.CAFile,
		u.grpc.CertFile,
		u.grpc.KeyFile,
	)
	if err != nil {
		u.log.Fatalf("Failed to create loggregator agent credentials: %s", err)
	}

	v2Ingress, err := loggregator.NewIngressClient(
		tlsConfig,
		loggregator.WithLogger(u.log),
		loggregator.WithAddr(u.grpc.Addr),
	)
	if err != nil {
		u.log.Fatalf("Failed to create loggregator agent client: %s", err)
	}

	w := v1.NewTagger(
		u.deployment,
		u.job,
		u.index,
		u.ip,
		v2Writer{v2Ingress},
	)

	dropsondeUnmarshaller := ingress.NewUnMarshaller(w)
	networkReader, err := ingress.NewNetworkReader(
		fmt.Sprintf("127.0.0.1:%d", u.udpPort),
		dropsondeUnmarshaller,
		u.metrics,
	)
	if err != nil {
		u.log.Fatalf("Failed to listen on 127.0.0.1:%d: %s", u.udpPort, err)
	}

	go func() {
		http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", u.debugPort), nil)
	}()

	go networkReader.StartReading()
	networkReader.StartWriting()
}

type v2Writer struct {
	ingressClient *loggregator.IngressClient
}

func (w v2Writer) Write(e *events.Envelope) {
	v2e := conversion.ToV2(e, true)
	w.ingressClient.Emit(v2e)
}
