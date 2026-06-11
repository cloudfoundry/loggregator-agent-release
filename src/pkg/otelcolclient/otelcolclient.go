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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const (
	defaultMaxRetries        = 7
	defaultInitialRetryDelay = 1 * time.Second
	defaultMaxRetryDelay     = 30 * time.Second
	defaultRetryQueueSize    = 1024
)

// retryItem holds a failed export closure and a count of retry attempts already made.
type retryItem struct {
	exportFn func() error
	attempts int
}

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

	// Retry configuration.
	maxRetries        int
	initialRetryDelay time.Duration
	maxRetryDelay     time.Duration

	// Per-signal queues for async retry workers. Items are enqueued by withRetry
	// and consumed by runRetryWorker goroutines started in NewGRPCWriter.
	// Sized to absorb the full retry window: 7 attempts over 91 s (1+2+4+8+16+30+30)
	// at the 100 ms flush interval produces at most 910 batches. 1024 clears that
	// with a small margin. A nil channel disables async retry (used in tests that
	// construct GRPCWriter directly without starting workers).
	metricsRetry chan retryItem
	logsRetry    chan retryItem
	tracesRetry  chan retryItem
}

// NewGRPCWriter dials the provided gRPC address and returns a *GRPCWriter.
func NewGRPCWriter(addr string, tlsConfig *tls.Config, l *log.Logger) (*GRPCWriter, error) {
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()

	w := &GRPCWriter{
		msc:               colmetricspb.NewMetricsServiceClient(cc),
		tsc:               coltracepb.NewTraceServiceClient(cc),
		lsc:               collogspb.NewLogsServiceClient(cc),
		ctx:               ctx,
		cancel:            cancel,
		l:                 l,
		maxRetries:        defaultMaxRetries,
		initialRetryDelay: defaultInitialRetryDelay,
		maxRetryDelay:     defaultMaxRetryDelay,
		metricsRetry:      make(chan retryItem, defaultRetryQueueSize),
		logsRetry:         make(chan retryItem, defaultRetryQueueSize),
		tracesRetry:       make(chan retryItem, defaultRetryQueueSize),
	}
	go w.runRetryWorker(w.metricsRetry)
	go w.runRetryWorker(w.logsRetry)
	go w.runRetryWorker(w.tracesRetry)
	cancel = nil
	return w, nil
}

func (w GRPCWriter) WriteLogs(batch []*logspb.ResourceLogs) {
	w.withRetry(func() error {
		resp, err := w.lsc.Export(w.ctx, &collogspb.ExportLogsServiceRequest{
			ResourceLogs: batch,
		})
		if err != nil {
			return err
		}
		return errorOnLogsRejection(resp)
	}, w.logsRetry)
}

func (w GRPCWriter) WriteMetrics(batch []*metricspb.Metric) {
	w.withRetry(func() error {
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
		if err != nil {
			return err
		}
		return errorOnRejection(resp)
	}, w.metricsRetry)
}

func (w GRPCWriter) WriteTrace(batch []*tracepb.ResourceSpans) {
	w.withRetry(func() error {
		resp, err := w.tsc.Export(w.ctx, &coltracepb.ExportTraceServiceRequest{
			ResourceSpans: batch,
		})
		if err != nil {
			return err
		}
		return errorOnTraceRejection(resp)
	}, w.tracesRetry)
}

// isRetryable reports whether a gRPC error is transient and worth retrying.
// Only codes.Unavailable is retried — the expected code when the OTel
// Collector is down or restarting. Partial-success rejections (plain errors
// from errorOn*Rejection) return false.
func isRetryable(err error) bool {
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	return s.Code() == codes.Unavailable
}

// isContextError reports whether an error is due to context cancellation so
// that shutdown paths can be distinguished from genuine write failures.
func isContextError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.Canceled
}

// withRetry makes a single export attempt. If the error is retryable the batch
// is handed off to the background retry worker via queue so the caller (running
// inside the SignalBatcher flush path, holding its mutex) is not blocked during
// backoff. Non-retryable errors and context errors are handled inline.
func (w GRPCWriter) withRetry(exportFn func() error, queue chan<- retryItem) {
	err := exportFn()
	if err == nil {
		return
	}
	if isContextError(err) {
		return
	}
	if !isRetryable(err) {
		w.l.Println("Write error:", err)
		return
	}
	select {
	case queue <- retryItem{exportFn: exportFn}:
	default:
		w.l.Println("Write error (retry queue full):", err)
	}
}

// drainRetryQueue non-blockingly moves all pending items from src into dst.
func drainRetryQueue(dst []retryItem, src <-chan retryItem) []retryItem {
	for {
		select {
		case item := <-src:
			dst = append(dst, item)
		default:
			return dst
		}
	}
}

// runRetryWorker retries batches that were queued by withRetry. It maintains a
// pool of pending batches and replays them all on each backoff tick so that
// when the collector recovers the entire backlog is flushed in one sweep rather
// than one batch per backoff cycle.
//
// The shared pool delay resets to initialRetryDelay each time the pool drains
// to empty. Items that exhaust maxRetries are logged and discarded. The
// goroutine exits silently when the writer's context is cancelled.
func (w *GRPCWriter) runRetryWorker(queue <-chan retryItem) {
	var pool []retryItem
	delay := w.initialRetryDelay

	for {
		// Merge any newly queued items into the pool.
		prevLen := len(pool)
		pool = drainRetryQueue(pool, queue)
		if prevLen == 0 && len(pool) > 0 {
			delay = w.initialRetryDelay
			w.l.Println("New item added to empty pool, delay set to", delay)
		}

		if len(pool) == 0 {
			// Block until there is work to do or the context is cancelled.
			select {
			case <-w.ctx.Done():
				return
			case item := <-queue:
				pool = append(pool, item)
				delay = w.initialRetryDelay
				w.l.Println("New item added to empty pool, delay set to", delay)
			}
			pool = drainRetryQueue(pool, queue)
		}

		// Wait before the retry attempt.
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(delay):
		}

		// Drain items that arrived during the sleep.
		pool = drainRetryQueue(pool, queue)

		// Attempt every item in the pool; keep the ones that still need more retries.
		var remaining []retryItem
		for _, item := range pool {
			if isContextError(w.ctx.Err()) {
				return
			}
			err := item.exportFn()
			if err == nil {
				continue
			}
			if isContextError(err) {
				return
			}
			item.attempts++
			if !isRetryable(err) || item.attempts >= w.maxRetries {
				w.l.Println("Write error:", err)
				continue
			}
			remaining = append(remaining, item)
		}
		pool = remaining

		if len(pool) == 0 {
			delay = w.initialRetryDelay
			w.l.Println("Pool is empty, delay set to", delay)
		} else {
			delay = min(delay*2, w.maxRetryDelay)
		}
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
						TimeUnixNano:         uint64(e.GetTimestamp()), // nolint:gosec
						Attributes:           atts,
						ObservedTimeUnixNano: uint64(time.Now().UnixNano()), // nolint:gosec
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
						TimeUnixNano:         uint64(e.GetTimestamp()), // nolint:gosec
						Attributes:           atts,
						ObservedTimeUnixNano: uint64(time.Now().UnixNano()), // nolint:gosec
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
				IsMonotonic:            true,
				DataPoints: []*metricspb.NumberDataPoint{
					{
						TimeUnixNano: uint64(e.GetTimestamp()), // nolint:gosec
						Attributes:   atts,
						Value: &metricspb.NumberDataPoint_AsInt{
							AsInt: int64(e.GetCounter().GetTotal() << 1 >> 1), //#nosec G115
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
							TimeUnixNano: uint64(e.GetTimestamp()), // nolint:gosec
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
						StartTimeUnixNano: uint64(e.GetTimer().GetStart()), // nolint:gosec
						EndTimeUnixNano:   uint64(e.GetTimer().GetStop()),  // nolint:gosec
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
