package v1_test

import (
	"io"
	"net"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	v1 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/clientpool/v1"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
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

	It("opens a stream to the server", func() {
		server := newSpyIngestorServer()
		Expect(server.Start()).To(Succeed())
		defer func() {
			server.Stop()
		}()

		fetcher := v1.NewPusherFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		var (
			closer io.Closer
			pusher plumbing.DopplerIngestor_PusherClient
		)
		f := func() error {
			var err error
			closer, pusher, err = fetcher.Fetch(server.addr)
			return err
		}
		Eventually(f).ShouldNot(HaveOccurred())

		err := pusher.Send(&plumbing.EnvelopeData{})
		Expect(err).ToNot(HaveOccurred())

		Eventually(server.envelopeData).Should(Receive())
		Expect(closer.Close()).To(Succeed())
	})

	It("increments a counter when a connection is established", func() {
		server := newSpyIngestorServer()
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v1.NewPusherFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		f := func() error {
			_, _, err := fetcher.Fetch(server.addr)
			return err
		}
		Eventually(f).ShouldNot(HaveOccurred())

		tags := map[string]string{"metric_version": "2.0"}
		Expect(mc.GetMetric("doppler_connections", tags).Value()).To(Equal(1.0))
		Expect(mc.GetMetric("doppler_v1_streams", tags).Value()).To(Equal(1.0))
	})

	It("decrements a counter when a connection is closed", func() {
		server := newSpyIngestorServer()
		Expect(server.Start()).To(Succeed())
		defer server.Stop()

		fetcher := v1.NewPusherFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		var closer io.Closer
		f := func() error {
			var err error
			closer, _, err = fetcher.Fetch(server.addr)
			return err
		}
		Eventually(f).ShouldNot(HaveOccurred())

		closer.Close()
		tags := map[string]string{"metric_version": "2.0"}
		Expect(mc.GetMetric("doppler_connections", tags).Value()).To(Equal(0.0))
		Expect(mc.GetMetric("doppler_v1_streams", tags).Value()).To(Equal(0.0))
	})

	It("returns an error when the server is unavailable", func() {
		fetcher := v1.NewPusherFetcher(mc, grpc.WithTransportCredentials(insecure.NewCredentials()))
		_, _, err := fetcher.Fetch("127.0.0.1:1122")
		Expect(err).To(HaveOccurred())
	})
})

type SpyIngestorServer struct {
	plumbing.UnimplementedDopplerIngestorServer

	addr         string
	server       *grpc.Server
	stop         chan struct{}
	envelopeData chan *plumbing.EnvelopeData
}

func newSpyIngestorServer() *SpyIngestorServer {
	return &SpyIngestorServer{
		stop:         make(chan struct{}),
		envelopeData: make(chan *plumbing.EnvelopeData),
	}
}

func (s *SpyIngestorServer) Start() error {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	s.server = grpc.NewServer()
	s.addr = lis.Addr().String()
	plumbing.RegisterDopplerIngestorServer(s.server, s)

	go s.server.Serve(lis)

	return nil
}

func (s *SpyIngestorServer) Stop() {
	close(s.stop)
	s.server.Stop()
}

func (s *SpyIngestorServer) Pusher(p plumbing.DopplerIngestor_PusherServer) error {
	for {
		select {
		case <-s.stop:
			return nil
		default:
			env, err := p.Recv()
			if err != nil {
				return err
			}

			s.envelopeData <- env
		}
	}
}
