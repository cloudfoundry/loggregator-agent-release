package syslog_test

import (
	"net/url"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
)

var _ = Describe("EgressFactory", func() {
	var (
		f  syslog.WriterFactory
		sm *metricsHelpers.SpyMetricsRegistry
	)

	BeforeEach(func() {
		sm = metricsHelpers.NewMetricsRegistry()
		f = syslog.NewWriterFactory(nil, syslog.NetworkTimeoutConfig{}, sm)
	})

	It("returns an https writer when the url begins with https", func() {
		url, err := url.Parse("https://the-syslog-endpoint.com")
		Expect(err).ToNot(HaveOccurred())
		urlBinding := &syslog.URLBinding{
			URL: url,
		}

		writer, err := f.NewWriter(urlBinding)
		Expect(err).ToNot(HaveOccurred())

		retryWriter, ok := writer.(*syslog.RetryWriter)
		Expect(ok).To(BeTrue())

		_, ok = retryWriter.Writer.(*syslog.HTTPSWriter)
		Expect(ok).To(BeTrue())

		metric := sm.GetMetric("egress", nil)
		Expect(metric).ToNot(BeNil())
	})

	It("returns a tcp writer when the url begins with syslog://", func() {
		url, err := url.Parse("syslog://the-syslog-endpoint.com")
		Expect(err).ToNot(HaveOccurred())
		urlBinding := &syslog.URLBinding{
			URL: url,
		}

		writer, err := f.NewWriter(urlBinding)
		Expect(err).ToNot(HaveOccurred())

		retryWriter, ok := writer.(*syslog.RetryWriter)
		Expect(ok).To(BeTrue())

		_, ok = retryWriter.Writer.(*syslog.TCPWriter)
		Expect(ok).To(BeTrue())

		metric := sm.GetMetric("egress", nil)
		Expect(metric).ToNot(BeNil())
	})

	It("returns a syslog-tls writer when the url begins with syslog-tls://", func() {
		url, err := url.Parse("syslog-tls://the-syslog-endpoint.com")
		Expect(err).ToNot(HaveOccurred())
		urlBinding := &syslog.URLBinding{
			URL: url,
		}

		writer, err := f.NewWriter(urlBinding)
		Expect(err).ToNot(HaveOccurred())

		retryWriter, ok := writer.(*syslog.RetryWriter)
		Expect(ok).To(BeTrue())

		_, ok = retryWriter.Writer.(*syslog.TLSWriter)
		Expect(ok).To(BeTrue())
		metric := sm.GetMetric("egress", nil)
		Expect(metric).ToNot(BeNil())
	})

	It("returns an error when given a binding with an invalid scheme", func() {
		url, err := url.Parse("invalid://the-syslog-endpoint.com")
		Expect(err).ToNot(HaveOccurred())
		urlBinding := &syslog.URLBinding{
			URL: url,
		}

		_, err = f.NewWriter(urlBinding)
		Expect(err).To(MatchError("unsupported protocol"))
	})
})
