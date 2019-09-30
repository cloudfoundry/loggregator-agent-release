package v1

import (
	"code.cloudfoundry.org/go-metric-registry"
	"context"
	"fmt"
	"io"
	"log"

	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"

	"google.golang.org/grpc"
)

type MetricClient interface {
	NewGauge(name, helpText string,  opts ...metrics.MetricOption) metrics.Gauge
}

type PusherFetcher struct {
	opts               []grpc.DialOption
	dopplerConnections func(float64)
	dopplerV1Streams   func(float64)
}

func NewPusherFetcher(mc MetricClient, opts ...grpc.DialOption) *PusherFetcher {
	dopplerV1Streams := mc.NewGauge(
		"doppler_v1_streams",
		"Current number of established gRPC streams from v1 agent.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "2.0"}),
	)

	dopplerConnections := mc.NewGauge(
		"doppler_connections",
		"Current number of gRPC connections from v1 and v2 agents.",
		metrics.WithMetricLabels(map[string]string{"metric_version": "2.0"}),
	)

	return &PusherFetcher{
		opts:               opts,
		dopplerConnections: func(i float64){ dopplerConnections.Add(i) },
		dopplerV1Streams:   func(i float64){ dopplerV1Streams.Add(i) },
	}
}

func (p *PusherFetcher) Fetch(addr string) (io.Closer, plumbing.DopplerIngestor_PusherClient, error) {
	conn, err := grpc.Dial(addr, p.opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("error dialing ingestor stream to %s: %s", addr, err)
	}
	p.dopplerConnections(1)

	client := plumbing.NewDopplerIngestorClient(conn)

	pusher, err := client.Pusher(context.Background())
	if err != nil {
		p.dopplerConnections(-1)
		conn.Close()
		return nil, nil, fmt.Errorf("error establishing ingestor stream to %s: %s", addr, err)
	}
	p.dopplerV1Streams(1)

	log.Printf("successfully established a stream to doppler %s", addr)

	closer := &decrementingCloser{
		closer:             conn,
		dopplerConnections: p.dopplerConnections,
		dopplerV1Streams:   p.dopplerV1Streams,
	}
	return closer, pusher, err
}

type decrementingCloser struct {
	closer             io.Closer
	dopplerConnections func(float64)
	dopplerV1Streams   func(float64)
}

func (d *decrementingCloser) Close() error {
	d.dopplerConnections(-1)
	d.dopplerV1Streams(-1)

	return d.closer.Close()
}
