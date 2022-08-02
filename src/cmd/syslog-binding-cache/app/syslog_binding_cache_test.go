package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/tlsconfig"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/syslog-binding-cache/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
)

var _ = Describe("SyslogBindingCache", func() {
	var (
		logger       = log.New(GinkgoWriter, "", log.LstdFlags)
		metricClient *metricsHelpers.SpyMetricsRegistry
		config       app.Config

		capi *fakeCC
		sbc  *app.SyslogBindingCache

		cachePort = 40000

		capiTestCerts         = testhelper.GenerateCerts("capiCA")
		bindingCacheTestCerts = testhelper.GenerateCerts("bindingCacheCA")
	)

	BeforeEach(func() {
		r := results{
			{

				Url:  "syslog://drain-a",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "org.space.app-name", AppID: "app-id-1"},
				},
			},
			{

				Url:  "syslog://drain-b",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "org.space.app-name", AppID: "app-id-1"},
				},
			},
			{

				Url:  "syslog://drain-c",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "org.space.app-name-2", AppID: "app-id-2"},
				},
			},
			{

				Url:  "syslog://drain-d",
				Cert: "cert",
				Key:  "key",
				Apps: []binding.App{
					{Hostname: "org.space.app-name-2", AppID: "app-id-2"},
				},
			},
		}

		capi = &fakeCC{
			results: r,
		}
		capi.startTLS(capiTestCerts)

		config = app.Config{
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
			AggregateDrains:    []string{"syslog://drain-e", "syslog://drain-f"},
		}
		metricClient = metricsHelpers.NewMetricsRegistry()
	})

	AfterEach(func() {
		capi.CloseClientConnections()
		capi.Close()

		cachePort++
	})

	It("debug metrics arn't enabled by default", func() {
		config.MetricsServer.PprofPort = 1234
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		Consistently(metricClient.GetDebugMetricsEnabled()).Should(BeFalse())
		Consistently(func() error {
			_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", config.MetricsServer.PprofPort))
			return err
		}).ShouldNot(BeNil())
	})

	It("debug metrics can be enabled", func() {
		config.MetricsServer.DebugMetrics = true
		config.MetricsServer.PprofPort = 1235
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		Eventually(metricClient.GetDebugMetricsEnabled).Should(BeTrue())
		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/", config.MetricsServer.PprofPort))
			return err
		}).Should(BeNil())
		Expect(resp.StatusCode).To(Equal(200))
	})

	It("polls CAPI on an interval for results", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		Eventually(capi.numRequests).Should(BeNumerically(">=", 2))
	})

	It("has an HTTP endpoint that returns bindings", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		client := plumbing.NewTLSHTTPClient(
			bindingCacheTestCerts.Cert("binding-cache"),
			bindingCacheTestCerts.Key("binding-cache"),
			bindingCacheTestCerts.CA(),
			"binding-cache",
		)

		addr := fmt.Sprintf("https://localhost:%d/v2/bindings", cachePort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var results []binding.Binding
		err = json.Unmarshal(body, &results)
		Expect(err).ToNot(HaveOccurred())

		Expect(results).To(HaveLen(4))
		b := findBindings(results, "app-id-1")
		Expect(b[0].Url).To(Equal("syslog://drain-a"))
		Expect(b[1].Url).To(Equal("syslog://drain-b"))
		Expect(b[0].Apps[0].Hostname).To(Equal("org.space.app-name"))

		b = findBindings(results, "app-id-2")
		Expect(b[0].Url).To(Equal("syslog://drain-c"))
		Expect(b[1].Url).To(Equal("syslog://drain-d"))
		Expect(b[0].Apps[0].Hostname).To(Equal("org.space.app-name-2"))
	})

	It("has an HTTP endpoint that returns aggregate drains", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
		client := plumbing.NewTLSHTTPClient(
			bindingCacheTestCerts.Cert("binding-cache"),
			bindingCacheTestCerts.Key("binding-cache"),
			bindingCacheTestCerts.CA(),
			"binding-cache",
		)

		addr := fmt.Sprintf("https://localhost:%d/aggregate", cachePort)

		var resp *http.Response
		Eventually(func() error {
			var err error
			resp, err = client.Get(addr)
			return err
		}).Should(Succeed())

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var result []string
		err = json.Unmarshal(body, &result)
		Expect(err).ToNot(HaveOccurred())

		Expect(result).To(HaveLen(2))
		Expect(result).To(ConsistOf("syslog://drain-e", "syslog://drain-f"))
	})

	It("has an HTTP endpoint that returns legacy bindings", func() {
		sbc = app.NewSyslogBindingCache(config, metricClient, logger)
		go sbc.Run()
		defer sbc.Stop()
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

		body, err := io.ReadAll(resp.Body)
		Expect(err).ToNot(HaveOccurred())

		var results []binding.LegacyBinding
		err = json.Unmarshal(body, &results)
		Expect(err).ToNot(HaveOccurred())

		Expect(results).To(HaveLen(2))
		Expect(results[0].AppID).To(Equal("app-id-1"))
		Expect(results[0].Drains[0]).To(Equal("syslog://drain-a"))
		Expect(results[0].Drains[1]).To(Equal("syslog://drain-b"))
		Expect(results[0].Hostname).To(Equal("org.space.app-name"))

		Expect(results[1].AppID).To(Equal("app-id-2"))
		Expect(results[1].Drains[0]).To(Equal("syslog://drain-c"))
		Expect(results[1].Drains[1]).To(Equal("syslog://drain-d"))
		Expect(results[1].Hostname).To(Equal("org.space.app-name-2"))
	})
})

type results []binding.Binding

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
	if r.URL.Path != "/internal/v5/syslog_drain_urls" {
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

	_, err = w.Write(resultData)
	if err != nil {
		w.WriteHeader(500)
		return
	}

	if f.withEmptyResult {
		f.count++
	}
}

func (f *fakeCC) numRequests() int64 {
	return atomic.LoadInt64(&f.called)
}

func findBindings(bindings []binding.Binding, appID string) []binding.Binding {
	var bindingResult []binding.Binding
	for _, b := range bindings {
		for _, a := range b.Apps {
			if appID == a.AppID {
				bindingResult = append(bindingResult, b)
			}
		}
	}
	//panic(fmt.Sprintf("unable to find binding with appID %s", appID))
	return bindingResult
}
