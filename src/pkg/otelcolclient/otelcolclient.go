// Package otelcolclient contains client code for communicating with an OTel
// Collector.
package otelcolclient

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"time"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type GRPCWriter struct {
	// The client API for the OTel Collector metrics service
	msc colmetricspb.MetricsServiceClient

	// Context passed to gRPC
	ctx context.Context

	// Cancel func invoked on shutdown
	cancel func()

	// The logger to use for errors
	l *log.Logger
}

// NewGRPCWriter dials the provided gRPC address and returns a *GRPCWriter.
func NewGRPCWriter(addr string, tlsConfig *tls.Config, l *log.Logger) (*GRPCWriter, error) {
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &GRPCWriter{
		msc:    colmetricspb.NewMetricsServiceClient(cc),
		ctx:    ctx,
		cancel: cancel,
		l:      l,
	}
	return w, nil
}

func (w GRPCWriter) Write(batch []*metricspb.Metric) {
	resp, err := w.msc.Export(w.ctx, &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						Metrics: batch,
					},
				},
			},
		},
	})
	if err == nil {
		err = errorOnRejection(resp)
	}

	if err != nil {
		w.l.Println("Write error:", err)
	}
}

func (w GRPCWriter) Close() error {
	w.cancel()
	return nil
}

type Client struct {
	// Batch metrics sent to OTel Collector
	b *MetricBatcher
}

// New creates a new Client that will batch metrics.
func New(w MetricWriter) *Client {
	return &Client{
		b: NewMetricBatcher(100, 100*time.Millisecond, w),
	}
}

// Write translates an envelope to OTLP and forwards it to the connected OTel
// Collector.
func (c *Client) Write(e *loggregator_v2.Envelope) error {
	switch e.Message.(type) {
	case *loggregator_v2.Envelope_Counter:
		c.addCounterToBatch(e)
	case *loggregator_v2.Envelope_Gauge:
		c.addGaugeToBatch(e)
	}

	return nil
}

// Close cancels the underlying context.
// TODO: add flushing of batcher before canceling
func (c *Client) Close() error {
	return c.b.w.Close()
}

// addCounterToBatch translates a loggregator v2 Counter to OTLP and adds the metric to the pending batch.
func (c *Client) addCounterToBatch(e *loggregator_v2.Envelope) {
	atts := attributes(e)
	c.b.Write(&metricspb.Metric{
		Name: e.GetCounter().GetName(),
		Data: &metricspb.Metric_Sum{
			Sum: &metricspb.Sum{
				AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
				IsMonotonic:            e.GetCounter().GetDelta() == 0,
				DataPoints: []*metricspb.NumberDataPoint{
					{
						TimeUnixNano: uint64(e.GetTimestamp()),
						Attributes:   atts,
						Value: &metricspb.NumberDataPoint_AsInt{
							AsInt: int64(e.GetCounter().GetTotal()), //#nosec G115
						},
					},
				},
			},
		},
	})
}

// addGaugeToBatch translates a loggregator v2 Gauge to OTLP and adds the metrics to the pending batch.
func (c *Client) addGaugeToBatch(e *loggregator_v2.Envelope) {
	atts := attributes(e)

	for k, v := range e.GetGauge().GetMetrics() {
		c.b.Write(&metricspb.Metric{
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
		if k == "instance_id" || k == "source_id" || k == "__v1_type" {
			continue
		}

		atts = append(atts, &commonpb.KeyValue{
			Key:   k,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}
	return atts
}
