package binding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/blacklist"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/applog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/simplecache"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Poller", func() {
	var (
		apiClient    *fakeAPIClient
		store        *fakeStore
		metrics      *metricsHelpers.SpyMetricsRegistry
		logger       = log.New(GinkgoWriter, "", 0)
		appLogStream applog.AppLogStream
		logClient    = testhelper.NewSpyLogClient()
	)

	BeforeEach(func() {
		apiClient = newFakeAPIClient()
		store = newFakeStore()
		metrics = metricsHelpers.NewMetricsRegistry()
		factory := applog.NewAppLogStreamFactory()
		logClient = testhelper.NewSpyLogClient()
		appLogStream = factory.NewAppLogStream(logClient, "test")
	})

	It("polls for bindings on an interval", func() {
		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, appLogStream, &dummyIPChecker{}, false)
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

		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, appLogStream, &dummyIPChecker{}, false)
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

		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, appLogStream, &dummyIPChecker{}, false)
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
		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, appLogStream, &dummyIPChecker{}, false)
		go p.Poll()

		apiClient.errors <- errors.New("expected")

		Eventually(func() float64 {
			return metrics.GetMetric("binding_refresh_error", nil).Value()
		}).Should(BeNumerically("==", 1))
	})

	It("does not update the stores if the response code is bad", func() {
		apiClient.statusCode <- 404

		p := NewPoller(apiClient, 10*time.Millisecond, store, metrics, logger, appLogStream, &dummyIPChecker{}, false)
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
		NewPoller(apiClient, time.Hour, store, metrics, logger, appLogStream, &dummyIPChecker{}, true)

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
			},
		}
		NewPoller(apiClient, time.Hour, store, metrics, logger, appLogStream, &dummyIPChecker{}, false)

		Expect(metrics.GetMetric("last_binding_refresh_count", nil).Value()).
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
		It("returns no binding which contains an invalid URL and increases invalid_drains", func() {
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
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				&dummyIPChecker{},
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("No hostname found in syslog drain url syslog:/invalid-url-a-slash-is-missing")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 1))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 0))
		})

		It("returns no binding which contains an invalid scheme in URL and increases invalid_drains", func() {
			bindings := []Binding{
				{
					Url: "syslog-ssl://drain-0",
					Credentials: []Credentials{
						{
							Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				&dummyIPChecker{},
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("Invalid Scheme for syslog drain url syslog-ssl://drain-0")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 1))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 0))
		})

		It("returns no binding with unresolvable URL and increases invalid_drains", func() {
			bindings := []Binding{
				{
					Url: "syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com",
					Credentials: []Credentials{
						{
							Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				&unresolvableIPChecker{},
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("Cannot resolve ip address for syslog drain with url syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 1))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 0))
		})

		It("returns no binding which has a blacklisted IP and increases invalid_drains and blacklisted drains", func() {
			bindings := []Binding{
				{
					Url: "syslog://192.168.188.15",
					Credentials: []Credentials{
						{
							Cert: "cert0", Key: "key0", CA: "ca0", Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
			}
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)
			blacklistRanges, _ := blacklist.NewBlacklistRanges(
				blacklist.BlacklistRange{Start: "192.168.188.1", End: "192.168.188.255"},
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				blacklistRanges,
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("Resolved ip address for syslog drain with url syslog://192.168.188.15 is blacklisted")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 1))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 1))
		})

		It("returns no binding when there is a prior IP checking failure for URL and increases invalid_drains", func() {
			bindings := []Binding{
				{
					Url: "syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname0", AppID: "app-id-0"}},
						},
					},
				},
				{
					Url: "syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com",
					Credentials: []Credentials{
						{
							Apps: []App{{Hostname: "app-hostname1", AppID: "app-id-1"}},
						},
					},
				},
			}
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)
			blacklistRanges, _ := blacklist.NewBlacklistRanges(
				blacklist.BlacklistRange{Start: "192.168.188.1", End: "192.168.188.255"},
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				blacklistRanges,
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("Skipped resolve ip address for syslog drain with url syslog://syslog-drain-test-37c4f6db-12e2-4206-8bb2-c8d6f440d4d2.example.com due to prior failure")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 2))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 0))
		})

		It("returns no binding when key pair cannot be loaded and does not increase invalid_drains and blacklisted drains", func() {
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
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				&dummyIPChecker{},
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("failed to load certificate for syslog-tls://drain-0")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 0))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 0))
		})

		It("returns no binding when CA cannot be loaded and does not increase invalid_drains and blacklisted drains", func() {
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
			cache := simplecache.New[string, bool](120 * time.Second)
			blacklistedDrainsGauge := metrics.NewGauge(
				"blacklisted_drains",
				"Count of blacklisted drains encountered in last binding fetch.",
			)
			invalidDrainsGauge := metrics.NewGauge(
				"invalid_drains",
				"Count of invalid drains encountered in last binding fetch. Includes blacklisted drains.",
			)

			filteredBindings := checkBindings(
				bindings,
				&appLogStream,
				&dummyIPChecker{},
				logger,
				cache,
				blacklistedDrainsGauge,
				invalidDrainsGauge,
				true,
			)

			Expect(filteredBindings).To(BeEmpty())
			Expect(logClient.Message()).To(ContainElement(Equal("failed to load root CA for syslog-tls://drain-0")))
			Expect(metrics.GetMetricValue("invalid_drains", map[string]string{})).To(BeNumerically("==", 0))
			Expect(metrics.GetMetricValue("blacklisted_drains", map[string]string{})).To(BeNumerically("==", 0))
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
	return net.IPv4(127, 0, 0, 1), nil
}

func (*dummyIPChecker) CheckBlacklist(ip net.IP) error {
	return nil
}

type unresolvableIPChecker struct{}

func (d *unresolvableIPChecker) ResolveAddr(host string) (net.IP, error) {
	return nil, fmt.Errorf("unable to resolve DNS entry: %s", host)
}

func (*unresolvableIPChecker) CheckBlacklist(ip net.IP) error {
	return nil
}
