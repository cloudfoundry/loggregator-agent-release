// Package otelcolclient contains client code for communicating with an OTel
// Collector.
package otelcolclient

import (
	"context"
	"crypto/tls"
	"errors"
	"log"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Client struct {
	// The client API for the OTel Collector metrics service
	msc colmetricspb.MetricsServiceClient
	// Context passed to gRPC
	ctx context.Context
	// Cancel func invoked on shutdown
	cancel func()
	// The logger to use for errors
	l *log.Logger
}

// New dials the provided gRPC address and returns a *Client or error based off
// that client connection.
func New(addr string, tlsConfig *tls.Config, l *log.Logger) (*Client, error) {
	cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		msc:    colmetricspb.NewMetricsServiceClient(cc),
		ctx:    ctx,
		cancel: cancel,
		l:      l,
	}, nil
}

// Write translates an envelope to OTLP and forwards it to the connected OTel
// Collector.
func (c *Client) Write(e *loggregator_v2.Envelope) error {
	var err error
	switch e.Message.(type) {
	case *loggregator_v2.Envelope_Counter:
		err = c.writeCounter(e)
	case *loggregator_v2.Envelope_Gauge:
		err = c.writeGauge(e)
	}
	// Need to log the error right now because the Forwarder Agent drops
	// returned errors. If that changes we can remove this conditional.
	if err != nil {
		c.l.Println("Write error:", err)
	}
	return err
}

// Close cancels the underlying context.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// writeCounter translates a loggregator v2 Counter to OTLP and forwards it.
func (c *Client) writeCounter(e *loggregator_v2.Envelope) error {
	atts := attributes(e)
	resp, err := c.msc.Export(c.ctx, &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: []*metricspb.Metric{
							{
								Name: e.GetCounter().GetName(),
								Data: &metricspb.Metric_Sum{
									Sum: &metricspb.Sum{
										AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
										DataPoints: []*metricspb.NumberDataPoint{
											{
												TimeUnixNano: uint64(e.GetTimestamp()),
												Attributes:   atts,
												Value: &metricspb.NumberDataPoint_AsInt{
													AsInt: int64(e.GetCounter().GetTotal()),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return errorOnRejection(resp)
}

// writeGauge translates a loggregator v2 Gauge to OTLP and forwards it.
func (c *Client) writeGauge(e *loggregator_v2.Envelope) error {
	atts := attributes(e)

	var metrics []*metricspb.Metric
	for k, v := range e.GetGauge().GetMetrics() {
		metrics = append(metrics, &metricspb.Metric{
			Name: k,
			Unit: v.GetUnit(),
			Data: &metricspb.Metric_Gauge{
				Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{
						{
							TimeUnixNano: uint64(e.GetTimestamp()),
							Attributes:   atts,
							Value: &metricspb.NumberDataPoint_AsDouble{
								AsDouble: v.GetValue(),
							},
						},
					},
				},
			},
		})
	}

	resp, err := c.msc.Export(c.ctx, &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: metrics,
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}
	return errorOnRejection(resp)
}

func errorOnRejection(r *colmetricspb.ExportMetricsServiceResponse) error {
	if ps := r.GetPartialSuccess(); ps != nil && ps.GetRejectedDataPoints() > 0 {
		return errors.New(ps.GetErrorMessage())
	}
	return nil
}

// attributes converts envelope tags to OTel key/value attributes.
func attributes(e *loggregator_v2.Envelope) []*commonpb.KeyValue {
	atts := []*commonpb.KeyValue{
		{
			Key:   "instance_id",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: e.GetInstanceId()}},
		},
		{
			Key:   "source_id",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: e.GetSourceId()}},
		},
	}

	for k, v := range e.Tags {
		if k == "instance_id" || k == "source_id" {
			continue
		}

		atts = append(atts, &commonpb.KeyValue{
			Key:   k,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}
	return atts
}
