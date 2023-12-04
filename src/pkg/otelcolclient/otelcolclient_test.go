package otelcolclient

import (
	"context"
	"errors"
	"log"
	"time"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/testing/protocmp"
)

var _ = Describe("Client", func() {
	var (
		c      Client
		spyMSC *spyMetricsServiceClient
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
		ctx, cancel := context.WithCancel(context.Background())
		w := GRPCWriter{
			msc:    spyMSC,
			ctx:    ctx,
			cancel: cancel,
			l:      log.New(GinkgoWriter, "", 0),
		}
		b := NewMetricBatcher(
			1,
			100*time.Millisecond,
			w,
		)
		c = Client{b: b}
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
				b := NewMetricBatcher(
					2,
					100*time.Millisecond,
					w,
				)
				c = Client{b: b}
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
							Delta: 5,
							Total: 10,
						},
					},
				}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("succeeds", func() {
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
				envelope = &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Timer{}}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("does nothing", func() {
				Expect(spyMSC.requests).NotTo(Receive())
				Consistently(buf.Contents()).Should(HaveLen(0))
			})
		})

		Context("when given a log", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Log{}}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("does nothing", func() {
				Expect(spyMSC.requests).NotTo(Receive())
				Consistently(buf.Contents()).Should(HaveLen(0))
			})
		})

		Context("when given an event", func() {
			BeforeEach(func() {
				envelope = &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Event{}}
			})

			It("returns nil", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
			})

			It("does nothing", func() {
				Expect(spyMSC.requests).NotTo(Receive())
				Consistently(buf.Contents()).Should(HaveLen(0))
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

				b := NewMetricBatcher(
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
