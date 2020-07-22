package app

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	metrics "code.cloudfoundry.org/go-metric-registry"

	gendiodes "code.cloudfoundry.org/go-diodes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/clientpool"
	clientpoolv2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/clientpool/v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/diodes"
	egress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"
	ingress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// MetricClient is used to serve metrics.
type MetricClient interface {
	NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
}

// AppV2Option configures AppV2 options.
type AppV2Option func(*AppV2)

// WithV2Lookup allows the default DNS resolver to be changed.
func WithV2Lookup(l func(string) ([]net.IP, error)) func(*AppV2) {
	return func(a *AppV2) {
		a.lookup = l
	}
}

type AppV2 struct {
	config       *Config
	clientCreds  credentials.TransportCredentials
	serverCreds  credentials.TransportCredentials
	metricClient MetricClient
	lookup       func(string) ([]net.IP, error)
}

type envelopeSetter interface {
	Set(e *loggregator_v2.Envelope)
}

func NewV2App(
	c *Config,
	clientCreds credentials.TransportCredentials,
	serverCreds credentials.TransportCredentials,
	metricClient MetricClient,
	opts ...AppV2Option,
) *AppV2 {
	a := &AppV2{
		config:       c,
		clientCreds:  clientCreds,
		serverCreds:  serverCreds,
		metricClient: metricClient,
		lookup:       net.LookupIP,
	}

	for _, o := range opts {
		o(a)
	}

	return a
}

func (a *AppV2) Start() {
	if a.serverCreds == nil {
		log.Panic("Failed to load TLS server config")
	}

	droppedMetric := a.metricClient.NewCounter(
		"dropped",
		"Total number of dropped envelopes.",
		metrics.WithMetricLabels(map[string]string{"direction": "ingress", "metric_version": "2.0"}),
	)
	envelopeBuffer := diodes.NewManyToOneEnvelopeV2(10000, gendiodes.AlertFunc(func(missed int) {
		// metric-documentation-v2: (loggregator.metron.dropped) Number of v2 envelopes
		// dropped from the agent ingress diode
		droppedMetric.Add(float64(missed))

		log.Printf("Dropped %d v2 envelopes", missed)
	}))

	pool := a.initializePool()
	tagger := egress.NewTagger(a.config.Tags)
	batchWriter := egress.NewBatchEnvelopeWriter(
		pool,
		egress.NewCounterAggregator(tagger.TagEnvelope),
	)

	ingressMetric := a.metricClient.NewCounter(
		"ingress",
		"Total number of envelopes ingressed by the agent.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "2.0"}),
	)
	originMappings := a.metricClient.NewCounter(
		"origin_mappings",
		"Total number of envelopes where the origin tag is used as the source_id.",
		metrics.WithMetricLabels(map[string]string{"unit": "bytes/minute", "metric_version": "2.0"}),
	)

	tx := egress.NewTransponder(
		envelopeBuffer,
		batchWriter,
		100, 100*time.Millisecond,
		a.metricClient,
	)
	go tx.Start()

	agentAddress := fmt.Sprintf("127.0.0.1:%d", a.config.GRPC.Port)
	log.Printf("agent v2 API started on addr %s", agentAddress)

	var es envelopeSetter
	es = envelopeBuffer
	if a.config.LogsDisabled {
		es = v2.NewFilteringSetter(envelopeBuffer)
	}

	rx := ingress.NewReceiver(es, ingressMetric, originMappings)

	kp := keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}
	ingressServer := ingress.NewServer(
		agentAddress,
		rx,
		grpc.Creds(a.serverCreds),
		grpc.KeepaliveEnforcementPolicy(kp),
	)
	ingressServer.Start()
}

func (a *AppV2) initializePool() *clientpoolv2.ClientPool {
	if a.clientCreds == nil {
		log.Panic("Failed to load TLS client config")
	}

	balancers := make([]*clientpoolv2.Balancer, 0, 2)
	if a.config.RouterAddrWithAZ != "" {
		balancers = append(balancers, clientpoolv2.NewBalancer(
			a.config.RouterAddrWithAZ,
			clientpoolv2.WithLookup(a.lookup)),
		)
	}
	balancers = append(balancers, clientpoolv2.NewBalancer(
		a.config.RouterAddr,
		clientpoolv2.WithLookup(a.lookup)),
	)

	avgEnvelopeSize := a.metricClient.NewGauge(
		"average_envelopes",
		"Average envelope size over the past minute.",
		metrics.WithMetricLabels(map[string]string{"unit": "bytes/minute", "metric_version": "2.0", "loggregator": "v2"}),
	)

	tracker := plumbing.NewEnvelopeAverager()
	tracker.Start(60*time.Second, func(i float64) { avgEnvelopeSize.Set(i) })
	statsHandler := clientpool.NewStatsHandler(tracker)

	kp := keepalive.ClientParameters{
		Time:                15 * time.Second,
		Timeout:             15 * time.Second,
		PermitWithoutStream: true,
	}
	fetcher := clientpoolv2.NewSenderFetcher(
		a.metricClient,
		grpc.WithTransportCredentials(a.clientCreds),
		grpc.WithStatsHandler(statsHandler),
		grpc.WithKeepaliveParams(kp),
	)

	connector := clientpoolv2.MakeGRPCConnector(fetcher, balancers)

	var connManagers []clientpoolv2.Conn
	for i := 0; i < 5; i++ {
		connManagers = append(connManagers, clientpoolv2.NewConnManager(
			connector,
			100000+rand.Int63n(1000),
			time.Second,
		))
	}

	return clientpoolv2.New(connManagers...)
}
