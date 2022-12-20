package bindings_test

import (
	"errors"
	"log"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BindingFetcher", func() {
	var (
		getter    *SpyGetter
		fetcher   *bindings.BindingFetcher
		metrics   *metricsHelpers.SpyMetricsRegistry
		maxDrains = 3
		logger    = log.New(GinkgoWriter, "", 0)
	)

	BeforeEach(func() {
		getter = &SpyGetter{}
		metrics = metricsHelpers.NewMetricsRegistry()
		fetcher = bindings.NewBindingFetcher(maxDrains, getter, metrics, logger)
	})

	BeforeEach(func() {
		getter.bindings = []binding.Binding{
			{
				Url:         "syslog://zzz-not-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://other.url",
				Credentials: []binding.Credentials{{CA: "ca", Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}, {Hostname: "org.space.app-name", AppID: "testAppID2"}}}},
			},
			{
				Url:         "syslog://zzz-not-included-again.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "https://other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://other-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
			{
				Url:         "syslog://zzz-not-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			},
			{
				Url:         "syslog://other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			}, {
				Url:         "syslog://zzz-not-included-again.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			}, {
				Url:         "https://other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			}, {
				Url:         "syslog://other-included.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "testAppID"}}}},
			},
		}

		getter.legacyBindings = []binding.LegacyBinding{
			{
				AppID: "9be15160-4845-4f05-b089-40e827ba61f1",
				Drains: []string{
					"syslog://zzz-not-included.url-legacy",
					"syslog://other.url-legacy",
					"syslog://zzz-not-included-again.url-legacy",
					"https://other.url-legacy",
					"syslog://other-included.url-legacy"},
				Hostname:    "org.space.logspinner-legacy",
				V2Available: true,
			},
			{
				AppID: "testAppID",
				Drains: []string{
					"syslog://zzz-not-included.url-legacy",
					"syslog://other.url-legacy",
					"syslog://zzz-not-included-again.url-legacy",
					"https://other.url-legacy",
					"syslog://other-included.url-legacy",
				},
				Hostname:    "org.space.logspinner-legacy",
				V2Available: true,
			},
		}

	})

	It("returns the max number of v2 bindings by app id", func() {
		fetchedBindings, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		expectedSyslogBindings := []syslog.Binding{
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "https://other.url"},
			},
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://other-included.url"},
			},
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://other.url", Credentials: syslog.Credentials{CA: "ca", Cert: "cert", Key: "key"}},
			},
			{
				AppId:    "testAppID",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "https://other.url"},
			},
			{
				AppId:    "testAppID",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://other-included.url"},
			},
			{
				AppId:    "testAppID",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://other.url"},
			},
			{
				AppId:    "testAppID2",
				Hostname: "org.space.app-name",
				Drain:    syslog.Drain{Url: "syslog://other.url", Credentials: syslog.Credentials{CA: "ca", Cert: "cert", Key: "key"}},
			},
		}
		Expect(fetchedBindings).To(ConsistOf(expectedSyslogBindings))
	})

	It("returns the max number of syslog bindings by app id in case of v2 endpoint failing", func() {
		getter.err = errors.New("getter error occurred")
		getter.legacyBindings[0].V2Available = false
		getter.legacyBindings[1].V2Available = false
		fetcher = bindings.NewBindingFetcher(maxDrains, getter, metrics, logger)
		fetchedBindings, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		expectedSyslogBindings := []syslog.Binding{
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner-legacy",
				Drain:    syslog.Drain{Url: "https://other.url-legacy"},
			},
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner-legacy",
				Drain:    syslog.Drain{Url: "syslog://other-included.url-legacy"},
			},
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner-legacy",
				Drain:    syslog.Drain{Url: "syslog://other.url-legacy"},
			},
			{
				AppId:    "testAppID",
				Hostname: "org.space.logspinner-legacy",
				Drain:    syslog.Drain{Url: "https://other.url-legacy"},
			},
			{
				AppId:    "testAppID",
				Hostname: "org.space.logspinner-legacy",
				Drain:    syslog.Drain{Url: "syslog://other-included.url-legacy"},
			},
			{
				AppId:    "testAppID",
				Hostname: "org.space.logspinner-legacy",
				Drain:    syslog.Drain{Url: "syslog://other.url-legacy"},
			},
		}
		Expect(fetchedBindings).To(ConsistOf(expectedSyslogBindings))

	})

	It("returns an empty array of syslog bindings if v1 endpoint sends no results", func() {
		getter.err = errors.New("getter error occurred")
		getter.legacyBindings = []binding.LegacyBinding{}
		fetcher = bindings.NewBindingFetcher(maxDrains, getter, metrics, logger)
		fetchedBindings, err := fetcher.FetchBindings()
		Expect(err).To(Not(HaveOccurred()))

		var expectedSyslogBindings []syslog.Binding
		Expect(fetchedBindings).To(ConsistOf(expectedSyslogBindings))
	})

	Describe("Binding Type", func() {
		DescribeTable("determines the binding type from the drain url", func(url string, expectedType syslog.BindingType) {
			getter.bindings = []binding.Binding{
				{
					Url:         url,
					Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
				},
			}

			fetcher = bindings.NewBindingFetcher(2, getter, metrics, logger)
			bindings, err := fetcher.FetchBindings()
			Expect(err).ToNot(HaveOccurred())

			Expect(bindings).To(HaveLen(1))
			Expect(bindings[0].Type).To(Equal(expectedType))
		},
			Entry("default", "syslog://something.url", syslog.BINDING_TYPE_LOG),
			Entry("logs", "syslog://something.url?drain-type=logs", syslog.BINDING_TYPE_LOG),
			Entry("metrics", "syslog://something.url?drain-type=metrics", syslog.BINDING_TYPE_METRIC),
			Entry("all", "syslog://something.url?drain-type=all", syslog.BINDING_TYPE_ALL),
		)
	})

	It("tracks the number of binding refreshes", func() {
		_, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(
			metrics.GetMetric("binding_refresh_count", nil).Value(),
		).To(BeNumerically("==", 1))
	})

	It("tracks the max latency of the requests", func() {
		_, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(
			metrics.GetMetric("latency_for_last_binding_refresh", map[string]string{"unit": "ms"}).Value(),
		).To(BeNumerically(">", 0))
	})

	It("returns all the bindings when there are fewer bindings than the limit", func() {
		getter.bindings = []binding.Binding{
			{
				Url:         "syslog://other.url",
				Credentials: []binding.Credentials{{Apps: []binding.App{{Hostname: "org.space.logspinner", AppID: "9be15160-4845-4f05-b089-40e827ba61f1"}}}},
			},
		}
		fetcher = bindings.NewBindingFetcher(2, getter, metrics, logger)
		bindings, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())
		Expect(bindings).To(HaveLen(1))

		Expect(bindings).To(Equal([]syslog.Binding{
			{
				AppId:    "9be15160-4845-4f05-b089-40e827ba61f1",
				Hostname: "org.space.logspinner",
				Drain:    syslog.Drain{Url: "syslog://other.url"},
			},
		}))
	})

	It("returns an error if the Getter returns an error", func() {
		getter.err = errors.New("boom")
		getter.legacyError = errors.New("boom-legacy")

		_, err := fetcher.FetchBindings()
		Expect(err).To(MatchError("boom-legacy"))
	})

	It("returns an error if the Getter returns an error and V2 is available", func() {
		getter.err = errors.New("boom")

		_, err := fetcher.FetchBindings()
		Expect(err).To(MatchError("legacy endpoint is deprecated: skipping result parsing"))
	})
})

type SpyGetter struct {
	bindings       []binding.Binding
	legacyBindings []binding.LegacyBinding
	err            error
	legacyError    error
}

func (s *SpyGetter) Get() ([]binding.Binding, error) {
	time.Sleep(10 * time.Millisecond)
	return s.bindings, s.err
}

func (s *SpyGetter) LegacyGet() ([]binding.LegacyBinding, error) {
	time.Sleep(10 * time.Millisecond)
	return s.legacyBindings, s.legacyError
}
