package app_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"

	"code.cloudfoundry.org/go-loggregator/v8"
	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/forwarder-agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"github.com/gogo/protobuf/proto"
	"github.com/onsi/gomega/gexec"
	"google.golang.org/grpc"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const forwardConfigTemplate = `---
ingress: %s
`

var (
	fConfigDir string
)

var _ = Describe("Main", func() {
	var (
		grpcPort   = 20000
		testLogger = log.New(GinkgoWriter, "", log.LstdFlags)
		testCerts  = testhelper.GenerateCerts("loggregatorCA")

		forwarderAgent *app.ForwarderAgent
		mc             *metricsHelpers.SpyMetricsRegistry
		cfg            app.Config
		ingressClient  *loggregator.IngressClient

		emitEnvelopes = func(ctx context.Context, d time.Duration, wg *sync.WaitGroup) {
			go func() {
				defer wg.Done()

				ticker := time.NewTicker(d)
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						ingressClient.Emit(sampleEnvelope)
					}
				}
			}()
		}

		emitCounters = func(ctx context.Context, d time.Duration, wg *sync.WaitGroup) {
			go func() {
				defer wg.Done()

				ticker := time.NewTicker(d)
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						ingressClient.Emit(sampleCounter)
					}
				}
			}()
		}
	)

	BeforeEach(func() {
		fConfigDir = forwarderPortConfigDir()

		mc = metricsHelpers.NewMetricsRegistry()
		cfg = app.Config{
			GRPC: app.GRPC{
				Port:     uint16(grpcPort),
				CAFile:   testCerts.CA(),
				CertFile: testCerts.Cert("metron"),
				KeyFile:  testCerts.Key("metron"),
			},
			DownstreamIngressPortCfg: fmt.Sprintf("%s/*/ingress_port.yml", fConfigDir),
			MetricsServer: config.MetricsServer{
				Port:     7392,
				CAFile:   testCerts.CA(),
				CertFile: testCerts.Cert("metron"),
				KeyFile:  testCerts.Key("metron"),
			},
			Tags: map[string]string{
				"some-tag": "some-value",
			},
		}
		ingressClient = newIngressClient(grpcPort, testCerts, 1)
	})

	AfterEach(func() {
		os.RemoveAll(fConfigDir)

		gexec.CleanupBuildArtifacts()
		grpcPort++
	})

	It("has a dropped metric with direction", func() {
		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		et := map[string]string{
			"direction": "ingress",
		}

		Eventually(func() bool {
			return mc.HasMetric("dropped", et)
		}).Should(BeTrue())

		m := mc.GetMetric("dropped", et)

		Expect(m).ToNot(BeNil())
		Expect(m.Opts.ConstLabels).To(HaveKeyWithValue("direction", "ingress"))
	})

	It("debug metrics arn't enabled by default", func() {
		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		cfg.MetricsServer.PprofPort = 1236
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		Consistently(mc.GetDebugMetricsEnabled()).Should(BeFalse())
		_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", cfg.MetricsServer.PprofPort))
		Expect(err).ToNot(BeNil())
	})
	It("debug metrics can be enabled", func() {
		cfg.MetricsServer.DebugMetrics = true
		cfg.MetricsServer.PprofPort = 1237
		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		Eventually(mc.GetDebugMetricsEnabled).Should(BeTrue())
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", cfg.MetricsServer.PprofPort))
		Expect(err).To(BeNil())
		Expect(resp.StatusCode).To(Equal(200))
	})

	It("forwards all envelopes downstream", func() {
		downstream1 := startSpyLoggregatorV2Ingress(testCerts)
		downstream2 := startSpyLoggregatorV2Ingress(testCerts)

		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		emitEnvelopes(ctx, 10*time.Millisecond, &wg)

		var e1, e2 *loggregator_v2.Envelope
		Eventually(downstream1.envelopes, 5).Should(Receive(&e1))
		Eventually(downstream2.envelopes, 5).Should(Receive(&e2))

		Expect(proto.Equal(e1, sampleEnvelope)).To(BeTrue())
		Expect(proto.Equal(e2, sampleEnvelope)).To(BeTrue())
	})

	It("can send a 100 sized batch of max diego size messages downstream", func() {
		downstream1 := startSpyLoggregatorV2Ingress(testCerts)

		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		maxBatchIngressClient := newIngressClient(grpcPort, testCerts, 100)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(time.Second)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					for i := 0; i < 100; i++ {
						maxBatchIngressClient.Emit(MakeSampleBigEnvelope())
					}
				}
			}
		}()

		var e1 *loggregator_v2.Envelope
		Eventually(downstream1.envelopes, 10).Should(Receive(&e1))
	})

	It("aggregates counter events before forwarding downstream", func() {
		downstream1 := startSpyLoggregatorV2Ingress(testCerts)

		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		emitCounters(ctx, 10*time.Millisecond, &wg)

		var e1 *loggregator_v2.Envelope
		Eventually(downstream1.envelopes, 5).Should(Receive(&e1))

		Expect(e1.GetCounter().GetTotal()).To(Equal(uint64(20)))
	})

	It("tags before forwarding downstream", func() {
		downstream1 := startSpyLoggregatorV2Ingress(testCerts)

		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		emitEnvelopes(ctx, 10*time.Millisecond, &wg)

		var e1 *loggregator_v2.Envelope
		Eventually(downstream1.envelopes, 5).Should(Receive(&e1))

		Expect(e1.GetTags()).To(HaveLen(1))
		Expect(e1.GetTags()["some-tag"]).To(Equal("some-value"))
	})

	It("continues writing to other consumers if one is slow", func() {
		downstreamNormal := startSpyLoggregatorV2Ingress(testCerts)
		startSpyLoggregatorV2BlockingIngress(testCerts)

		forwarderAgent = app.NewForwarderAgent(cfg, mc, testLogger)
		go forwarderAgent.Run()
		defer forwarderAgent.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		defer wg.Wait()
		defer cancel()

		wg.Add(1)
		emitEnvelopes(ctx, 1*time.Millisecond, &wg)

		Eventually(downstreamNormal.envelopes, 5).Should(Receive())

		var prevSize int
		Consistently(func() bool {
			notEqual := len(downstreamNormal.envelopes) != prevSize
			prevSize = len(downstreamNormal.envelopes)
			return notEqual
		}, 5, 1).Should(BeTrue())
	})
})

var sampleEnvelope = &loggregator_v2.Envelope{
	Timestamp: time.Now().UnixNano(),
	SourceId:  "some-id",
	Message: &loggregator_v2.Envelope_Log{
		Log: &loggregator_v2.Log{
			Payload: []byte("hello"),
		},
	},
	Tags: map[string]string{
		"some-tag": "some-value",
	},
}

func MakeSampleBigEnvelope() *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Timestamp: time.Now().UnixNano(),
		SourceId:  "some-id",
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte(strings.Repeat("A", 61440)),
			},
		},
		Tags: map[string]string{
			"some-tag": "some-value",
		},
	}
}

var sampleCounter = &loggregator_v2.Envelope{
	Timestamp: time.Now().UnixNano(),
	SourceId:  "some-id",
	Message: &loggregator_v2.Envelope_Counter{
		Counter: &loggregator_v2.Counter{
			Delta: 20,
			Total: 0,
		},
	},
}

func newIngressClient(port int, testCerts *testhelper.TestCerts, batchSize uint) *loggregator.IngressClient {
	tlsConfig, err := loggregator.NewIngressTLSConfig(
		testCerts.CA(),
		testCerts.Cert("metron"),
		testCerts.Key("metron"),
	)
	Expect(err).ToNot(HaveOccurred())
	ingressClient, err := loggregator.NewIngressClient(
		tlsConfig,
		loggregator.WithAddr(fmt.Sprintf("127.0.0.1:%d", port)),
		loggregator.WithLogger(log.New(GinkgoWriter, "[TEST INGRESS CLIENT] ", 0)),
		loggregator.WithBatchMaxSize(batchSize),
	)
	Expect(err).ToNot(HaveOccurred())
	return ingressClient
}

func startSpyLoggregatorV2Ingress(testCerts *testhelper.TestCerts) *spyLoggregatorV2Ingress {
	s := &spyLoggregatorV2Ingress{
		envelopes: make(chan *loggregator_v2.Envelope, 10000),
	}

	serverCreds, err := plumbing.NewServerCredentials(
		testCerts.Cert("metron"),
		testCerts.Key("metron"),
		testCerts.CA(),
	)

	lis, err := net.Listen("tcp", ":0")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	grpcServer := grpc.NewServer(grpc.Creds(serverCreds))
	loggregator_v2.RegisterIngressServer(grpcServer, s)

	s.close = func() {
		lis.Close()
	}
	s.addr = lis.Addr().String()
	port := strings.Split(s.addr, ":")

	createForwarderPortConfigFile(port[len(port)-1])
	go grpcServer.Serve(lis)

	return s
}

type spyLoggregatorV2Ingress struct {
	addr      string
	close     func()
	envelopes chan *loggregator_v2.Envelope
}

func (s *spyLoggregatorV2Ingress) Sender(loggregator_v2.Ingress_SenderServer) error {
	panic("not implemented")
}

func (s *spyLoggregatorV2Ingress) Send(context.Context, *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	panic("not implemented")
}

func (s *spyLoggregatorV2Ingress) BatchSender(srv loggregator_v2.Ingress_BatchSenderServer) error {
	for {
		batch, err := srv.Recv()
		if err != nil {
			return err
		}

		for _, e := range batch.Batch {
			s.envelopes <- e
		}
	}
}

func startSpyLoggregatorV2BlockingIngress(testCerts *testhelper.TestCerts) *spyLoggregatorV2BlockingIngress {
	s := &spyLoggregatorV2BlockingIngress{}

	serverCreds, err := plumbing.NewServerCredentials(
		testCerts.Cert("metron"),
		testCerts.Key("metron"),
		testCerts.CA(),
	)

	lis, err := net.Listen("tcp", ":0")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	grpcServer := grpc.NewServer(grpc.Creds(serverCreds))
	loggregator_v2.RegisterIngressServer(grpcServer, s)

	s.close = func() {
		lis.Close()
	}
	s.addr = lis.Addr().String()

	port := strings.Split(s.addr, ":")
	createForwarderPortConfigFile(port[len(port)-1])
	go grpcServer.Serve(lis)

	return s
}

type spyLoggregatorV2BlockingIngress struct {
	addr  string
	close func()
}

func (s *spyLoggregatorV2BlockingIngress) Sender(loggregator_v2.Ingress_SenderServer) error {
	panic("not implemented")
}

func (s *spyLoggregatorV2BlockingIngress) Send(context.Context, *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	panic("not implemented")
}

func (s *spyLoggregatorV2BlockingIngress) BatchSender(srv loggregator_v2.Ingress_BatchSenderServer) error {
	c := make(chan struct{})
	for {
		_, err := srv.Recv()
		if err != nil {
			return err
		}

		<-c
	}
}

func forwarderPortConfigDir() string {
	dir, err := os.MkdirTemp("", "")
	if err != nil {
		log.Fatal(err)
	}

	return dir
}

func createForwarderPortConfigFile(port string) {
	fDir, err := os.MkdirTemp(fConfigDir, "")
	if err != nil {
		log.Fatal(err)
	}

	tmpfn := filepath.Join(fDir, "ingress_port.yml")
	tmpfn, err = filepath.Abs(tmpfn)
	Expect(err).ToNot(HaveOccurred())

	contents := fmt.Sprintf(forwardConfigTemplate, port)
	if err := os.WriteFile(tmpfn, []byte(contents), 0666); err != nil {
		log.Fatal(err)
	}
}
