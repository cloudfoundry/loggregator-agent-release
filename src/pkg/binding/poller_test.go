package binding

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/simplecache"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Poller", func() {
	var (
		logBuffer *gbytes.Buffer
		logger    *log.Logger

		apiClient *fakeAPIClient
		store     *fakeStore
		metrics   *metricsHelpers.SpyMetricsRegistry
		logClient v2.LogClient
	)

	BeforeEach(func() {
		logBuffer = gbytes.NewBuffer()
		logger = log.New(logBuffer, "", 0)
		apiClient = newFakeAPIClient()
		store = newFakeStore()
		metrics = metricsHelpers.NewMetricsRegistry()
		logClient = testhelper.NewSpyLogClient()
	})

	It("polls for bindings on an interval", func() {
		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, logClient, &dummyIPChecker{}, false)
		go p.Poll()

		Eventually(apiClient.called).Should(BeNumerically(">=", 2))
	})

	It("calls the api client and stores the result", func() {
		apiClient.bindings <- response{
			Results: []Binding{
				{
					Url: "syslog://drain-0",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
						{
							Apps: []App{
								{Hostname: "app-hostname1", AppID: "app-id-1"},
								{Hostname: "app-hostname2", AppID: "app-id-2"},
							},
						},
					},
				},
				{
					Url: "syslog://drain-1",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			},
		}

		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, logClient, &dummyIPChecker{}, false)
		go p.Poll()

		var expectedBindings []Binding
		Eventually(store.bindings).Should(Receive(&expectedBindings))
		Expect(expectedBindings).To(ConsistOf([]Binding{
			{
				Url: "syslog://drain-0",
				Credentials: []Credentials{
					{
						Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
					},
					{
						Apps: []App{
							{Hostname: "app-hostname1", AppID: "app-id-1"},
							{Hostname: "app-hostname2", AppID: "app-id-2"},
						},
					},
				},
			},
			{
				Url: "syslog://drain-1",
				Credentials: []Credentials{
					{
						Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
					},
				},
			},
		}))
	})

	It("fetches the next page of bindings and stores the result", func() {
		apiClient.bindings <- response{
			NextID: 2,
			Results: []Binding{
				{
					Url: "syslog://drain-0",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
				{
					Url: "syslog://drain-1",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname1", AppID: "app-id-1"}},
						},
					},
				},
			},
		}

		apiClient.bindings <- response{
			Results: []Binding{
				{
					Url: "syslog://drain-2",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname2", AppID: "app-id-2"}},
						},
					},
				},
				{
					Url: "syslog://drain-3",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname3", AppID: "app-id-3"}},
						},
					},
				},
			},
		}

		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, logClient, &dummyIPChecker{}, false)
		go p.Poll()

		var expectedBindings []Binding
		Eventually(store.bindings).Should(Receive(&expectedBindings))
		Expect(expectedBindings).To(ConsistOf(
			[]Binding{
				{
					Url: "syslog://drain-0",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
				{
					Url: "syslog://drain-1",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname1", AppID: "app-id-1"}},
						},
					},
				},
				{
					Url: "syslog://drain-2",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname2", AppID: "app-id-2"}},
						},
					},
				},
				{
					Url: "syslog://drain-3",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname3", AppID: "app-id-3"}},
						},
					},
				},
			},
		))

		Expect(apiClient.requestedIDs).To(ConsistOf(0, 2))
	})

	It("tracks the number of API errors", func() {
		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, logClient, &dummyIPChecker{}, false)
		go p.Poll()

		apiClient.errors <- errors.New("expected")

		Eventually(func() float64 {
			return metrics.GetMetric("binding_refresh_error", nil).Value()
		}).Should(BeNumerically("==", 1))
	})

	It("does not update the stores if the response code is bad", func() {
		apiClient.statusCode <- 404

		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, logClient, &dummyIPChecker{}, false)
		go p.Poll()

		Eventually(store.bindings).Should(BeEmpty())
	})

	It("tracks the number of bindings returned from CAPI", func() {
		apiClient.bindings <- response{
			Results: []Binding{
				{
					Url: "syslog://drain-0.example.com",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
				{
					Url: "syslog://drain-1.example.com",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname1", AppID: "app-id-1"}},
						},
					},
				},
			},
		}
		NewPoller(apiClient, time.Hour, store, metrics, logger, logClient, &dummyIPChecker{}, true)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
			To(BeNumerically("==", 2))
	})

	It("filters invalid bindings", func() {
		apiClient.bindings <- response{
			Results: []Binding{
				{
					Url: "drain-0",
					Credentials: []Credentials{
						{
							Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
				{
					Url: "drain-1",
					Credentials: []Credentials{
						{
							Cert: "cert1", Key: "key1", CA: "ca1", Apps: []App{{Hostname: "app-hostname1", AppID: "app-id-1"}},
						},
					},
				},
				{
					Url: "syslog://drain-2.example.com",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname2", AppID: "app-id-2"}},
						},
					},
				},
				{
					Url: "syslog://blacklisted_domain",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			},
		}
		NewPoller(apiClient, time.Hour, store, metrics, logger, logClient, &dummyIPChecker{}, false)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
			To(BeNumerically("==", 1))
		tags := map[string]string{"unit": "total"}
		Expect(metrics.GetMetric("invalid_drains", tags).Value()).
			To(BeNumerically("==", 3))
		Expect(metrics.GetMetric("blacklisted_drains", tags).Value()).
			To(BeNumerically("==", 1))
	})

	It("tracks the isolated CalculateBindingsCount call", func() {
		noBinding := []Binding{}
		singleBinding := []Binding{
			{
				Url: "drain-0",
				Credentials: []Credentials{
					{
						Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
					},
				},
			},
			{
				Url: "drain-1",
				Credentials: []Credentials{
					{
						Cert: "cert1", Key: "key1", CA: "ca1", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
					},
				},
			},
		}
		multipleBindings := []Binding{
			{
				Url: "drain-0",
				Credentials: []Credentials{
					{
						Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
					},
				},
			},
			{
				Url: "drain-1",
				Credentials: []Credentials{
					{
						Cert: "cert1", Key: "key1", CA: "ca1", Apps: []App{{Hostname: "app-hostname1", AppID: "app-id-1"}},
					},
				},
			},
		}
		Expect(CalculateBindingCount(noBinding)).
			To(BeNumerically("==", 0))
		Expect(CalculateBindingCount(singleBinding)).
			To(BeNumerically("==", 1))
		Expect(CalculateBindingCount(multipleBindings)).
			To(BeNumerically("==", 2))
	})

	Describe("checkBindings", func() {

		var bndChecker *bindingChecker
		var logClient *testhelper.SpyLogClient

		BeforeEach(func() {
			logBuffer.Clear() //nolint:errcheck
			logClient = testhelper.NewSpyLogClient()

			bndChecker = &bindingChecker{
				appLogClient:     logClient,
				logger:           logger,
				checker:          &dummyIPChecker{},
				failedHostsCache: simplecache.New[string, bool](120 * time.Second),
				warn:             true,
			}
		})

		It("returns no binding and writes an error if no credentials are set for a binding", func() {

			bindings := []Binding{
				{
					Url: "syslog://my-syslog-servers.com",
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say("No credentials - which include appIDs - for a binding. Check the bindings in the cloud controller."))
		})

		It("returns no binding and writes an error if the binding url cannot be parsed", func() {

			bindings := []Binding{
				{
					Url: "http://example.com/\x7f",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say("Cannot parse syslog drain URL. for app app-id-0"))
			Expect(logClient.Message()).To(ContainElement(Equal("Cannot parse syslog drain URL.")))
			Expect(len(logClient.Message())).To(Equal(1))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns no binding and writes an error if the binding url has invalid scheme", func() {

			bindings := []Binding{
				{
					Url: "syslog-ssl://drain-0.com?user=trlala&password=123213",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say("Invalid Scheme syslog-ssl for syslog drain url syslog-ssl://drain-0.com for app app-id-0"))
			Expect(logClient.Message()).To(ContainElement(Equal("Invalid Scheme syslog-ssl for syslog drain url syslog-ssl://drain-0.com")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns no binding if a host cannot be parsed from the given url", func() {
			bindings := []Binding{
				{
					Url: "syslog:/invalid-url-a-slash-is-missing",
					Credentials: []Credentials{
						{
							Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say(("No hostname found in syslog drain url syslog:/invalid-url-a-slash-is-missing for app app-id-0")))
			Expect(logClient.Message()).To(ContainElement(Equal("No hostname found in syslog drain url syslog:/invalid-url-a-slash-is-missing")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns no binding when there is a prior IP checking failure for a URL", func() {
			bindings := []Binding{
				{
					Url: "syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}
			cache := simplecache.New[string, bool](120 * time.Second)
			cache.Set("syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com", true)
			bndChecker.failedHostsCache = cache

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say(("Skipped resolve ip address for syslog drain with url syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com due to prior failure for app app-id-0")))
			Expect(logClient.Message()).To(ContainElement(Equal("Skipped resolve ip address for syslog drain with url syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com due to prior failure")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(0)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns no binding when the URL cannot be resolved", func() {
			bindings := []Binding{
				{
					Url: "syslog://fail_to_resolve_ip",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say(("Cannot resolve ip address for syslog drain with url syslog://fail_to_resolve_ip for app app-id-0")))
			Expect(logClient.Message()).To(ContainElement(Equal("Cannot resolve ip address for syslog drain with url syslog://fail_to_resolve_ip")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns no binding which has a blacklisted IP", func() {
			bindings := []Binding{
				{
					Url: "syslog://blacklisted_domain",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say(("Resolved ip address for syslog drain with url syslog://blacklisted_domain is blacklisted for app app-id-0")))
			Expect(logClient.Message()).To(ContainElement(Equal("Resolved ip address for syslog drain with url syslog://blacklisted_domain is blacklisted")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(1)))
		})

		It("returns no binding when key pair cannot be loaded", func() {
			bindings := []Binding{
				{
					Url: "syslog-tls://drain-0",
					Credentials: []Credentials{
						{
							Cert: "cert0", Key: "key0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say(("failed to load certificate for syslog-tls://drain-0 for app app-id-0")))
			Expect(logClient.Message()).To(ContainElement(Equal("failed to load certificate for syslog-tls://drain-0")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns no binding when CA cannot be loaded", func() {
			bindings := []Binding{
				{
					Url: "syslog-tls://drain-0",
					Credentials: []Credentials{
						{
							CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logBuffer).Should(gbytes.Say(("failed to load root CA for syslog-tls://drain-0 for app app-id-0")))
			Expect(logClient.Message()).To(ContainElement(Equal("failed to load root CA for syslog-tls://drain-0")))
			Expect(bndChecker.invalidDrains).To(Equal(float64(1)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		It("returns bindings when there are no certificates set", func() {
			bindings := []Binding{
				{
					Url: "syslog-tls://drain-0",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}

			filteredBindings := bndChecker.checkBindings(bindings)

			Expect(filteredBindings).To(HaveLen(1))
			Expect(logBuffer.Contents()).Should(BeEmpty())
			Expect(logClient.Message()).To(BeEmpty())
			Expect(bndChecker.invalidDrains).To(Equal(float64(0)))
			Expect(bndChecker.blacklistedDrains).To(Equal(float64(0)))
		})

		Context("when both include-log-source-types and exclude-log-source-types are specified", func() {
			It("ignores the drain and counts as invalid", func() {
				bindings := []Binding{
					{
						Url: "https://test.org/drain?include-log-source-types=app&exclude-log-source-types=rtr",
						Credentials: []Credentials{
							{
								Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
							},
						},
					},
				}

				filteredBindings := bndChecker.checkBindings(bindings)

				Expect(filteredBindings).To(BeEmpty())
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("include-log-source-types and exclude-log-source-types cannot be used at the same time")))
				Expect(bndChecker.invalidDrains).To(BeNumerically("==", 1))
				Expect(bndChecker.blacklistedDrains).To(BeNumerically("==", 0))
			})

			It("doesn't log the conflicting filters warning when warn is false", func() {
				bndChecker.warn = false
				bindings := []Binding{
					{
						Url: "https://test.org/drain?include-log-source-types=app&exclude-log-source-types=rtr",
						Credentials: []Credentials{
							{
								Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
							},
						},
					},
				}
				bndChecker.checkBindings(bindings)

				for _, msg := range logClient.Message() {
					Expect(msg).ToNot(MatchRegexp("include-log-source-types and exclude-log-source-types cannot be used at the same time"))
				}
			})
		})

		Context("when unknown source types are provided", func() {
			It("logs a warning and ignores the drain in include mode", func() {
				bindings := []Binding{
					{
						Url: "https://test.org/drain?include-log-source-types=app,unknown,invalid,rtr",
						Credentials: []Credentials{
							{
								Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
							},
						},
					},
				}
				filteredBindings := bndChecker.checkBindings(bindings)

				Expect(filteredBindings).To(BeEmpty())
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("Unknown source types")))
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("unknown")))
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("invalid")))
				Expect(bndChecker.invalidDrains).To(BeNumerically("==", 1))
			})

			It("logs a warning and ignores the drain in exclude mode", func() {
				bindings := []Binding{
					{
						Url: "https://test.org/drain?exclude-log-source-types=rtr,unknown",
						Credentials: []Credentials{
							{
								Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
							},
						},
					},
				}
				filteredBindings := bndChecker.checkBindings(bindings)

				Expect(filteredBindings).To(BeEmpty())
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("Unknown source types")))
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("unknown")))
				Expect(bndChecker.invalidDrains).To(BeNumerically("==", 1))
			})

			It("logs a warning and ignores the drain when source types have spaces", func() {
				bindings := []Binding{
					{
						Url: "https://test.org/drain?include-log-source-types=app, rtr",
						Credentials: []Credentials{
							{
								Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
							},
						},
					},
				}
				filteredBindings := bndChecker.checkBindings(bindings)

				Expect(filteredBindings).To(BeEmpty())
				Expect(logClient.Message()).To(ContainElement(MatchRegexp("Unknown source types")))
				Expect(bndChecker.invalidDrains).To(BeNumerically("==", 1))
			})

			It("doesn't log the warning when warn is false", func() {
				bndChecker.warn = false
				bindings := []Binding{
					{
						Url: "https://test.org/drain?include-log-source-types=app,unknown,rtr",
						Credentials: []Credentials{
							{
								Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
							},
						},
					},
				}
				bndChecker.checkBindings(bindings)

				for _, msg := range logClient.Message() {
					Expect(msg).ToNot(MatchRegexp("Unknown source types"))
				}
			})
		})
	})
})

type fakeAPIClient struct {
	numRequests  int64
	bindings     chan response
	errors       chan error
	statusCode   chan int
	requestedIDs []int
}

func newFakeAPIClient() *fakeAPIClient {
	return &fakeAPIClient{
		bindings:   make(chan response, 100),
		errors:     make(chan error, 100),
		statusCode: make(chan int, 100),
	}
}

func (c *fakeAPIClient) Get(nextID int) (*http.Response, error) {
	atomic.AddInt64(&c.numRequests, 1)

	var binding response
	var statusCode = 200
	select {
	case err := <-c.errors:
		return nil, err
	case binding = <-c.bindings:
		c.requestedIDs = append(c.requestedIDs, nextID)
	case injectedStatusCode := <-c.statusCode:
		statusCode = injectedStatusCode
	default:
	}

	var body []byte
	b, err := json.Marshal(&binding)
	Expect(err).ToNot(HaveOccurred())
	body = b
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewReader(body)),
	}
	resp.StatusCode = statusCode

	return resp, err
}

func (c *fakeAPIClient) called() int64 {
	return atomic.LoadInt64(&c.numRequests)
}

type fakeStore struct {
	bindings chan []Binding
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		bindings: make(chan []Binding, 100),
	}
}

func (c *fakeStore) Set(b []Binding, bindingCount int) {
	c.bindings <- b
}

type response struct {
	Results []Binding
	NextID  int `json:"next_id"`
}

type dummyIPChecker struct{}

func (d *dummyIPChecker) ResolveAddr(host string) (net.IP, error) {
	if host == "fail_to_resolve_ip" {
		return net.IPv4(127, 0, 0, 1), errors.New(host)
	}
	if host == "blacklisted_domain" {
		return net.IPv4(192, 168, 188, 15), nil
	}

	return net.IPv4(127, 0, 0, 1), nil
}

func (*dummyIPChecker) CheckBlacklist(ip net.IP) error {
	if ip.String() == "192.168.188.15" {
		return errors.New(ip.String())
	}
	return nil
}
