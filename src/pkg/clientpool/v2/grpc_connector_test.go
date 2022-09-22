package v2_test

import (
	"errors"
	"io"
	"net"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/clientpool/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type SpyFetcher struct {
	Addr   string
	Closer io.Closer
	Client loggregator_v2.Ingress_BatchSenderClient
}

func (f *SpyFetcher) Fetch(
	addr string,
) (conn io.Closer, client loggregator_v2.Ingress_BatchSenderClient, err error) {
	f.Addr = addr
	return f.Closer, f.Client, nil
}

type SpyStream struct {
	loggregator_v2.Ingress_BatchSenderClient
}

var _ = Describe("GRPCConnector", func() {
	Context("when first balancer returns an address", func() {
		var (
			fetcher   *SpyFetcher
			connector v2.GRPCConnector
		)

		BeforeEach(func() {
			balancers := []*v2.Balancer{
				v2.NewBalancer("z1.doppler.com:99", v2.WithLookup(func(string) ([]net.IP, error) {
					return []net.IP{net.ParseIP("10.10.10.1")}, nil
				})),
				v2.NewBalancer("doppler.com:99", v2.WithLookup(func(string) ([]net.IP, error) {
					return []net.IP{net.ParseIP("1.1.1.1")}, nil
				})),
			}
			fetcher = &SpyFetcher{
				Closer: io.NopCloser(nil),
				Client: SpyStream{},
			}
			connector = v2.MakeGRPCConnector(fetcher, balancers)
		})

		It("fetches a client with the given address", func() {
			_, _, err := connector.Connect()
			Expect(err).To(BeNil())
			Expect(fetcher.Addr).To(Equal("10.10.10.1:99"))
		})

		It("returns the connection as a closer and client", func() {
			closer, client, err := connector.Connect()

			Expect(err).ToNot(HaveOccurred())
			Expect(closer).To(Equal(fetcher.Closer))
			Expect(client).To(Equal(fetcher.Client))
		})
	})

	Context("when first balancer does not return an address", func() {
		It("dials the next balancer", func() {
			balancers := []*v2.Balancer{
				v2.NewBalancer("z1.doppler.com:99", v2.WithLookup(func(string) ([]net.IP, error) {
					return nil, errors.New("Should not get here")
				})),
				v2.NewBalancer("doppler.com:99", v2.WithLookup(func(string) ([]net.IP, error) {
					return []net.IP{net.ParseIP("1.1.1.1")}, nil
				})),
			}
			fetcher := &SpyFetcher{
				Closer: io.NopCloser(nil),
				Client: SpyStream{},
			}
			connector := v2.MakeGRPCConnector(fetcher, balancers)

			_, _, err := connector.Connect()
			Expect(err).To(BeNil())
			Expect(fetcher.Addr).To(Equal("1.1.1.1:99"))
		})
	})

	Context("when the none balancer return anything", func() {
		It("returns an error", func() {
			balancers := []*v2.Balancer{
				v2.NewBalancer("z1.doppler.com:99", v2.WithLookup(func(string) ([]net.IP, error) {
					return nil, errors.New("Should not get here")
				})),
				v2.NewBalancer("doppler.com:99", v2.WithLookup(func(string) ([]net.IP, error) {
					return nil, errors.New("Should not get here")
				})),
			}
			fetcher := &SpyFetcher{
				Closer: io.NopCloser(nil),
				Client: SpyStream{},
			}
			connector := v2.MakeGRPCConnector(fetcher, balancers)

			_, _, err := connector.Connect()
			Expect(err).To(HaveOccurred())
		})
	})
})
