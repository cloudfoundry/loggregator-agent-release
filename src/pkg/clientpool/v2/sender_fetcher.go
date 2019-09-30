package v2

import (
	"code.cloudfoundry.org/go-metric-registry"
	"context"
	"fmt"
	"io"
	"log"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"google.golang.org/grpc"
)

type MetricClient interface {
	NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge
}

type SenderFetcher struct {
	opts               []grpc.DialOption
	dopplerConnections func(float64)
	dopplerV2Streams   func(float64)
}

func NewSenderFetcher(mc MetricClient, opts ...grpc.DialOption) *SenderFetcher {
	dopplerV2Streams := mc.NewGauge(
		"doppler_v2_streams",
		"Current number of established gRPC streams from v2 agent.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "2.0"}),
	)

	dopplerConnections := mc.NewGauge(
		"doppler_connections",
		"Current number of gRPC connections from v1 and v2 agents.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "2.0"}),
	)

	fetcher := SenderFetcher{
		opts:               opts,
		dopplerConnections: func(i float64) { dopplerConnections.Add(i) },
		dopplerV2Streams:  func(i float64)  { dopplerV2Streams.Add(i) },
	}
	return &fetcher
}

func (p *SenderFetcher) Fetch(addr string) (io.Closer, loggregator_v2.Ingress_BatchSenderClient, error) {
	conn, err := grpc.Dial(addr, p.opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("error dialing ingestor stream to %s: %s", addr, err)
	}

	client := loggregator_v2.NewIngressClient(conn)
	sender, err := client.BatchSender(context.Background())
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("failed to establish stream to doppler (%s): %s", addr, err)
	}

	p.dopplerConnections(1)
	p.dopplerV2Streams(1)

	log.Printf("successfully established a stream to doppler %s", addr)

	closer := &decrementingCloser{
		closer:             conn,
		dopplerConnections: p.dopplerConnections,
		dopplerV2Streams:   p.dopplerV2Streams,
	}
	return closer, sender, err
}

type decrementingCloser struct {
	closer             io.Closer
	dopplerConnections func(float64)
	dopplerV2Streams   func(float64)
}

func (d *decrementingCloser) Close() error {
	d.dopplerConnections(-1)
	d.dopplerV2Streams(-1)

	return d.closer.Close()
}
