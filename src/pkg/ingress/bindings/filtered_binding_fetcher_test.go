package bindings_test

import (
	"bytes"
	"errors"
	"log"
	"net"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings/bindingsfakes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FilteredBindingFetcher", func() {
	var (
		log     = log.New(GinkgoWriter, "", log.LstdFlags)
		filter  *bindings.FilteredBindingFetcher
		metrics *metricsHelpers.SpyMetricsRegistry
	)

	BeforeEach(func() {
		metrics = metricsHelpers.NewMetricsRegistry()
	})

	It("returns valid bindings", func() {
		input := []syslog.Binding{
			{AppId: "app-id-with-multiple-drains", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "syslog://10.10.10.10"}},
			{AppId: "app-id-with-multiple-drains", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "syslog://10.10.10.12"}},
			{AppId: "app-id-with-good-drain", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "syslog://10.10.10.10"}},
		}
		bindingReader := &SpyBindingReader{bindings: input}

		filter = bindings.NewFilteredBindingFetcher(&spyIPChecker{}, bindingReader, metrics, true, log)
		actual, err := filter.FetchBindings()

		Expect(err).ToNot(HaveOccurred())
		Expect(actual).To(Equal(input))
	})

	It("returns an error if the binding reader cannot fetch bindings", func() {
		bindingReader := &SpyBindingReader{nil, errors.New("Woops")}

		filter := bindings.NewFilteredBindingFetcher(&spyIPChecker{}, bindingReader, metrics, true, log)
		actual, err := filter.FetchBindings()

		Expect(err).To(HaveOccurred())
		Expect(actual).To(BeNil())
	})

	Context("when syslog drain is unparsable", func() {
		var logBuffer bytes.Buffer
		var warn bool

		BeforeEach(func() {
			logBuffer = bytes.Buffer{}
			log.SetOutput(&logBuffer)
			warn = true
		})

		JustBeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "://"}},
			}
			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{},
				&SpyBindingReader{bindings: input},
				metrics,
				warn,
				log,
			)
		})

		It("removes the binding", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(logBuffer.String()).Should(MatchRegexp("Cannot parse syslog drain url for application"))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})

		Context("when configured not to warn", func() {
			BeforeEach(func() {
				warn = false
			})
			It("doesn't log the warning", func() {
				_, err := filter.FetchBindings()
				Expect(err).ToNot(HaveOccurred())
				Expect(logBuffer.String()).ToNot(MatchRegexp("Cannot parse syslog drain url for application"))
			})
		})
	})

	Context("when drain has no host", func() {
		var logBuffer bytes.Buffer
		var warn bool

		BeforeEach(func() {
			logBuffer = bytes.Buffer{}
			log.SetOutput(&logBuffer)
			warn = true
		})

		JustBeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "https:///path"}},
			}
			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{},
				&SpyBindingReader{bindings: input},
				metrics,
				warn,
				log,
			)
		})

		It("removes the binding", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(logBuffer.String()).Should(MatchRegexp("No hostname found in syslog drain url (.*) for application"))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})

		Context("when configured not to warn", func() {
			BeforeEach(func() {
				warn = false
			})
			It("doesn't log the warning", func() {
				_, err := filter.FetchBindings()
				Expect(err).ToNot(HaveOccurred())
				Expect(logBuffer.String()).ToNot(MatchRegexp("No hostname found"))
			})
		})
	})

	Context("when syslog drain has unsupported scheme", func() {
		var (
			input     []syslog.Binding
			logBuffer bytes.Buffer
			warn      bool
		)

		BeforeEach(func() {
			input = []syslog.Binding{
				{AppId: "app-id", Hostname: "known", Drain: syslog.Drain{Url: "syslog://10.10.10.10"}},
				{AppId: "app-id", Hostname: "known", Drain: syslog.Drain{Url: "syslog-tls://10.10.10.10"}},
				{AppId: "app-id", Hostname: "known", Drain: syslog.Drain{Url: "https://10.10.10.10"}},
				{AppId: "app-id", Hostname: "unknown", Drain: syslog.Drain{Url: "bad-scheme://10.10.10.10"}},
				{AppId: "app-id", Hostname: "unknown", Drain: syslog.Drain{Url: "bad-scheme:///path"}},
				{AppId: "app-id", Hostname: "unknown", Drain: syslog.Drain{Url: "blah://10.10.10.10"}},
			}

			logBuffer = bytes.Buffer{}
			log.SetOutput(&logBuffer)
			warn = true

			metrics = metricsHelpers.NewMetricsRegistry()
		})

		JustBeforeEach(func() {
			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{},
				&SpyBindingReader{bindings: input},
				metrics,
				warn,
				log,
			)
		})

		It("ignores the bindings", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(input[:3]))
			Expect(logBuffer.String()).Should(MatchRegexp("Invalid scheme (.*) in syslog drain url (.*) for application"))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(0.0))
		})
		Context("when configured not to warn", func() {
			BeforeEach(func() {
				warn = false
			})
			It("doesn't log the warning", func() {
				_, err := filter.FetchBindings()
				Expect(err).ToNot(HaveOccurred())
				Expect(logBuffer.String()).ToNot(MatchRegexp("Invalid scheme"))
			})
		})
	})

	Context("when the drain host fails to resolve", func() {
		var logBuffer bytes.Buffer
		var warn bool
		var mockic *bindingsfakes.FakeIPChecker

		BeforeEach(func() {
			logBuffer = bytes.Buffer{}
			log.SetOutput(&logBuffer)
			warn = true
			mockic = &bindingsfakes.FakeIPChecker{}
			mockic.ResolveAddrReturns(net.IP{}, errors.New("oof ouch ip not resolved"))
		})

		JustBeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "syslog://some.invalid.host"}},
			}
			filter = bindings.NewFilteredBindingFetcher(
				mockic,
				&SpyBindingReader{bindings: input},
				metrics,
				warn,
				log,
			)
		})

		It("removes bindings that failed to resolve", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(logBuffer.String()).Should(MatchRegexp("Cannot resolve ip address for syslog drain with url"))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})

		It("caches bindings that failed to resolve", func() {
			actual, err := filter.FetchBindings()
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Eventually(func() int { return mockic.ResolveAddrCallCount() }).Should(BeNumerically(">", 0))
			Expect(logBuffer.String()).Should(MatchRegexp("Cannot resolve ip address for syslog drain with url"))
			Expect(logBuffer.String()).ToNot(MatchRegexp("Skipped resolve ip address for syslog drain with url"))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))

			actual, err = filter.FetchBindings()
			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(BeEmpty())
			Consistently(func() int { return mockic.ResolveAddrCallCount() }).Should(Equal(1))
			Expect(logBuffer.String()).Should(MatchRegexp("Skipped resolve ip address for syslog drain with url"))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})

		Context("when configured not to warn", func() {
			BeforeEach(func() {
				warn = false
			})
			It("doesn't log the warning", func() {
				_, err := filter.FetchBindings()
				Expect(err).ToNot(HaveOccurred())
				Expect(logBuffer.String()).ToNot(MatchRegexp("Cannot resolve ip address for syslog drain with url"))
			})
		})
	})

	Context("when the syslog drain has been blacklisted", func() {
		var logBuffer bytes.Buffer
		var warn bool

		BeforeEach(func() {
			logBuffer = bytes.Buffer{}
			log.SetOutput(&logBuffer)
			warn = true
		})

		JustBeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: syslog.Drain{Url: "syslog://some.invalid.host"}},
			}

			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{
					checkBlacklistError: errors.New("blacklist error"),
					resolvedIP:          net.ParseIP("127.0.0.1"),
				},
				&SpyBindingReader{bindings: input},
				metrics,
				warn,
				log,
			)
		})

		It("removes the binding", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(logBuffer.String()).Should(MatchRegexp("Resolved ip address for syslog drain with url (.*) for application (.*) is blacklisted"))
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
			Expect(metrics.GetMetric("blacklisted_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})

		Context("when configured not to warn", func() {
			BeforeEach(func() {
				warn = false
			})
			It("doesn't log the warning", func() {
				_, err := filter.FetchBindings()

				Expect(err).ToNot(HaveOccurred())
				Expect(logBuffer.String()).ToNot(MatchRegexp("Resolved ip address for syslog drain with url (.*) for application (.*) is blacklisted"))
			})
		})
	})
})

type spyIPChecker struct {
	checkBlacklistError error
	resolveAddrError    error
	resolvedIP          net.IP
}

func (s *spyIPChecker) CheckBlacklist(net.IP) error {
	return s.checkBlacklistError
}

func (s *spyIPChecker) ResolveAddr(host string) (net.IP, error) {
	return s.resolvedIP, s.resolveAddrError
}

type SpyBindingReader struct {
	bindings []syslog.Binding
	err      error
}

func (s *SpyBindingReader) FetchBindings() ([]syslog.Binding, error) {
	return s.bindings, s.err
}

func (s *SpyBindingReader) DrainLimit() int {
	return 0
}
