// Package otelcolclient contains client code for communicating with an OTel
// Collector.
package otelcolclient

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"log"
	"net/url"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type GRPCWriter struct {
	// The client API for the OTel Collector metrics service
	msc colmetricspb.MetricsServiceClient

	// The client API for the OTel Collector trace service
	tsc coltracepb.TraceServiceClient

	// The client API for the OTel Collector logs service
	lsc collogspb.LogsServiceClient

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
		tsc:    coltracepb.NewTraceServiceClient(cc),
		lsc:    collogspb.NewLogsServiceClient(cc),
		ctx:    ctx,
		cancel: cancel,
		l:      l,
	}
	return w, nil
}

func (w GRPCWriter) WriteLogs(batch []*logspb.ResourceLogs) {
	resp, err := w.lsc.Export(w.ctx, &collogspb.ExportLogsServiceRequest{
		ResourceLogs: batch,
	})
	if err == nil {
		err = errorOnLogsRejection(resp)
	}
	if err != nil {
		w.l.Println("Write error:", err)
	}
}

func (w GRPCWriter) WriteMetrics(batch []*metricspb.Metric) {
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

func (w GRPCWriter) WriteTrace(batch []*tracepb.ResourceSpans) {
	resp, err := w.tsc.Export(w.ctx, &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: batch,
	})
	if err == nil {
		err = errorOnTraceRejection(resp)
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
	b           *SignalBatcher
	emitTraces  bool
	emitMetrics bool
	emitLogs    bool
}

// New creates a new Client that will batch metrics and logs.
func New(w Writer, emitTraces bool, emitMetrics bool, emitLogs bool) *Client {
	return &Client{
		b:           NewSignalBatcher(100, 100*time.Millisecond, w),
		emitTraces:  emitTraces,
		emitMetrics: emitMetrics,
		emitLogs:    emitLogs,
	}
}

// Write translates an envelope to OTLP and forwards it to the connected OTel
// Collector.
func (c *Client) Write(e *loggregator_v2.Envelope) error {
	switch e.Message.(type) {
	case *loggregator_v2.Envelope_Counter:
		c.writeCounter(e)
	case *loggregator_v2.Envelope_Gauge:
		c.writeGauge(e)
	case *loggregator_v2.Envelope_Timer:
		c.writeTimer(e)
	case *loggregator_v2.Envelope_Log:
		c.writeLog(e)
	case *loggregator_v2.Envelope_Event:
		c.writeEvent(e)
	}
	return nil
}

// Close cancels the underlying context.
// TODO: add flushing of batcher before canceling
func (c *Client) Close() error {
	return c.b.w.Close()
}

// writeCounter translates a loggregator v2 Counter to OTLP and adds the metric to the pending batch.
func (c *Client) writeLog(e *loggregator_v2.Envelope) {
	if !c.emitLogs {
		return
	}
	atts := attributes(e)
	svrtyNumber := logspb.SeverityNumber_SEVERITY_NUMBER_UNSPECIFIED
	switch e.GetLog().GetType() {
	case loggregator_v2.Log_OUT:
		svrtyNumber = logspb.SeverityNumber_SEVERITY_NUMBER_INFO
	case loggregator_v2.Log_ERR:
		svrtyNumber = logspb.SeverityNumber_SEVERITY_NUMBER_ERROR
	}
	c.b.WriteLog(&logspb.ResourceLogs{
		ScopeLogs: []*logspb.ScopeLogs{
			{
				LogRecords: []*logspb.LogRecord{
					{
						TimeUnixNano:         uint64(e.GetTimestamp()),
						Attributes:           atts,
						ObservedTimeUnixNano: uint64(time.Now().UnixNano()),
						SeverityText:         svrtyNumber.String(),
						SeverityNumber:       svrtyNumber,
						Body: &commonpb.AnyValue{
							Value: &commonpb.AnyValue_StringValue{
								StringValue: string(e.GetLog().GetPayload()),
							},
						},
					},
				},
			},
		},
	})
}

func (c *Client) writeEvent(e *loggregator_v2.Envelope) {
	if !c.emitLogs {
		return
	}
	atts := attributes(e)
	body := e.GetEvent().GetBody()
	title := e.GetEvent().GetTitle()
	c.b.WriteLog(&logspb.ResourceLogs{
		ScopeLogs: []*logspb.ScopeLogs{
			{
				LogRecords: []*logspb.LogRecord{
					{
						TimeUnixNano:         uint64(e.GetTimestamp()),
						Attributes:           atts,
						ObservedTimeUnixNano: uint64(time.Now().UnixNano()),
						Body: &commonpb.AnyValue{
							Value: &commonpb.AnyValue_KvlistValue{
								KvlistValue: &commonpb.KeyValueList{
									Values: []*commonpb.KeyValue{
										{
											Key:   "title",
											Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: title}},
										},
										{
											Key:   "body",
											Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: body}},
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
}

// writeCounter translates a loggregator v2 Counter to OTLP and adds the metric to the pending batch.
func (c *Client) writeCounter(e *loggregator_v2.Envelope) {
	if !c.emitMetrics {
		return
	}
	atts := attributes(e)
	c.b.WriteMetric(&metricspb.Metric{
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

// writeGauge translates a loggregator v2 Gauge to OTLP and adds the metrics to the pending batch.
func (c *Client) writeGauge(e *loggregator_v2.Envelope) {
	if !c.emitMetrics {
		return
	}
	atts := attributes(e)

	for k, v := range e.GetGauge().GetMetrics() {
		c.b.WriteMetric(&metricspb.Metric{
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

// writeTimer translates a loggregator v2 Timer to OTLP and adds the spans to the pending batch.
func (c *Client) writeTimer(e *loggregator_v2.Envelope) {
	if !c.emitTraces {
		return
	}

	if ok := validateTimerTags(e.GetTags()); !ok {
		return
	}

	spanId, err := hex.DecodeString(e.GetTags()["span_id"])
	if err != nil {
		return
	}
	traceId, err := hex.DecodeString(e.GetTags()["trace_id"])
	if err != nil {
		return
	}

	name := e.GetTimer().GetName()
	if uri, ok := e.GetTags()["uri"]; ok {
		if u, err := url.Parse(uri); err == nil {
			name = u.Path
		}
	}

	kind := tracepb.Span_SPAN_KIND_INTERNAL
	if e.GetTags()["peer_type"] == "Server" {
		kind = tracepb.Span_SPAN_KIND_SERVER
	}

	atts := attributes(e)
	c.b.WriteTrace(&tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					{
						TraceId:           traceId,
						SpanId:            spanId,
						Name:              name,
						Kind:              kind,
						StartTimeUnixNano: uint64(e.GetTimer().GetStart()),
						EndTimeUnixNano:   uint64(e.GetTimer().GetStop()),
						Status: &tracepb.Status{
							Code: tracepb.Status_STATUS_CODE_UNSET,
						},
						Attributes: atts,
					},
				},
			},
		},
	})
}

func errorOnRejection(r *colmetricspb.ExportMetricsServiceResponse) error {
	if ps := r.GetPartialSuccess(); ps != nil && ps.GetRejectedDataPoints() > 0 {
		return errors.New(ps.GetErrorMessage())
	}
	return nil
}

func errorOnTraceRejection(r *coltracepb.ExportTraceServiceResponse) error {
	if ps := r.GetPartialSuccess(); ps != nil && ps.GetRejectedSpans() > 0 {
		return errors.New(ps.GetErrorMessage())
	}
	return nil
}

func errorOnLogsRejection(r *collogspb.ExportLogsServiceResponse) error {
	if ps := r.GetPartialSuccess(); ps != nil && ps.GetRejectedLogRecords() > 0 {
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
		if k == "instance_id" || k == "source_id" || k == "__v1_type" || k == "span_id" || k == "trace_id" {
			continue
		}

		atts = append(atts, &commonpb.KeyValue{
			Key:   k,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}
	return atts
}

func validateTimerTags(tags map[string]string) bool {
	if tags == nil {
		return false
	}
	if tags["peer_type"] == "Client" {
		return false
	}
	if tags["span_id"] == "" {
		return false
	}
	if tags["trace_id"] == "" {
		return false
	}
	return true
}
