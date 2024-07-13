package otelcolclient

import (
	"context"
	"errors"
	"log"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/testing/protocmp"
)

var _ = Describe("Client", func() {
	var (
		c      Client
		b      *SignalBatcher
		spyMSC *spyMetricsServiceClient
		spyTSC *spyTraceServiceClient
		spyLSC *spyLogsServiceClient
		buf    *gbytes.Buffer
	)

	BeforeEach(func() {
		buf = gbytes.NewBuffer()
		GinkgoWriter.TeeTo(buf)
		spyMSC = &spyMetricsServiceClient{
			requests:    make(chan *colmetricspb.ExportMetricsServiceRequest, 1),
			response:    &colmetricspb.ExportMetricsServiceResponse{},
			responseErr: nil,
		}
		spyTSC = &spyTraceServiceClient{
			requests:    make(chan *coltracepb.ExportTraceServiceRequest, 1),
			response:    &coltracepb.ExportTraceServiceResponse{},
			responseErr: nil,
		}
		spyLSC = &spyLogsServiceClient{
			requests:    make(chan *collogspb.ExportLogsServiceRequest, 1),
			response:    &collogspb.ExportLogsServiceResponse{},
			responseErr: nil,
		}
		ctx, cancel := context.WithCancel(context.Background())
		w := GRPCWriter{
			msc:    spyMSC,
			tsc:    spyTSC,
			lsc:    spyLSC,
			ctx:    ctx,
			cancel: cancel,
			l:      log.New(GinkgoWriter, "", 0),
		}
		b = NewSignalBatcher(
			1,
			100*time.Millisecond,
			w,
		)
		c = Client{b: b, emitTraces: true, emitEvents: true}
	})

	AfterEach(func() {
		GinkgoWriter.ClearTeeWriters()
	})

	Describe("Write", func() {
		var (
			envelope    *loggregator_v2.Envelope
			returnedErr error
		)

		JustBeforeEach(func() {
			returnedErr = c.Write(envelope)
		})

		Context("when given a gauge", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"deployment": "cf-1234",
						"ip":         "10.0.1.5",
					},
					Message: &loggregator_v2.Envelope_Gauge{
						Gauge: &loggregator_v2.Gauge{
							Metrics: map[string]*loggregator_v2.GaugeValue{
								"cpu": {
									Unit:  "percentage",
									Value: 0.3257,
								},
								"memory": {
									Unit:  "bytes",
									Value: 71755,
								},
							},
						},
					},
				}

				ctx, cancel := context.WithCancel(context.Background())
				w := GRPCWriter{
					msc:    spyMSC,
					ctx:    ctx,
					cancel: cancel,
					l:      log.New(GinkgoWriter, "", 0),
				}
				b := NewSignalBatcher(
					2,
					100*time.Millisecond,
					w,
				)
				c = Client{b: b, emitTraces: true}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("converts the envelope to OTLP and passes it to the Metric Service Client", func() {
				var msr *colmetricspb.ExportMetricsServiceRequest
				Expect(spyMSC.requests).To(Receive(&msr))

				expectedReq := &colmetricspb.ExportMetricsServiceRequest{
					ResourceMetrics: []*metricspb.ResourceMetrics{
						{
							ScopeMetrics: []*metricspb.ScopeMetrics{
								{
									Metrics: []*metricspb.Metric{
										{
											Name: "cpu",
											Unit: "percentage",
											Data: &metricspb.Metric_Gauge{
												Gauge: &metricspb.Gauge{
													DataPoints: []*metricspb.NumberDataPoint{
														{
															TimeUnixNano: 1257894000000000000,
															Attributes: []*commonpb.KeyValue{
																{
																	Key:   "deployment",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "cf-1234"}},
																},
																{
																	Key:   "instance_id",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
																},
																{
																	Key:   "ip",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "10.0.1.5"}},
																},
																{
																	Key:   "source_id",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
																},
															},
															Value: &metricspb.NumberDataPoint_AsDouble{
																AsDouble: 0.3257,
															},
														},
													},
												},
											},
										},
										{
											Name: "memory",
											Unit: "bytes",
											Data: &metricspb.Metric_Gauge{
												Gauge: &metricspb.Gauge{

													DataPoints: []*metricspb.NumberDataPoint{
														{
															TimeUnixNano: 1257894000000000000,
															Attributes: []*commonpb.KeyValue{
																{
																	Key:   "deployment",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "cf-1234"}},
																},
																{
																	Key:   "instance_id",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
																},
																{
																	Key:   "ip",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "10.0.1.5"}},
																},
																{
																	Key:   "source_id",
																	Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
																},
															},
															Value: &metricspb.NumberDataPoint_AsDouble{
																AsDouble: 71755,
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
				}

				s1 := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
					return x.Key < y.Key
				})
				s2 := protocmp.SortRepeated(func(x *metricspb.Metric, y *metricspb.Metric) bool {
					return x.Name < y.Name
				})
				Expect(cmp.Diff(expectedReq, msr, protocmp.Transform(), s1, s2)).To(BeEmpty())
			})

			Context("when Metric Service Client returns an error", func() {
				BeforeEach(func() {
					spyMSC.responseErr = errors.New("test-error")
				})

				It("logs it", func() {
					Eventually(buf).Should(gbytes.Say("Write error: test-error"))
				})
			})

			Context("when Metric Service Client indicates data points were rejected", func() {
				BeforeEach(func() {
					spyMSC.response = &colmetricspb.ExportMetricsServiceResponse{
						PartialSuccess: &colmetricspb.ExportMetricsPartialSuccess{
							RejectedDataPoints: 1,
							ErrorMessage:       "test-error-message",
						},
					}
				})

				It("logs it", func() {
					Eventually(buf).Should(gbytes.Say("Write error: test-error-message"))
				})
			})

			Context("when the instance id or source id are provided as tags", func() {
				BeforeEach(func() {
					envelope.Tags = map[string]string{}
					envelope.Tags["source_id"] = "some-other-source-id"
					envelope.Tags["instance_id"] = "some-other-instance-id"
				})

				It("ignores them and uses the envelope fields instead", func() {
					var msr *colmetricspb.ExportMetricsServiceRequest
					Expect(spyMSC.requests).To(Receive(&msr))

					expectedAtts := []*commonpb.KeyValue{
						{
							Key:   "instance_id",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
						},
						{
							Key:   "source_id",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
						},
					}
					sortFunc := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
						return x.Key < y.Key
					})
					actualAtts := msr.GetResourceMetrics()[0].GetScopeMetrics()[0].GetMetrics()[0].GetGauge().GetDataPoints()[0].GetAttributes()
					Expect(cmp.Diff(actualAtts, expectedAtts, protocmp.Transform(), sortFunc)).To(BeEmpty())
				})
			})

			Context("when the envelope has been converted from a v1 representation", func() {
				BeforeEach(func() {
					envelope.Tags["__v1_type"] = "ValueMetric"
				})

				It("drops the __v1_type tag", func() {
					var msr *colmetricspb.ExportMetricsServiceRequest
					Expect(spyMSC.requests).To(Receive(&msr))

					actualAtts := msr.GetResourceMetrics()[0].GetScopeMetrics()[0].GetMetrics()[0].GetGauge().GetDataPoints()[0].GetAttributes()
					Expect(actualAtts).ToNot(ContainElement(HaveField("Key", "__v1_type")))
				})
			})
		})

		Context("when given a counter", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"direction": "egress",
						"origin":    "fake-origin.some-vm",
					},
					Message: &loggregator_v2.Envelope_Counter{
						Counter: &loggregator_v2.Counter{
							Name:  "dropped",
							Total: 10,
						},
					},
				}
			})

			Context("when the envelope has a total but no delta", func() {
				It("returns nil", func() {
					Expect(returnedErr).NotTo(HaveOccurred())
				})

				It("emits a monotonic sum", func() {
					var msr *colmetricspb.ExportMetricsServiceRequest
					Expect(spyMSC.requests).To(Receive(&msr))

					expectedReq := &colmetricspb.ExportMetricsServiceRequest{
						ResourceMetrics: []*metricspb.ResourceMetrics{
							{
								ScopeMetrics: []*metricspb.ScopeMetrics{
									{
										Metrics: []*metricspb.Metric{
											{
												Name: "dropped",
												Data: &metricspb.Metric_Sum{
													Sum: &metricspb.Sum{
														AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
														IsMonotonic:            true,
														DataPoints: []*metricspb.NumberDataPoint{
															{
																TimeUnixNano: 1257894000000000000,
																Attributes: []*commonpb.KeyValue{
																	{
																		Key:   "direction",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "egress"}},
																	},
																	{
																		Key:   "instance_id",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
																	},
																	{
																		Key:   "origin",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-origin.some-vm"}},
																	},
																	{
																		Key:   "source_id",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
																	},
																},
																Value: &metricspb.NumberDataPoint_AsInt{
																	AsInt: 10,
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
					}

					s1 := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
						return x.Key < y.Key
					})
					s2 := protocmp.SortRepeated(func(x *metricspb.Metric, y *metricspb.Metric) bool {
						return x.Name < y.Name
					})
					Expect(cmp.Diff(msr, expectedReq, protocmp.Transform(), s1, s2)).To(BeEmpty())
				})
			})
			Context("when the envelope has a delta and a calculated total", func() {
				BeforeEach(func() {
					envelope = &loggregator_v2.Envelope{
						Timestamp:  1257894000000000000,
						SourceId:   "fake-source-id",
						InstanceId: "fake-instance-id",
						Tags: map[string]string{
							"direction": "egress",
							"origin":    "fake-origin.some-vm",
						},
						Message: &loggregator_v2.Envelope_Counter{
							Counter: &loggregator_v2.Counter{
								Name:  "dropped",
								Delta: 5,
								Total: 10,
							},
						},
					}
				})

				It("returns nil", func() {
					Expect(returnedErr).NotTo(HaveOccurred())
				})

				It("emits a non-monotonic sum", func() {
					var msr *colmetricspb.ExportMetricsServiceRequest
					Expect(spyMSC.requests).To(Receive(&msr))

					expectedReq := &colmetricspb.ExportMetricsServiceRequest{
						ResourceMetrics: []*metricspb.ResourceMetrics{
							{
								ScopeMetrics: []*metricspb.ScopeMetrics{
									{
										Metrics: []*metricspb.Metric{
											{
												Name: "dropped",
												Data: &metricspb.Metric_Sum{
													Sum: &metricspb.Sum{
														AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
														IsMonotonic:            false,
														DataPoints: []*metricspb.NumberDataPoint{
															{
																TimeUnixNano: 1257894000000000000,
																Attributes: []*commonpb.KeyValue{
																	{
																		Key:   "direction",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "egress"}},
																	},
																	{
																		Key:   "instance_id",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
																	},
																	{
																		Key:   "origin",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-origin.some-vm"}},
																	},
																	{
																		Key:   "source_id",
																		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
																	},
																},
																Value: &metricspb.NumberDataPoint_AsInt{
																	AsInt: 10,
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
					}

					s1 := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
						return x.Key < y.Key
					})
					s2 := protocmp.SortRepeated(func(x *metricspb.Metric, y *metricspb.Metric) bool {
						return x.Name < y.Name
					})
					Expect(cmp.Diff(msr, expectedReq, protocmp.Transform(), s1, s2)).To(BeEmpty())
				})
			})
			Context("when Metric Service Client returns an error", func() {
				BeforeEach(func() {
					spyMSC.responseErr = errors.New("test-error")
				})

				It("logs it", func() {
					Eventually(buf).Should(gbytes.Say("Write error: test-error"))
				})
			})

			Context("when Metric Service Client indicates data points were rejected", func() {
				BeforeEach(func() {
					spyMSC.response = &colmetricspb.ExportMetricsServiceResponse{
						PartialSuccess: &colmetricspb.ExportMetricsPartialSuccess{
							RejectedDataPoints: 1,
							ErrorMessage:       "test-error-message",
						},
					}
				})

				It("logs it", func() {
					Eventually(buf).Should(gbytes.Say("Write error: test-error-message"))
				})
			})

			Context("when the instance id or source id are provided as tags", func() {
				BeforeEach(func() {
					envelope.Tags = map[string]string{}
					envelope.Tags["source_id"] = "some-other-source-id"
					envelope.Tags["instance_id"] = "some-other-instance-id"
				})

				It("ignores them and uses the envelope fields instead", func() {
					var msr *colmetricspb.ExportMetricsServiceRequest
					Expect(spyMSC.requests).To(Receive(&msr))

					expectedAtts := []*commonpb.KeyValue{
						{
							Key:   "instance_id",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
						},
						{
							Key:   "source_id",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
						},
					}
					sortFunc := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
						return x.Key < y.Key
					})
					actualAtts := msr.GetResourceMetrics()[0].GetScopeMetrics()[0].GetMetrics()[0].GetSum().GetDataPoints()[0].GetAttributes()
					Expect(cmp.Diff(actualAtts, expectedAtts, protocmp.Transform(), sortFunc)).To(BeEmpty())
				})
			})
		})

		Context("when given a timer", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"origin":     "gorouter",
						"peer_type":  "Server",
						"request_id": "97118ab4-b679-4761-4443-40131fd8e1d5",
						"uri":        "http://dora.example.com/",
						"span_id":    "deadbeefdeadbeef",
						"trace_id":   "beefdeadbeefdeadbeefdeadbeefdead",
					},
					Message: &loggregator_v2.Envelope_Timer{
						Timer: &loggregator_v2.Timer{
							Name:  "http",
							Start: 1710799972405641252,
							Stop:  1710799972408946683,
						},
					},
				}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("emits a trace", func() {
				var tsr *coltracepb.ExportTraceServiceRequest
				Expect(spyTSC.requests).To(Receive(&tsr))

				expectedReq := &coltracepb.ExportTraceServiceRequest{
					ResourceSpans: []*tracepb.ResourceSpans{
						{
							ScopeSpans: []*tracepb.ScopeSpans{
								{
									Spans: []*tracepb.Span{
										{
											TraceId:           []byte("\xbe\xef\xde\xad\xbe\xef\xde\xad\xbe\xef\xde\xad\xbe\xef\xde\xad"),
											SpanId:            []byte("\xde\xad\xbe\xef\xde\xad\xbe\xef"),
											Name:              "/",
											Kind:              tracepb.Span_SPAN_KIND_SERVER,
											StartTimeUnixNano: 1710799972405641252,
											EndTimeUnixNano:   1710799972408946683,
											Status: &tracepb.Status{
												Code: tracepb.Status_STATUS_CODE_UNSET,
											},
											Attributes: []*commonpb.KeyValue{
												{
													Key:   "instance_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
												},
												{
													Key:   "origin",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gorouter"}},
												},
												{
													Key:   "source_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
												},
												{
													Key:   "peer_type",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "Server"}},
												},
												{
													Key:   "request_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "97118ab4-b679-4761-4443-40131fd8e1d5"}},
												},
												{
													Key:   "uri",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "http://dora.example.com/"}},
												},
											},
										},
									},
								},
							},
						},
					},
				}

				s1 := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
					return x.Key < y.Key
				})
				s2 := protocmp.SortRepeated(func(x *metricspb.Metric, y *metricspb.Metric) bool {
					return x.Name < y.Name
				})
				Expect(cmp.Diff(tsr, expectedReq, protocmp.Transform(), s1, s2)).To(BeEmpty())
			})

			noForwardTests := []struct {
				name     string
				spanID   string
				traceID  string
				peerType string
			}{
				{
					name:     "when the timer has no span_id tag",
					traceID:  "beefdeadbeefdeadbeefdeadbeefdead",
					peerType: "Server",
				},
				{
					name:     "when the timer has a malformed span_id tag",
					traceID:  "beefdeadbeefdeadbeefdeadbeefdead",
					spanID:   "gggggggggggggggg",
					peerType: "Server",
				},
				{
					name:     "when the timer has no trace_id tag",
					spanID:   "deadbeefdeadbeef",
					peerType: "Server",
				},
				{
					name:     "when the timer has a malformed trace_id tag",
					traceID:  "gggggggggggggggggggggggggggggggg",
					spanID:   "deadbeefdeadbeef",
					peerType: "Server",
				},
				{
					name:     "when the timer has a peer_type tag of 'Client'",
					traceID:  "beefdeadbeefdeadbeefdeadbeefdead",
					spanID:   "deadbeefdeadbeef",
					peerType: "Client",
				},
			}
			for _, tc := range noForwardTests {
				tc := tc
				Context(tc.name, func() {
					BeforeEach(func() {
						envelope.Tags["span_id"] = tc.spanID
						envelope.Tags["trace_id"] = tc.traceID
						envelope.Tags["peer_type"] = tc.peerType
					})

					It("does not forward a trace", func() {
						Expect(spyTSC.requests).NotTo(Receive())
					})
				})
			}

			Context("when support for forwarding traces is not active", func() {
				BeforeEach(func() {
					c = Client{b: b, emitTraces: false}
				})
				It("does not forward a trace", func() {
					Expect(spyTSC.requests).NotTo(Receive())
				})
			})

			Context("when the timer has no peer_type tag", func() {
				BeforeEach(func() {
					delete(envelope.Tags, "peer_type")
				})

				It("forwards a trace with Kind set to Internal", func() {
					var tsr *coltracepb.ExportTraceServiceRequest
					Expect(spyTSC.requests).To(Receive(&tsr))
					Expect(span(tsr).GetKind()).To(Equal(tracepb.Span_SPAN_KIND_INTERNAL))
				})
			})

			Context("when there's no uri tag", func() {
				BeforeEach(func() {
					delete(envelope.Tags, "uri")
				})

				It("sets Name to the name of the timer", func() {
					var tsr *coltracepb.ExportTraceServiceRequest
					Expect(spyTSC.requests).To(Receive(&tsr))
					Expect(span(tsr).GetName()).To(Equal("http"))
				})
			})

			Context("when there's a malformed uri tag", func() {
				BeforeEach(func() {
					envelope.Tags["uri"] = "\t"
				})

				It("sets Name to the name of the timer", func() {
					var tsr *coltracepb.ExportTraceServiceRequest
					Expect(spyTSC.requests).To(Receive(&tsr))
					Expect(span(tsr).GetName()).To(Equal("http"))
				})
			})

			Context("when the instance id or source id are provided as tags", func() {
				BeforeEach(func() {
					envelope.Tags["source_id"] = "some-other-source-id"
					envelope.Tags["instance_id"] = "some-other-instance-id"
				})

				It("ignores them and uses the envelope fields instead", func() {
					var tsr *coltracepb.ExportTraceServiceRequest
					Expect(spyTSC.requests).To(Receive(&tsr))
					Expect(span(tsr).GetAttributes()).To(ContainElements(
						&commonpb.KeyValue{
							Key:   "instance_id",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
						},
						&commonpb.KeyValue{
							Key:   "source_id",
							Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
						},
					))
				})
			})
		})

		Context("when given a stdout log", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"direction": "egress",
						"origin":    "fake-origin.some-vm",
					},
					Message: &loggregator_v2.Envelope_Log{
						Log: &loggregator_v2.Log{
							Payload: []byte("log message"),
							Type:    loggregator_v2.Log_OUT,
						},
					},
				}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("emits an info log", func() {
				var lsr *collogspb.ExportLogsServiceRequest
				Expect(spyLSC.requests).To(Receive(&lsr))

				expectedReq := &collogspb.ExportLogsServiceRequest{
					ResourceLogs: []*logspb.ResourceLogs{
						{
							ScopeLogs: []*logspb.ScopeLogs{
								{
									LogRecords: []*logspb.LogRecord{
										{
											ObservedTimeUnixNano: uint64(time.Now().UnixNano()),
											TimeUnixNano:         uint64(1257894000000000000),
											SeverityText:         logspb.SeverityNumber_SEVERITY_NUMBER_INFO.String(),
											SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
											Body: &commonpb.AnyValue{
												Value: &commonpb.AnyValue_StringValue{
													StringValue: "log message",
												},
											},
											Attributes: []*commonpb.KeyValue{
												{
													Key:   "direction",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "egress"}},
												},
												{
													Key:   "instance_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
												},
												{
													Key:   "origin",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-origin.some-vm"}},
												},
												{
													Key:   "source_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				dict := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
					return x.Key < y.Key
				})
				Expect(lsr.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano).NotTo(BeZero())
				expectedReq.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano = lsr.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano
				Expect(cmp.Diff(lsr, expectedReq, protocmp.Transform(), dict)).To(BeEmpty())
			})
		})

		Context("when given a stderr log", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"direction": "egress",
						"origin":    "fake-origin.some-vm",
					},
					Message: &loggregator_v2.Envelope_Log{
						Log: &loggregator_v2.Log{
							Payload: []byte("log error message"),
							Type:    loggregator_v2.Log_ERR,
						},
					},
				}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("emits an error log", func() {
				var lsr *collogspb.ExportLogsServiceRequest
				Expect(spyLSC.requests).To(Receive(&lsr))

				expectedReq := &collogspb.ExportLogsServiceRequest{
					ResourceLogs: []*logspb.ResourceLogs{
						{
							ScopeLogs: []*logspb.ScopeLogs{
								{
									LogRecords: []*logspb.LogRecord{
										{
											ObservedTimeUnixNano: uint64(time.Now().UnixNano()),
											TimeUnixNano:         uint64(1257894000000000000),
											SeverityText:         logspb.SeverityNumber_SEVERITY_NUMBER_ERROR.String(),
											SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
											Body: &commonpb.AnyValue{
												Value: &commonpb.AnyValue_StringValue{
													StringValue: "log error message",
												},
											},
											Attributes: []*commonpb.KeyValue{
												{
													Key:   "direction",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "egress"}},
												},
												{
													Key:   "instance_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
												},
												{
													Key:   "origin",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-origin.some-vm"}},
												},
												{
													Key:   "source_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				dict := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
					return x.Key < y.Key
				})
				Expect(lsr.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano).NotTo(BeZero())
				expectedReq.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano = lsr.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano
				Expect(cmp.Diff(lsr, expectedReq, protocmp.Transform(), dict)).To(BeEmpty())
			})
		})

		Context("when given event", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"origin": "fake-origin.some-vm",
					},
					Message: &loggregator_v2.Envelope_Event{
						Event: &loggregator_v2.Event{
							Title: "event title",
							Body:  "event body",
						},
					},
				}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("emits an event log", func() {
				var lsr *collogspb.ExportLogsServiceRequest
				Expect(spyLSC.requests).To(Receive(&lsr))

				expectedReq := &collogspb.ExportLogsServiceRequest{
					ResourceLogs: []*logspb.ResourceLogs{
						{
							ScopeLogs: []*logspb.ScopeLogs{
								{
									LogRecords: []*logspb.LogRecord{
										{
											ObservedTimeUnixNano: uint64(time.Now().UnixNano()),
											TimeUnixNano:         uint64(1257894000000000000),
											Body: &commonpb.AnyValue{
												Value: &commonpb.AnyValue_KvlistValue{
													KvlistValue: &commonpb.KeyValueList{
														Values: []*commonpb.KeyValue{
															{
																Key:   "title",
																Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "event title"}},
															},
															{
																Key:   "body",
																Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "event body"}},
															},
														},
													},
												},
											},
											Attributes: []*commonpb.KeyValue{
												{
													Key:   "instance_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-instance-id"}},
												},
												{
													Key:   "origin",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-origin.some-vm"}},
												},
												{
													Key:   "source_id",
													Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "fake-source-id"}},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				dict := protocmp.SortRepeated(func(x *commonpb.KeyValue, y *commonpb.KeyValue) bool {
					return x.Key < y.Key
				})
				Expect(lsr.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano).NotTo(BeZero())
				expectedReq.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano = lsr.ResourceLogs[0].ScopeLogs[0].LogRecords[0].ObservedTimeUnixNano
				Expect(cmp.Diff(lsr, expectedReq, protocmp.Transform(), dict)).To(BeEmpty())
			})
		})

		Context("when no writes have occurred for a while", func() {
			BeforeEach(func() {
				spyMSC = &spyMetricsServiceClient{
					requests:    make(chan *colmetricspb.ExportMetricsServiceRequest, 1),
					response:    &colmetricspb.ExportMetricsServiceResponse{},
					responseErr: nil,
				}
				ctx, cancel := context.WithCancel(context.Background())
				w := GRPCWriter{
					msc:    spyMSC,
					ctx:    ctx,
					cancel: cancel,
					l:      log.New(GinkgoWriter, "", 0),
				}

				b := NewSignalBatcher(
					1000,
					10*time.Millisecond,
					w,
				)
				c = Client{b: b}
				envelope = &loggregator_v2.Envelope{
					Timestamp:  1257894000000000000,
					SourceId:   "fake-source-id",
					InstanceId: "fake-instance-id",
					Tags: map[string]string{
						"direction": "egress",
						"origin":    "fake-origin.some-vm",
					},
					Message: &loggregator_v2.Envelope_Counter{
						Counter: &loggregator_v2.Counter{
							Name:  "dropped",
							Delta: 5,
							Total: 10,
						},
					},
				}
			})
			It("flushes pending writes", func() {
				Expect(returnedErr).NotTo(HaveOccurred())

				var msr *colmetricspb.ExportMetricsServiceRequest
				Eventually(spyMSC.requests).Should(Receive(&msr))
			})
		})
	})

	Describe("Close", func() {
		It("cancels the gRPC context", func() {
			envelope := &loggregator_v2.Envelope{
				Message: &loggregator_v2.Envelope_Gauge{
					Gauge: &loggregator_v2.Gauge{
						Metrics: map[string]*loggregator_v2.GaugeValue{
							"cpu": {
								Unit:  "percentage",
								Value: 0.3257,
							},
						},
					},
				},
			}
			Expect(c.Write(envelope)).ToNot(HaveOccurred())
			Expect(c.Close()).ToNot(HaveOccurred())
			Eventually(spyMSC.ctx.Done()).Should(BeClosed())
		})
	})
})

type spyMetricsServiceClient struct {
	requests    chan *colmetricspb.ExportMetricsServiceRequest
	response    *colmetricspb.ExportMetricsServiceResponse
	responseErr error
	ctx         context.Context
}

func (c *spyMetricsServiceClient) Export(ctx context.Context, in *colmetricspb.ExportMetricsServiceRequest, opts ...grpc.CallOption) (*colmetricspb.ExportMetricsServiceResponse, error) {
	c.requests <- in
	c.ctx = ctx
	return c.response, c.responseErr
}

type spyLogsServiceClient struct {
	requests    chan *collogspb.ExportLogsServiceRequest
	response    *collogspb.ExportLogsServiceResponse
	responseErr error
	ctx         context.Context
}

func (c *spyLogsServiceClient) Export(ctx context.Context, in *collogspb.ExportLogsServiceRequest, opts ...grpc.CallOption) (*collogspb.ExportLogsServiceResponse, error) {
	c.requests <- in
	c.ctx = ctx
	return c.response, c.responseErr
}

type spyTraceServiceClient struct {
	requests    chan *coltracepb.ExportTraceServiceRequest
	response    *coltracepb.ExportTraceServiceResponse
	responseErr error
	ctx         context.Context
}

func (c *spyTraceServiceClient) Export(ctx context.Context, in *coltracepb.ExportTraceServiceRequest, opts ...grpc.CallOption) (*coltracepb.ExportTraceServiceResponse, error) {
	c.requests <- in
	c.ctx = ctx
	return c.response, c.responseErr
}

func span(tsr *coltracepb.ExportTraceServiceRequest) *tracepb.Span {
	GinkgoHelper()
	rs := tsr.ResourceSpans
	Expect(rs).To(HaveLen(1))
	ss := rs[0].GetScopeSpans()
	Expect(ss).To(HaveLen(1))
	spans := ss[0].GetSpans()
	Expect(spans).To(HaveLen(1))
	return spans[0]
}
