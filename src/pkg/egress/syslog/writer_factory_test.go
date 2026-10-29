package syslog_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/bindingfakes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EgressFactory", func() {
	var (
		f         syslog.WriterFactory
		sm        *metricsHelpers.SpyMetricsRegistry
		logClient v2.LogClient
	)

	BeforeEach(func() {
		sm = metricsHelpers.NewMetricsRegistry()
		f = syslog.NewWriterFactory(&tls.Config{}, &tls.Config{}, syslog.NetworkTimeoutConfig{}, sm, nil) //nolint:gosec
	})

	Context("when the url begins with https", func() {
		It("returns an single https writer", func() {
			url, err := url.Parse("https://syslog.example.com")
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			writer, err := f.NewWriter(urlBinding, logClient)
			Expect(err).ToNot(HaveOccurred())

			retryWriter, ok := writer.(*syslog.RetryWriter)
			Expect(ok).To(BeTrue())

			_, ok = retryWriter.Writer.(*syslog.HTTPSWriter)
			Expect(ok).To(BeTrue())
		})
	})

	Context("when the url begins with https and enables batching", func() {
		It("returns an single https batch writer", func() {
			url, err := url.Parse("https-batch://syslog.example.com")
			Expect(err).ToNot(HaveOccurred())
			urlBinding := &syslog.URLBinding{
				URL: url,
			}

			writer, err := f.NewWriter(urlBinding, logClient)
			Expect(err).ToNot(HaveOccurred())

			_, ok := writer.(*syslog.HTTPSBatchWriter)
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

			writer, err := f.NewWriter(urlBinding, logClient)
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

			writer, err := f.NewWriter(urlBinding, logClient)
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

				_, err = f.NewWriter(urlBinding, logClient)
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

				_, err = f.NewWriter(urlBinding, logClient)
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

			_, err = f.NewWriter(urlBinding, logClient)

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
			tags := map[string]string{"direction": "egress", "drain_scope": "app", "drain_url": u}
			if aggregate {
				appID = ""
				tags["drain_scope"] = "aggregate"
			}
			urlBinding := &syslog.URLBinding{
				URL:   url,
				AppID: appID,
			}

			_, err = f.NewWriter(urlBinding, logClient)
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

	Describe("IP blacklist enforcement", func() {
		var (
			fakeChecker *bindingfakes.FakeIPChecker
			testEnv     *loggregator_v2.Envelope
			testTLSConf *tls.Config
			testNetConf syslog.NetworkTimeoutConfig
			certFile    string
			keyFile     string
		)

		BeforeEach(func() {
			fakeChecker = &bindingfakes.FakeIPChecker{}
			fakeChecker.CheckBlacklistReturns(errors.New("blacklisted"))
			testEnv = buildLogEnvelope("APP", "1", "test payload", loggregator_v2.Log_OUT)
			testTLSConf = &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			}
			testNetConf = syslog.NetworkTimeoutConfig{
				DialTimeout:  100 * time.Millisecond,
				WriteTimeout: 100 * time.Millisecond,
			}
			testCerts := testhelper.GenerateCerts("loggregatorCA")
			certFile = testCerts.Cert("metron")
			keyFile = testCerts.Key("metron")
		})

		DescribeTable("blocks user drain (AppID set) with ErrBlockedAddress for each scheme",
			func(scheme string) {
				var drainURL *url.URL
				var cleanup func()
				switch scheme {
				case "syslog":
					l, err := net.Listen("tcp", "127.0.0.1:0")
					Expect(err).ToNot(HaveOccurred())
					cleanup = func() { _ = l.Close() }
					drainURL, _ = url.Parse(fmt.Sprintf("syslog://%s", l.Addr()))
				case "syslog-tls":
					tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
					Expect(err).ToNot(HaveOccurred())
					l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{tlsCert}})
					Expect(err).ToNot(HaveOccurred())
					cleanup = func() { _ = l.Close() }
					drainURL, _ = url.Parse(fmt.Sprintf("syslog-tls://%s", l.Addr()))
				case "https":
					drain := newMockDrain(http.StatusOK)
					cleanup = drain.Close
					drainURL, _ = url.Parse(drain.URL)
				case "https-batch":
					drain := newBatchMockDrain(http.StatusOK)
					cleanup = drain.Close
					drainURL, _ = url.Parse(fmt.Sprintf("https-batch://%s", drain.Listener.Addr()))
				}
				defer cleanup()

				factory := syslog.NewWriterFactory(
					testTLSConf,
					testTLSConf,
					testNetConf,
					sm,
					fakeChecker,
				)

				binding := &syslog.URLBinding{
					AppID:   "test-app-id",
					URL:     drainURL,
					Context: context.Background(),
				}

				writer, err := factory.NewWriter(binding, logClient)
				Expect(err).ToNot(HaveOccurred())
				defer writer.Close()

				err = writeDrain(writer, testEnv)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())
			},
			Entry("for syslog scheme", "syslog"),
			Entry("for syslog-tls scheme", "syslog-tls"),
			Entry("for https scheme", "https"),
			Entry("for https-batch scheme", "https-batch"),
		)

		It("allows aggregate drain (AppID == \"\") to connect to the same listener that blocks user drain", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			accepted := make(chan net.Conn, 1)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					accepted <- conn
				}
			}()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog://%s", listener.Addr()))

			factory := syslog.NewWriterFactory(
				testTLSConf,
				testTLSConf,
				testNetConf,
				sm,
				fakeChecker,
			)

			// User drain is blocked
			userBinding := &syslog.URLBinding{
				AppID:   "test-app-id",
				URL:     drainURL,
				Context: context.Background(),
			}
			userWriter, err := factory.NewWriter(userBinding, logClient)
			Expect(err).ToNot(HaveOccurred())
			defer userWriter.Close()

			err = writeDrain(userWriter, testEnv)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())

			// Aggregate drain to same listener connects successfully without invoking IPChecker
			initialCallCount := fakeChecker.CheckBlacklistCallCount()

			aggBinding := &syslog.URLBinding{
				AppID:   "",
				URL:     drainURL,
				Context: context.Background(),
			}
			aggWriter, err := factory.NewWriter(aggBinding, logClient)
			Expect(err).ToNot(HaveOccurred())
			defer aggWriter.Close()

			err = writeDrain(aggWriter, testEnv)
			Expect(err).ToNot(HaveOccurred())

			var conn net.Conn
			Eventually(accepted).Should(Receive(&conn))
			defer conn.Close()

			Expect(fakeChecker.CheckBlacklistCallCount()).To(Equal(initialCallCount))
		})

		It("blocks user drain even when InternalTls=true", func() {
			tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
			Expect(err).ToNot(HaveOccurred())
			serverTLS := &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
			}
			listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog-tls://%s", listener.Addr()))
			binding := &syslog.URLBinding{
				AppID:       "test-app-id",
				InternalTls: true,
				URL:         drainURL,
				Context:     context.Background(),
			}

			factory := syslog.NewWriterFactory(
				testTLSConf,
				testTLSConf,
				testNetConf,
				sm,
				fakeChecker,
			)

			writer, err := factory.NewWriter(binding, logClient)
			Expect(err).ToNot(HaveOccurred())
			defer writer.Close()

			err = writeDrain(writer, testEnv)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())
		})

		It("connects successfully when ipChecker is nil (no-op)", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			accepted := make(chan net.Conn, 1)
			go func() {
				conn, err := listener.Accept()
				if err == nil {
					accepted <- conn
				}
			}()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog://%s", listener.Addr()))
			binding := &syslog.URLBinding{
				AppID:   "test-app-id",
				URL:     drainURL,
				Context: context.Background(),
			}

			factory := syslog.NewWriterFactory(
				testTLSConf,
				testTLSConf,
				testNetConf,
				sm,
				nil,
			)

			writer, err := factory.NewWriter(binding, logClient)
			Expect(err).ToNot(HaveOccurred())
			defer writer.Close()

			err = writeDrain(writer, testEnv)
			Expect(err).ToNot(HaveOccurred())

			var conn net.Conn
			Eventually(accepted).Should(Receive(&conn))
			defer conn.Close()
		})

		It("retries a blocked dial through RetryWriter making at least 2 attempts and returning error without panic", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			defer listener.Close()

			drainURL, _ := url.Parse(fmt.Sprintf("syslog://%s", listener.Addr()))
			ctx, cancel := context.WithCancel(context.Background())
			binding := &syslog.URLBinding{
				AppID:   "test-app-id",
				URL:     drainURL,
				Context: ctx,
			}

			retryChecker := &bindingfakes.FakeIPChecker{}
			retryChecker.CheckBlacklistStub = func(ip net.IP) error {
				if retryChecker.CheckBlacklistCallCount() >= 2 {
					cancel()
				}
				return errors.New("blacklisted")
			}

			factory := syslog.NewWriterFactory(
				testTLSConf,
				testTLSConf,
				testNetConf,
				sm,
				retryChecker,
			)

			writer, err := factory.NewWriter(binding, logClient)
			Expect(err).ToNot(HaveOccurred())
			defer writer.Close()

			// First write: retries and returns blocked address error after >= 2 attempts
			err = writer.Write(testEnv)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())
			Expect(retryChecker.CheckBlacklistCallCount()).To(BeNumerically(">=", 2))

			// Second write: returns error without panic
			Expect(func() {
				err = writer.Write(testEnv)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())
			}).ToNot(Panic())
		})
	})
})

func writeDrain(w egress.WriteCloser, env *loggregator_v2.Envelope) error {
	if rw, ok := w.(*syslog.RetryWriter); ok {
		return rw.Writer.Write(env)
	}
	if bw, ok := w.(*syslog.HTTPSBatchWriter); ok {
		return bw.HTTPSWriter.Write(env)
	}
	return w.Write(env)
}
