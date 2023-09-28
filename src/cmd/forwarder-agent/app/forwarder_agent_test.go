package app_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"

	"code.cloudfoundry.org/go-loggregator/v9"
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/forwarder-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("App", func() {
	const agentCN = "metron"

	var (
		grpcPort    int
		pprofPort   int
		metricsPort int

		ingressCfgPath string
		ingressClient  *loggregator.IngressClient
		ingressServer1 *spyLoggregatorV2Ingress
		ingressServer2 *spyLoggregatorV2Ingress
		ingressServer3 *spyLoggregatorV2Ingress

		agentCfg     app.Config
		agentMetrics *metricsHelpers.SpyMetricsRegistry
		agentLogr    *log.Logger
		agentCerts   *testhelper.TestCerts
		agent        *app.ForwarderAgent
	)

	BeforeEach(func() {
		grpcPort = 30000 + GinkgoParallelProcess()
		pprofPort = 31000 + GinkgoParallelProcess()
		metricsPort = 32000 + GinkgoParallelProcess()

		agentCerts = testhelper.GenerateCerts("forwarder-ca")

		ingressCfgPath = GinkgoT().TempDir()
		ingressClient = newIngressClient(grpcPort, agentCerts, 1)

		ingressServer1 = startSpyLoggregatorV2Ingress(agentCerts, agentCN, ingressCfgPath)
		ingressServer2 = startSpyLoggregatorV2Ingress(agentCerts, agentCN, ingressCfgPath)
		ingressServer3 = startSpyLoggregatorV2Ingress(agentCerts, agentCN, ingressCfgPath)
		ingressServer3.blocking = true

		agentCfg = app.Config{
			GRPC: app.GRPC{
				Port:     uint16(grpcPort),
				CAFile:   agentCerts.CA(),
				CertFile: agentCerts.Cert(agentCN),
				KeyFile:  agentCerts.Key(agentCN),
			},
			DownstreamIngressPortCfg: fmt.Sprintf("%s/*/ingress_port.yml", ingressCfgPath),
			MetricsServer: config.MetricsServer{
				Port:      uint16(metricsPort),
				CAFile:    agentCerts.CA(),
				CertFile:  agentCerts.Cert(agentCN),
				KeyFile:   agentCerts.Key(agentCN),
				PprofPort: uint16(pprofPort),
			},
			Tags: map[string]string{
				"some-tag": "some-value",
			},
		}
		agentMetrics = metricsHelpers.NewMetricsRegistry()
		agentLogr = log.New(GinkgoWriter, "", log.LstdFlags)
	})

	JustBeforeEach(func() {
		agent = app.NewForwarderAgent(agentCfg, agentMetrics, agentLogr)
		go agent.Run()
		Eventually(func() bool {
			err := ingressClient.EmitEvent(context.TODO(), "test-title", "test-body")
			return err == nil
		}, 10).Should(BeTrue())
		Eventually(ingressServer1.envelopes, 5).Should(Receive())
		Eventually(ingressServer2.envelopes, 5).Should(Receive())
		Eventually(ingressServer3.envelopes, 5).Should(Receive())
	})

	AfterEach(func() {
		ingressServer3.close()
		ingressServer2.close()
		ingressServer1.close()
		agent.Stop()
	})

	It("emits a dropped metric for envelope ingress", func() {
		et := map[string]string{
			"direction": "ingress",
		}

		Eventually(func() bool {
			return agentMetrics.HasMetric("dropped", et)
		}).Should(BeTrue())

		m := agentMetrics.GetMetric("dropped", et)

		Expect(m).ToNot(BeNil())
		Expect(m.Opts.ConstLabels).To(HaveKeyWithValue("direction", "ingress"))
	})

	It("emits an expired metric for each egress destination", func() {
		dests := []string{
			ingressServer1.addr,
			ingressServer2.addr,
			ingressServer3.addr,
		}
		for i, d := range dests {
			ingressServerName := fmt.Sprintf("ingressServer%d", i+1)

			et := map[string]string{
				"protocol":    "loggregator",
				"destination": d,
			}

			Eventually(agentMetrics.HasMetric).WithArguments("egress_expired_total", et).Should(BeTrue(), fmt.Sprintf("no metric found for %s", ingressServerName))

			m := agentMetrics.GetMetric("egress_expired_total", et)
			for k, v := range et {
				Expect(m.Opts.ConstLabels).To(HaveKeyWithValue(k, v), fmt.Sprintf("missing label for metric for %s", ingressServerName))
			}
		}
	})

	It("does not emit debug metrics", func() {
		Consistently(agentMetrics.GetDebugMetricsEnabled(), 5).Should(BeFalse())
	})

	It("does not expose a pprof endpoint", func() {
		Consistently(func() error {
			_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", agentCfg.MetricsServer.PprofPort))
			return err
		}, 5).ShouldNot(BeNil())
	})

	Context("when debug configuration is enabled", func() {
		BeforeEach(func() {
			agentCfg.MetricsServer.DebugMetrics = true
		})

		It("does not emit debug metrics", func() {
			Eventually(agentMetrics.GetDebugMetricsEnabled(), 5).Should(BeTrue())
		})

		It("does not expose a pprof endpoint", func() {
			Eventually(func() error {
				resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", agentCfg.MetricsServer.PprofPort))
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				return nil
			}, 5).Should(BeNil())
		})
	})

	It("forwards all envelopes downstream", func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(10 * time.Millisecond)
			for {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-ticker.C:
					ingressClient.Emit(sampleEnvelope)
				}
			}
		}()

		Eventually(ingressServer1.envelopes, 5).Should(Receive(protoEqual(sampleEnvelope)))
		Eventually(ingressServer2.envelopes, 5).Should(Receive(protoEqual(sampleEnvelope)))
	})

	It("can send a batch of 100, max-size (for Diego) messages downstream", func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		maxBatchIngressClient := newIngressClient(grpcPort, agentCerts, 100)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(time.Second)
			for {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-ticker.C:
					for i := 0; i < 100; i++ {
						maxBatchIngressClient.Emit(MakeSampleBigEnvelope())
					}
				}
			}
		}()

		Eventually(ingressServer1.envelopes, 5).Should(Receive())
		Eventually(ingressServer2.envelopes, 5).Should(Receive())
	})

	It("aggregates counter events before forwarding them downstream", func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(10 * time.Millisecond)
			for {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-ticker.C:
					ingressClient.Emit(sampleCounter)
				}
			}
		}()

		var e1, e2 *loggregator_v2.Envelope
		Eventually(ingressServer1.envelopes, 5).Should(Receive(&e1))
		Eventually(ingressServer2.envelopes, 5).Should(Receive(&e2))

		Expect(e1.GetCounter().GetTotal()).To(Equal(uint64(20)))
		Expect(e2.GetCounter().GetTotal()).To(Equal(uint64(20)))
	})

	It("tags before forwarding downstream", func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(10 * time.Millisecond)
			for {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-ticker.C:
					ingressClient.Emit(sampleEnvelope)
				}
			}
		}()

		var e1, e2 *loggregator_v2.Envelope
		Eventually(ingressServer1.envelopes, 5).Should(Receive(&e1))
		Eventually(ingressServer2.envelopes, 5).Should(Receive(&e2))

		Expect(e1.GetTags()).To(HaveLen(1))
		Expect(e1.GetTags()["some-tag"]).To(Equal("some-value"))
		Expect(e2.GetTags()).To(HaveLen(1))
		Expect(e2.GetTags()["some-tag"]).To(Equal("some-value"))
	})

	It("continues writing to other consumers if one is slow", func() {
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(10 * time.Millisecond)
			for {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-ticker.C:
					ingressClient.Emit(sampleEnvelope)
				}
			}
		}()

		Eventually(ingressServer1.envelopes, 5).Should(Receive())
		Eventually(ingressServer2.envelopes, 5).Should(Receive())

		prevSize := 100 // set to big number so it doesn't fail immediately
		Consistently(func() bool {
			notEqual := len(ingressServer1.envelopes) != prevSize
			prevSize = len(ingressServer1.envelopes)
			return notEqual
		}, 5, 1).Should(BeTrue())
		prevSize = 0
		Consistently(func() bool {
			notEqual := len(ingressServer2.envelopes) != prevSize
			prevSize = len(ingressServer2.envelopes)
			return notEqual
		}, 5, 1).Should(BeTrue())
	})

	Context("when an OTel Collector is co-located but disabled", func() {
		var buf *gbytes.Buffer

		BeforeEach(func() {
			buf = gbytes.NewBuffer()
			GinkgoWriter.TeeTo(buf)

			dir, err := os.MkdirTemp(ingressCfgPath, "")
			Expect(err).ToNot(HaveOccurred())
			tmpfn := filepath.Join(dir, "ingress_port.yml")

			err = os.WriteFile(tmpfn, []byte{}, 0600)
			Expect(err).ToNot(HaveOccurred())

		})
		It("logs a message", func() {
			Eventually(buf).Should(gbytes.Say("No ingress port defined in .*/ingress_port.yml. Ignoring this destination."))
		})
	})

	Context("when an OTel Collector is registered to forward to", func() {
		var otelServer *spyOtelColMetricServer

		BeforeEach(func() {
			otelServer = startSpyOtelColMetricServer(ingressCfgPath, agentCerts, "otel-collector")
		})

		AfterEach(func() {
			otelServer.close()
		})

		DescribeTable("some envelopes are not forwarded",
			func(e *loggregator_v2.Envelope) {
				ingressClient.Emit(e)
				Consistently(otelServer.requests, 3).ShouldNot(Receive())
			},
			Entry("drops logs", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Log{}}),
			Entry("drops events", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Event{}}),
			Entry("drops timers", &loggregator_v2.Envelope{Message: &loggregator_v2.Envelope_Timer{}}),
		)

		It("forwards counters", func() {
			name := "test-counter-name"
			ingressClient.EmitCounter(name)

			var req *colmetricspb.ExportMetricsServiceRequest
			Eventually(otelServer.requests).Should(Receive(&req))

			metric := req.ResourceMetrics[0].ScopeMetrics[0].Metrics[0]
			Expect(metric.GetName()).To(Equal(name))
		})

		It("forwards gauges", func() {
			name := "test-gauge-name"
			ingressClient.EmitGauge(loggregator.WithGaugeValue(name, 20.2, "test-unit"))

			var req *colmetricspb.ExportMetricsServiceRequest
			Eventually(otelServer.requests).Should(Receive(&req))

			metric := req.ResourceMetrics[0].ScopeMetrics[0].Metrics[0]
			Expect(metric.GetName()).To(Equal(name))
		})

		It("emits an expired metric", func() {
			et := map[string]string{
				"protocol":    "otelcol",
				"destination": otelServer.addr,
			}

			Eventually(agentMetrics.HasMetric).WithArguments("egress_expired_total", et).Should(BeTrue())

			m := agentMetrics.GetMetric("egress_expired_total", et)
			for k, v := range et {
				Expect(m.Opts.ConstLabels).To(HaveKeyWithValue(k, v))
			}
		})
	})
})
