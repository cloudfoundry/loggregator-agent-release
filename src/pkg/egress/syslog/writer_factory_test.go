package syslog_test

import (
	"crypto/tls"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
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
		f = syslog.NewWriterFactory(&tls.Config{}, &tls.Config{}, syslog.NetworkTimeoutConfig{}, sm) //nolint:gosec
	})

	Context("when the url begins with https", func() {
		It("returns an https writer", func() {
			url, err := url.Parse("https://syslog.example.com")
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
		})
	})

	Context("when the url begins with syslog://", func() {
		It("returns a tcp writer", func() {
			url, err := url.Parse("syslog://syslog.example.com")
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
		})
	})

	Context("when the url begins with syslog-tls://", func() {
		It("returns a syslog-tls writer", func() {
			url, err := url.Parse("syslog-tls://syslog.example.com")
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
		})

		Context("when the private key is not passed", func() {
			It("the certificate is ignored", func() {
				url, err := url.Parse("syslog-tls://syslog.example.com")
				Expect(err).ToNot(HaveOccurred())
				urlBinding := &syslog.URLBinding{
					URL:         url,
					Certificate: []byte("invalid-certificate"),
				}

				_, err = f.NewWriter(urlBinding)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when the certificate is not passed", func() {
			It("the private key is ignored", func() {
				url, err := url.Parse("syslog-tls://syslog.example.com")
				Expect(err).ToNot(HaveOccurred())
				urlBinding := &syslog.URLBinding{
					URL:        url,
					PrivateKey: []byte("invalid-private-key"),
				}

				_, err = f.NewWriter(urlBinding)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	DescribeTable("Errors",
		func(u string, certFail bool, caFail bool, expectedErr string) {
			url, err := url.Parse(u)
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			if certFail {
				urlBinding.Certificate = []byte("invalid-cert")
				urlBinding.PrivateKey = []byte("invalid-key")
			}

			if caFail {
				urlBinding.CA = []byte("invalid-ca")
			}

			_, err = f.NewWriter(urlBinding)

			Expect(err).To(MatchError(expectedErr))
		},
		Entry("When the scheme is invalid", "invalid://syslog.example.com", false, false, `"invalid://syslog.example.com": unsupported protocol: "invalid"`),
		Entry("When the scheme is https and the cert/key pair is invalid", "https://username:password@syslog.example.com", true, false, `"https://syslog.example.com": failed to load certificate: tls: failed to find any PEM data in certificate input`),
		Entry("When the scheme is https and the CA is invalid", "https://username:password@syslog.example.com", false, true, `"https://syslog.example.com": failed to load root CA`),
		Entry("When the scheme is syslog-tls and the cert/key pair is invalid", "syslog-tls://username:password@syslog.example.com", true, false, `"syslog-tls://syslog.example.com": failed to load certificate: tls: failed to find any PEM data in certificate input`),
		Entry("When the scheme is syslog-tls and the CA is invalid", "syslog-tls://username:password@syslog.example.com", false, true, `"syslog-tls://syslog.example.com": failed to load root CA`),
	)

	DescribeTable("Metrics",
		func(u string, aggregate bool) {
			url, err := url.Parse(u)
			Expect(err).ToNot(HaveOccurred())
			appID := "app-id"
			tags := map[string]string{"drain_scope": "app", "drain_url": u}
			if aggregate {
				appID = ""
				tags["drain_scope"] = "aggregate"
			}
			urlBinding := &syslog.URLBinding{
				URL:   url,
				AppID: appID,
			}

			_, err = f.NewWriter(urlBinding)
			Expect(err).ToNot(HaveOccurred())

			metric := sm.GetMetric("egress", tags)
			Expect(metric).ToNot(BeNil())
		},
		Entry("For https aggregate drain", "https://syslog.example.com", true),
		Entry("For https app drain", "https://syslog.example.com", false),
		Entry("For syslog aggregate drain", "syslog://syslog.example.com", true),
		Entry("For syslog app drain", "syslog://syslog.example.com", false),
		Entry("For syslog-tls aggregate drain", "syslog-tls://syslog.example.com", true),
		Entry("For syslog-tls app drain", "syslog-tls://syslog.example.com", false),
	)
})
