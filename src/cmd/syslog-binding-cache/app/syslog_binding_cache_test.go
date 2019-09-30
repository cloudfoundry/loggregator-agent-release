package app_test

import (
	"code.cloudfoundry.org/tlsconfig"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-binding-cache/app"
	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/plumbing"
)

var _ = Describe("SyslogBindingCache", func() {
	var (
		logger = log.New(GinkgoWriter, "", log.LstdFlags)

		capi *fakeCC
		sbc  *app.SyslogBindingCache

		cachePort = 40000

		capiTestCerts = testhelper.GenerateCerts("capiCA")
		bindingCacheTestCerts = testhelper.GenerateCerts("bindingCacheCA")
	)

	BeforeEach(func() {
		r := results{
			"app-id-1": appBindings{
				Drains:   []string{"syslog://drain-a", "syslog://drain-b"},
				Hostname: "org.space.app-name",
			},
			"app-id-2": appBindings{
				Drains:   []string{"syslog://drain-c", "syslog://drain-d"},
				Hostname: "org.space.app-name-2",
			},
		}

		capi = &fakeCC{
			results: r,
		}
		capi.startTLS(capiTestCerts)

		config := app.Config{
			APIURL:             capi.URL,
			APIPollingInterval: 10 * time.Millisecond,
			APIBatchSize:       1000,
			APICAFile:          capiTestCerts.CA(),
			APICertFile:        capiTestCerts.Cert("capi"),
			APIKeyFile:         capiTestCerts.Key("capi"),
			APICommonName:      "capi",
			CacheCAFile:        bindingCacheTestCerts.CA(),
			CacheCertFile:      bindingCacheTestCerts.Cert("binding-cache"),
			CacheKeyFile:       bindingCacheTestCerts.Key("binding-cache"),
			CacheCommonName:    "binding-cache",
			CachePort:          cachePort,
		}
		sbc = app.NewSyslogBindingCache(config, metricsHelpers.NewMetricsRegistry(), logger)
		go sbc.Run()
	})

	AfterEach(func() {
		capi.CloseClientConnections()
		capi.Close()

		cachePort++
	})

	It("polls CAPI on an interval for results", func() {
		Eventually(capi.numRequests).Should(BeNumerically(">=", 2))
	})

	It("has an HTTP endpoint that returns bindings", func() {
		client := plumbing.NewTLSHTTPClient(
			bindingCacheTestCerts.Cert("binding-cache"),
			bindingCacheTestCerts.Key("binding-cache"),
			bindingCacheTestCerts.CA(),
			"binding-cache",
		)

		addr := fmt.Sprintf("https://localhost:%d/bindings", cachePort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := ioutil.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var results []binding.Binding
		err = json.Unmarshal(body, &results)
		Expect(err).ToNot(HaveOccurred())

		Expect(results).To(HaveLen(2))
		b := findBinding(results, "app-id-1")
		Expect(b.Drains).To(ConsistOf("syslog://drain-a", "syslog://drain-b"))
		Expect(b.Hostname).To(Equal("org.space.app-name"))

		b = findBinding(results, "app-id-2")
		Expect(b.Drains).To(ConsistOf("syslog://drain-c", "syslog://drain-d"))
		Expect(b.Hostname).To(Equal("org.space.app-name-2"))
	})
})

type results map[string]appBindings

type appBindings struct {
	Drains   []string `json:"drains"`
	Hostname string   `json:"hostname"`
}

type fakeCC struct {
	*httptest.Server
	count           int
	called          int64
	withEmptyResult bool
	results         results
}

func (f *fakeCC) startTLS(testCerts *testhelper.TestCerts) {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			testCerts.Cert("capi"),
			testCerts.Key("capi"),
		),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(testCerts.CA()),
	)

	Expect(err).ToNot(HaveOccurred())

	f.Server = httptest.NewUnstartedServer(f)
	f.Server.TLS = tlsConfig
	f.Server.StartTLS()
}

func (f *fakeCC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&f.called, 1)
	if r.URL.Path != "/internal/v4/syslog_drain_urls" {
		w.WriteHeader(500)
		return
	}

	if r.URL.Query().Get("batch_size") != "1000" {
		w.WriteHeader(500)
		return
	}

	f.serveWithResults(w, r)
}

func (f *fakeCC) serveWithResults(w http.ResponseWriter, r *http.Request) {
	resultData, err := json.Marshal(struct {
		Results results `json:"results"`
	}{
		Results: f.results,
	})
	if err != nil {
		w.WriteHeader(500)
		return
	}

	if f.count > 0 {
		resultData = []byte(`{"results": {}}`)
	}

	w.Write(resultData)
	if f.withEmptyResult {
		f.count++
	}
}

func (f *fakeCC) numRequests() int64 {
	return atomic.LoadInt64(&f.called)
}

func findBinding(bindings []binding.Binding, appID string) binding.Binding {
	for _, b := range bindings {
		if b.AppID == appID {
			return b
		}
	}
	panic(fmt.Sprintf("unable to find binding with appID %s", appID))
}
