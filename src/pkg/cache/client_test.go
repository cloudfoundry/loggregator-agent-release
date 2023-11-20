package cache_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
)

var _ = Describe("Client", func() {
	var (
		spyHTTPClient *spyHTTPClient
		addr          string
		client        *cache.CacheClient
	)

	BeforeEach(func() {
		spyHTTPClient = newSpyHTTPClient()
		addr = "https://cache.address.com"
		client = cache.NewClient(addr, spyHTTPClient)
	})

	It("returns bindings from the cache", func() {
		bindings := []binding.Binding{
			{
				Url: "drain-1",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "host-1", AppID: "app-id-1"}},
					},
				},
			},
		}

		j, err := json.Marshal(bindings)
		Expect(err).ToNot(HaveOccurred())
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(j)),
		}

		Expect(client.Get()).To(Equal(bindings))
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.address.com/v2/bindings"))
	})

	It("returns aggregate drains from the cache", func() {
		bindings := []binding.Binding{
			{
				Url: "url",
				Credentials: []binding.Credentials{
					{
						Cert: "cert",
						Key:  "key",
						CA:   "ca",
					},
				},
			},
		}

		j, err := json.Marshal(bindings)
		Expect(err).ToNot(HaveOccurred())
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(j)),
		}

		Expect(client.GetAggregate()).To(Equal(bindings))
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.address.com/v2/aggregate"))
	})

	It("returns empty bindings if an HTTP error occurs", func() {
		spyHTTPClient.err = errors.New("http error")

		_, err := client.Get()

		Expect(err).To(MatchError("http error"))

		_, err = client.GetAggregate()

		Expect(err).To(MatchError("http error"))
	})

	It("returns empty bindings if cache returns a non-OK status code", func() {
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		_, err := client.Get()

		Expect(err).To(MatchError("unexpected http response from binding cache: 500"))

		_, err = client.GetAggregate()

		Expect(err).To(MatchError("unexpected http response from binding cache: 500"))
	})
})

type spyHTTPClient struct {
	response   *http.Response
	requestURL string
	err        error
}

func newSpyHTTPClient() *spyHTTPClient {
	return &spyHTTPClient{}
}

func (s *spyHTTPClient) Get(url string) (*http.Response, error) {
	s.requestURL = url
	return s.response, s.err
}

func (s *spyHTTPClient) GetAggregate(url string) (*http.Response, error) {
	s.requestURL = url
	return s.response, s.err
}
