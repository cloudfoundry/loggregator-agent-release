// Package otelcolclient contains client code for communicating with an OTel
// Collector.
package otelcolclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"github.com/google/uuid"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type GRPCWriter struct {
	// The client API for the OTel Collector metrics service
	msc colmetricspb.MetricsServiceClient
	tsc coltracepb.TraceServiceClient

	// Context passed to gRPC
	ctx context.Context

	// Cancel func invoked on shutdown
	cancel func()

	// The logger to use for errors
	l *log.Logger
}

// NewGRPCWriter dials the provided gRPC address and returns a *GRPCWriter.
func NewGRPCWriter(addr string, tlsConfig *tls.Config, l *log.Logger) (*GRPCWriter, error) {
	cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &GRPCWriter{
		msc:    colmetricspb.NewMetricsServiceClient(cc),
		tsc:    coltracepb.NewTraceServiceClient(cc),
		ctx:    ctx,
		cancel: cancel,
		l:      l,
	}
	return w, nil
}

func (w GRPCWriter) WriteSpanImmediately(sourceId string, span *tracepb.Span) error {
	resp, err := w.tsc.Export(w.ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{
							Key:   "service.name",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: sourceId}},
						}}},
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{span},
					},
				},
			},
		},
	})

	if err == nil {
		if ps := resp.GetPartialSuccess(); ps != nil && ps.GetRejectedSpans() > 0 {
			return errors.New(ps.GetErrorMessage())
		}
	}
	return err
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
	w MetricWriter
}

// New creates a new Client that will batch metrics.
func New(w MetricWriter) *Client {
	return &Client{
		b: NewMetricBatcher(100, 100*time.Millisecond, w),
		w: w,
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
	case *loggregator_v2.Envelope_Timer:
		return c.sendTimerImmediately(e)
	}

	return nil
}

// Close cancels the underlying context.
// TODO: add flushing of batcher before canceling
func (c *Client) Close() error {
	return c.b.w.Close()
}

func (c *Client) sendTimerImmediately(e *loggregator_v2.Envelope) error {

	if e.Tags["peer_type"] == "client" {
		return nil
	}

	fmt.Println("!!!!!!!!!!!")
	fmt.Printf("%+v\n", e)
	fmt.Println("!!!!!!!!!!!")
	atts := attributes(e)

	sc, err := strconv.Atoi(e.Tags["status_code"])
	if err != nil {
		return err
	}

	statusCode := tracepb.Status_STATUS_CODE_OK
	if sc >= 400 {
		statusCode = tracepb.Status_STATUS_CODE_ERROR
	}

	var u uuid.UUID
	u, err = uuid.Parse(e.Tags["request_id"])
	ub, _ := u.MarshalBinary()

	atts = append(atts, &commonpb.KeyValue{
		Key:   "instance_id",
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: e.GetInstanceId()}},
	})

	if processId, ok := e.Tags["process_id"]; ok {
		atts = append(atts, &commonpb.KeyValue{
			Key:   "process_id",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: processId}},
		})
	}

	if processInstanceId, ok := e.Tags["process_instance_id"]; ok {
		atts = append(atts, &commonpb.KeyValue{
			Key:   "process_instance_id",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: processInstanceId}},
		})
	}

	err = c.w.WriteSpanImmediately(e.GetSourceId(), &tracepb.Span{
		TraceId:           ub[0:16],
		SpanId:            ub[len(ub)-8:],
		Status:            &tracepb.Status{Code: statusCode},
		Kind:              tracepb.Span_SPAN_KIND_SERVER,
		Name:              e.GetSourceId(),
		StartTimeUnixNano: uint64(e.GetTimer().Start),
		EndTimeUnixNano:   uint64(e.GetTimer().Stop),
		Attributes:        atts,
	})
	fmt.Printf("%+v\n", err)
	return err
}

// addCounterToBatch translates a loggregator v2 Counter to OTLP and adds the metric to the pending batch.
func (c *Client) addCounterToBatch(e *loggregator_v2.Envelope) {
	atts := attributes(e)
	c.b.Write(&metricspb.Metric{
		Name: e.GetCounter().GetName(),
		Data: &metricspb.Metric_Sum{
			Sum: &metricspb.Sum{
				IsMonotonic:            true,
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
