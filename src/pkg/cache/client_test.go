package cache_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

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
		addr = "https://cache.example.com"
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
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.example.com/v2/bindings"))
	})

	It("returns legacy bindings from the cache", func() {
		bindings := []binding.LegacyBinding{
			{
				AppID:       "app-id-1",
				Drains:      []string{"drain-1"},
				Hostname:    "host-1",
				V2Available: true,
			},
		}

		j, err := json.Marshal(bindings)
		Expect(err).ToNot(HaveOccurred())
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(j)),
		}

		Expect(client.LegacyGet()).To(Equal(bindings))
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.example.com/bindings"))
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
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.example.com/v2/aggregate"))
	})

	It("returns legacy aggregate drains from the cache", func() {
		bindings := []binding.LegacyBinding{
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
			Body:       io.NopCloser(bytes.NewReader(j)),
		}

		Expect(client.GetLegacyAggregate()).To(Equal(bindings))
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.example.com/aggregate"))
	})

	It("returns aggregate metric drains from the cache", func() {
		drains := map[string]any{
			"logging": map[string]any{
				"verbosity": "detailed",
			},
		}

		j, err := json.Marshal(drains)
		Expect(err).ToNot(HaveOccurred())
		spyHTTPClient.response = &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(j)),
		}

		Expect(client.GetAggregateMetric()).To(Equal(drains))
		Expect(spyHTTPClient.requestURL).To(Equal("https://cache.example.com/v2/aggregatemetric"))
	})

	Context("when an http error occurs", func() {
		BeforeEach(func() {
			spyHTTPClient.err = errors.New("http error")
		})
		Context("Aggregate", func() {
			It("errors", func() {
				_, err := client.GetAggregate()
				Expect(err).To(MatchError("http error"))
			})
		})
		Context("App Drains", func() {
			It("errors", func() {
				_, err := client.Get()
				Expect(err).To(MatchError("http error"))
			})
		})
		Context("Legacy App Drains", func() {
			It("errors", func() {
				_, err := client.LegacyGet()
				Expect(err).To(MatchError("http error"))
			})
		})
		Context("Legacy Aggregate Drains", func() {
			It("errors", func() {
				_, err := client.GetLegacyAggregate()
				Expect(err).To(MatchError("http error"))
			})
		})
		Context("Aggregate Metric", func() {
			It("errors", func() {
				_, err := client.GetAggregateMetric()
				Expect(err).To(MatchError("http error"))
			})
		})
	})
	Context("when server responds with a non-200 response code", func() {
		BeforeEach(func() {
			spyHTTPClient.response = &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte{})),
			}
		})
		Context("Aggregate", func() {
			It("errors", func() {
				_, err := client.GetAggregate()
				Expect(err).To(MatchError("unexpected http response from binding cache: 404"))
			})
		})
		Context("App Drains", func() {
			It("errors", func() {
				_, err := client.Get()
				Expect(err).To(MatchError("unexpected http response from binding cache: 404"))
			})
		})
		Context("Legacy App Drains", func() {
			It("errors", func() {
				_, err := client.LegacyGet()
				Expect(err).To(MatchError("unexpected http response from binding cache: 404"))
			})
		})
		Context("Legacy Aggregate Drains", func() {
			It("errors", func() {
				_, err := client.GetLegacyAggregate()
				Expect(err).To(MatchError("unexpected http response from binding cache: 404"))
			})
		})
		Context("Aggregate Metric", func() {
			It("errors", func() {
				_, err := client.GetAggregateMetric()
				Expect(err).To(MatchError("unexpected http response from binding cache: 404"))
			})
		})
	})

	Context("when the server responds with malformed JSON", func() {
		BeforeEach(func() {
			spyHTTPClient.response = &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader([]byte("{"))),
			}
		})
		Context("Aggregate", func() {
			It("errors", func() {
				_, err := client.GetAggregate()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("App Drains", func() {
			It("errors", func() {
				_, err := client.Get()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("Legacy App Drains", func() {
			It("errors", func() {
				_, err := client.LegacyGet()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("Legacy Aggregate Drains", func() {
			It("errors", func() {
				_, err := client.GetLegacyAggregate()
				Expect(err).To(HaveOccurred())
			})
		})
		Context("Aggregate Metric", func() {
			It("errors", func() {
				_, err := client.GetAggregateMetric()
				Expect(err).To(HaveOccurred())
			})
		})
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

func (s *spyHTTPClient) LegacyGet(url string) (*http.Response, error) {
	s.requestURL = url
	return s.response, s.err
}

func (s *spyHTTPClient) GetAggregate(url string) (*http.Response, error) {
	s.requestURL = url
	return s.response, s.err
}
