package bindings_test

import (
	"errors"
	"log"
	"net"
	"net/url"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"

	. "github.com/onsi/ginkgo"
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
			{AppId: "app-id-with-multiple-drains", Hostname: "we.dont.care", Drain: "syslog://10.10.10.10"},
			{AppId: "app-id-with-multiple-drains", Hostname: "we.dont.care", Drain: "syslog://10.10.10.12"},
			{AppId: "app-id-with-good-drain", Hostname: "we.dont.care", Drain: "syslog://10.10.10.10"},
		}
		bindingReader := &SpyBindingReader{bindings: input}

		filter = bindings.NewFilteredBindingFetcher(&spyIPChecker{}, bindingReader, metrics, log)
		actual, err := filter.FetchBindings()

		Expect(err).ToNot(HaveOccurred())
		Expect(actual).To(Equal(input))
	})

	It("returns an error if the binding reader cannot fetch bindings", func() {
		bindingReader := &SpyBindingReader{nil, errors.New("Woops")}

		filter := bindings.NewFilteredBindingFetcher(&spyIPChecker{}, bindingReader, metrics, log)
		actual, err := filter.FetchBindings()

		Expect(err).To(HaveOccurred())
		Expect(actual).To(BeNil())
	})

	Context("when syslog drain has invalid host", func() {
		BeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "syslog://some.invalid.host"},
			}

			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{parseHostError: errors.New("parse error")},
				&SpyBindingReader{bindings: input},
				metrics,
				log,
			)
		})

		It("removes the binding", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})
	})

	Context("when syslog drain has invalid scheme", func() {
		var (
			input []syslog.Binding
		)

		BeforeEach(func() {
			input = []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "syslog://10.10.10.10"},
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "syslog-tls://10.10.10.10"},
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "https://10.10.10.10"},
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "bad-scheme://10.10.10.10"},
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "blah://10.10.10.10"},
			}

			metrics = metricsHelpers.NewMetricsRegistry()
			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{},
				&SpyBindingReader{bindings: input},
				metrics,
				log,
			)
		})

		It("ignores the bindings", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal(input[:3]))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(2.0))
		})
	})

	Context("when the drain host fails to resolve", func() {
		BeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "syslog://some.invalid.host"},
			}

			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{
					resolveAddrError: errors.New("resolve error"),
					parsedHost:       "some.invalid.host",
				},
				&SpyBindingReader{bindings: input},
				metrics,
				log,
			)
		})

		It("removes bindings that failed to resolve", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})
	})

	Context("when the syslog drain has been blacklisted", func() {
		BeforeEach(func() {
			input := []syslog.Binding{
				{AppId: "app-id", Hostname: "we.dont.care", Drain: "syslog://some.invalid.host"},
			}

			filter = bindings.NewFilteredBindingFetcher(
				&spyIPChecker{
					checkBlacklistError: errors.New("blacklist error"),
					parsedHost:          "some.invalid.host",
					resolvedIP:          net.ParseIP("127.0.0.1"),
				},
				&SpyBindingReader{bindings: input},
				metrics,
				log,
			)
		})

		It("removes the binding", func() {
			actual, err := filter.FetchBindings()

			Expect(err).ToNot(HaveOccurred())
			Expect(actual).To(Equal([]syslog.Binding{}))
			Expect(metrics.GetMetric("invalid_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
			Expect(metrics.GetMetric("blacklisted_drains", map[string]string{"unit": "total"}).Value()).To(Equal(1.0))
		})
	})
})

type spyIPChecker struct {
	checkBlacklistError error
	resolveAddrError    error
	resolvedIP          net.IP
	parseHostError      error
	parsedHost          string
}

func (s *spyIPChecker) CheckBlacklist(net.IP) error {
	return s.checkBlacklistError
}

func (s *spyIPChecker) ParseHost(URL string) (string, string, error) {
	u, err := url.Parse(URL)
	if err != nil {
		panic(err)
	}

	return u.Scheme, s.parsedHost, s.parseHostError
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
