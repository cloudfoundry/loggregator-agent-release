package v2_test

import (
	"io"
	"net"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/clientpool/v2"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("PusherFetcher", func() {
	var (
		mc *metricsHelpers.SpyMetricsRegistry
	)

	BeforeEach(func() {
		mc = metricsHelpers.NewMetricsRegistry()
	})

	It("opens a stream with the ingress client", func() {
		server := newSpyIngestorServer()
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v2.NewSenderFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		closer, sender, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		err = sender.Send(&loggregator_v2.EnvelopeBatch{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(server.batch).Should(Receive())
		Expect(closer.Close()).To(Succeed())
	})

	It("increments a counter when a connection is established", func() {
		server := newSpyIngestorServer()
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v2.NewSenderFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		_, _, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		tags := map[string]string{"metric_version": "2.0"}
		Expect(mc.GetMetric("doppler_connections", tags).Value()).To(Equal(float64(1)))
		Expect(mc.GetMetric("doppler_v2_streams", tags).Value()).To(Equal(float64(1)))
	})

	It("decrements a counter when a connection is closed", func() {
		server := newSpyIngestorServer()
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v2.NewSenderFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		closer, _, err := fetcher.Fetch(server.addr)
		Expect(err).ToNot(HaveOccurred())

		closer.Close()
		tags := map[string]string{"metric_version": "2.0"}
		Expect(mc.GetMetric("doppler_connections", tags).Value()).To(Equal(float64(0)))
		Expect(mc.GetMetric("doppler_v2_streams", tags).Value()).To(Equal(float64(0)))
	})

	It("returns an error when the server is unavailable", func() {
		fetcher := v2.NewSenderFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		_, _, err := fetcher.Fetch("127.0.0.1:1122")
		Expect(err).To(HaveOccurred())
	})
})

type SpyIngestorServer struct {
	addr            string
	server          *grpc.Server
	stop            chan struct{}
	deprecatedBatch chan *loggregator_v2.EnvelopeBatch
	batch           chan *loggregator_v2.EnvelopeBatch
}

func newSpyIngestorServer() *SpyIngestorServer {
	return &SpyIngestorServer{
		stop:            make(chan struct{}),
		batch:           make(chan *loggregator_v2.EnvelopeBatch),
		deprecatedBatch: make(chan *loggregator_v2.EnvelopeBatch),
	}
}

func (s *SpyIngestorServer) Start() error {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	s.addr = lis.Addr().String()
	loggregator_v2.RegisterIngressServer(s.server, &spyV2IngressServer{spyIngestorServer: s})

	go s.server.Serve(lis)

	return nil
}

func (s *SpyIngestorServer) Stop() {
	close(s.stop)
	s.server.Stop()
}

type spyV2IngressServer struct {
	loggregator_v2.UnimplementedIngressServer

	spyIngestorServer *SpyIngestorServer
}

func (s *spyV2IngressServer) Send(context.Context, *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	return nil, nil
}

func (s *spyV2IngressServer) Sender(srv loggregator_v2.Ingress_SenderServer) error {
	return nil
}

func (s *spyV2IngressServer) BatchSender(srv loggregator_v2.Ingress_BatchSenderServer) error {
	for {
		select {
		case <-srv.Context().Done():
			return nil
		case <-s.spyIngestorServer.stop:
			return io.EOF
		default:
			b, err := srv.Recv()
			if err != nil {
				return nil
			}

			s.spyIngestorServer.batch <- b
		}
	}
}
