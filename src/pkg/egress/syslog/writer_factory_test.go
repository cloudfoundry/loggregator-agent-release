package syslog_test

import (
	"net/url"

	"code.cloudfoundry.org/loggregator-agent/internal/testhelper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent/pkg/egress/syslog"
)

var _ = Describe("EgressFactory", func() {
	var (
		f       syslog.WriterFactory
		sm      *testhelper.SpyMetricClient
		skipSSL = false
	)

	BeforeEach(func() {
		sm = testhelper.NewMetricClient()
		f = syslog.NewWriterFactory(sm, true)
	})

	It("returns an https writer when the url begins with https", func() {
		url, err := url.Parse("https://the-syslog-endpoint.com")
		Expect(err).ToNot(HaveOccurred())
		urlBinding := &syslog.URLBinding{
			URL: url,
		}

		writer, err := f.NewWriter(urlBinding, syslog.NetworkTimeoutConfig{}, skipSSL)
		Expect(err).ToNot(HaveOccurred())

		_, ok := writer.(*syslog.HTTPSWriter)
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

		writer, err := f.NewWriter(urlBinding, syslog.NetworkTimeoutConfig{}, skipSSL)
		Expect(err).ToNot(HaveOccurred())

		_, ok := writer.(*syslog.TCPWriter)
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

		writer, err := f.NewWriter(urlBinding, syslog.NetworkTimeoutConfig{}, skipSSL)
		Expect(err).ToNot(HaveOccurred())

		_, ok := writer.(*syslog.TLSWriter)
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

		_, err = f.NewWriter(urlBinding, syslog.NetworkTimeoutConfig{}, skipSSL)
		Expect(err).To(MatchError("unsupported protocol"))
	})
})
