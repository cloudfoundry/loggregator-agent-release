package app_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/cloudfoundry/dropsonde/emitter"
	"github.com/cloudfoundry/sonde-go/events"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/udp-forwarder/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/config"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

var _ = Describe("UDPForwarder", func() {
	const forwarderCN = "metron"

	var (
		forwarderPort int
		pprofPort     int
		metricsPort   int

		spyReceiver    *spyReceiver
		spyEmitter     *emitter.EventEmitter
		emitCancelFunc context.CancelFunc

		forwarderCerts   *testhelper.TestCerts
		forwarderCfg     app.Config
		forwarderMetrics *metricsHelpers.SpyMetricsRegistry
		forwarderLogr    *log.Logger
		forwarder        *app.UDPForwarder
	)

	BeforeEach(func() {
		forwarderPort = 30000 + GinkgoParallelProcess()
		pprofPort = 31000 + GinkgoParallelProcess()
		metricsPort = 32000 + GinkgoParallelProcess()

		forwarderCerts = testhelper.GenerateCerts("forwarder-ca")

		spyReceiver = startSpyReceiver(forwarderCerts, forwarderCN)

		forwarderCfg = app.Config{
			UDPPort: forwarderPort,
			LoggregatorAgentGRPC: app.GRPC{
				Addr:     spyReceiver.addr,
				CAFile:   forwarderCerts.CA(),
				CertFile: forwarderCerts.Cert(forwarderCN),
				KeyFile:  forwarderCerts.Key(forwarderCN),
			},
			Deployment: "test-deployment",
			Job:        "test-job",
			Index:      "4",
			IP:         "127.0.0.1",
			MetricsServer: config.MetricsServer{
				Port:      uint16(metricsPort),
				CAFile:    forwarderCerts.CA(),
				CertFile:  forwarderCerts.Cert(forwarderCN),
				KeyFile:   forwarderCerts.Key(forwarderCN),
				PprofPort: uint16(pprofPort),
			},
		}
		forwarderLogr = log.New(GinkgoWriter, "", log.LstdFlags)
		forwarderMetrics = metricsHelpers.NewMetricsRegistry()
	})

	JustBeforeEach(func() {
		forwarder = app.NewUDPForwarder(forwarderCfg, forwarderLogr, forwarderMetrics)
		go forwarder.Run()

		e, err := emitter.NewUdpEmitter(fmt.Sprintf("127.0.0.1:%d", forwarderPort))
		Expect(err).ToNot(HaveOccurred())
		spyEmitter = emitter.NewEventEmitter(e, "")
		var ctx context.Context
		ctx, emitCancelFunc = context.WithCancel(context.Background())
		go func() {
			v1e := &events.Envelope{
				Origin:    proto.String("doppler"),
				EventType: events.Envelope_LogMessage.Enum(),
				Timestamp: proto.Int64(time.Now().UnixNano()),
				LogMessage: &events.LogMessage{
					Message:     []byte("some-log-message"),
					MessageType: events.LogMessage_OUT.Enum(),
					Timestamp:   proto.Int64(time.Now().UnixNano()),
				},
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			err := spyEmitter.EmitEnvelope(v1e)
			Expect(err).ToNot(HaveOccurred())
			for {
				select {
				case <-ticker.C:
					err := spyEmitter.EmitEnvelope(v1e)
					Expect(err).ToNot(HaveOccurred())
				case <-ctx.Done():
					return
				}
			}
		}()
		Eventually(spyReceiver.envelopes, 5).Should(Receive())
	})

	AfterEach(func() {
		emitCancelFunc()
		forwarder.Stop()
		spyReceiver.close()
	})

	It("forwards envelopes from Loggregator V1 to V2", func() {
		var v2e *loggregator_v2.Envelope
		Eventually(spyReceiver.envelopes, 5).Should(Receive(&v2e))
		Expect(string(v2e.GetLog().GetPayload())).To(Equal("some-log-message"))

		Expect(v2e.GetTags()["deployment"]).To(Equal("test-deployment"))
		Expect(v2e.GetTags()["job"]).To(Equal("test-job"))
		Expect(v2e.GetTags()["index"]).To(Equal("4"))
		Expect(v2e.GetTags()["ip"]).To(Equal("127.0.0.1"))
	})

	It("does not have debug metrics by default", func() {
		Consistently(forwarderMetrics.GetDebugMetricsEnabled()).Should(BeFalse())
		Consistently(func() error {
			_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", pprofPort))
			return err
		}).ShouldNot(BeNil())
	})

	Context("when debug configuration is enabled", func() {
		BeforeEach(func() {
			forwarderCfg.MetricsServer.DebugMetrics = true
		})

		It("serves pprof", func() {
			u := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", pprofPort)
			Eventually(func() bool {
				resp, err := http.Get(u) //nolint:gosec
				if err != nil {
					return false
				}
				defer resp.Body.Close()
				return resp.StatusCode == 200
			}, 3).Should(BeTrue())
		})

		It("registers debug metrics", func() {
			Eventually(forwarderMetrics.GetDebugMetricsEnabled).Should(BeTrue())
		})
	})
})

type spyReceiver struct {
	loggregator_v2.UnimplementedIngressServer

	addr      string
	close     func()
	envelopes chan *loggregator_v2.Envelope
}

func (s *spyReceiver) Sender(loggregator_v2.Ingress_SenderServer) error {
	panic("not implemented")
}

func (s *spyReceiver) Send(context.Context, *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	panic("not implemented")
}

func (s *spyReceiver) BatchSender(srv loggregator_v2.Ingress_BatchSenderServer) error {
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

func startSpyReceiver(tc *testhelper.TestCerts, commonName string) *spyReceiver {
	sr := &spyReceiver{
		envelopes: make(chan *loggregator_v2.Envelope, 100),
	}

	creds, err := plumbing.NewServerCredentials(
		tc.Cert(commonName),
		tc.Key(commonName),
		tc.CA(),
	)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	lis, err := net.Listen("tcp", "127.0.0.1:")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	srv := grpc.NewServer(grpc.Creds(creds))
	loggregator_v2.RegisterIngressServer(srv, sr)

	sr.close = func() {
		srv.Stop()
		_ = lis.Close()
	}
	sr.addr = lis.Addr().String()

	go func() {
		_ = srv.Serve(lis)
	}()

	return sr
}
