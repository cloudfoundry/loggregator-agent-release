package cache_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/cache"
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
				AppID:    "app-id-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		j, err := json.Marshal(bindings)
		Expect(err).ToNot(HaveOccurred())
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(j)),
		}

		Expect(client.Get()).To(Equal(bindings))
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.address.com/bindings"))
	})

	It("returns empty bindings if an HTTP error occurs", func() {
		spyHTTPClient.err = errors.New("http error")

		_, err := client.Get()

		Expect(err).To(MatchError("http error"))
	})

	It("returns empty bindings if cache returns a non-OK status code", func() {
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       ioutil.NopCloser(strings.NewReader("")),
		}

		_, err := client.Get()

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
