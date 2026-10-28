package syslog_test

import (
	"context"
	"errors"
	"net"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/bindingfakes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CheckedDialer", func() {
	var (
		fakeChecker *bindingfakes.FakeIPChecker
		netConf     syslog.NetworkTimeoutConfig
	)

	BeforeEach(func() {
		fakeChecker = &bindingfakes.FakeIPChecker{}
		netConf = syslog.NetworkTimeoutConfig{
			DialTimeout: 500 * time.Millisecond,
			Keepalive:   10 * time.Second,
			IPChecker:   fakeChecker,
		}
	})

	It("returns a dialer with Timeout and Keepalive configured", func() {
		dialer := syslog.CheckedDialer(netConf)
		Expect(dialer.Timeout).To(Equal(500 * time.Millisecond))
		Expect(dialer.KeepAlive).To(Equal(10 * time.Second))
	})

	It("does not configure Control if IPChecker is nil", func() {
		netConf.IPChecker = nil
		dialer := syslog.CheckedDialer(netConf)
		Expect(dialer.Control).To(BeNil())
	})

	Describe("address parsing and blacklist checks in Control", func() {
		DescribeTable("blocks addresses with ErrBlockedAddress when checker blocks and allows when checker allows",
			func(address string) {
				dialer := syslog.CheckedDialer(netConf)
				Expect(dialer.Control).ToNot(BeNil())

				// When checker blocks
				fakeChecker.CheckBlacklistReturns(errors.New("blacklisted"))
				err := dialer.Control("tcp", address, nil)
				Expect(err).To(MatchError(syslog.ErrBlockedAddress))
				Expect(fakeChecker.CheckBlacklistCallCount()).To(Equal(1))
				checkedIP := fakeChecker.CheckBlacklistArgsForCall(0)
				Expect(len(checkedIP)).To(Equal(16))

				// When checker allows
				fakeChecker.CheckBlacklistReturns(nil)
				err = dialer.Control("tcp", address, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeChecker.CheckBlacklistCallCount()).To(Equal(2))
				checkedIP = fakeChecker.CheckBlacklistArgsForCall(1)
				Expect(len(checkedIP)).To(Equal(16))
			},
			Entry("IPv4 literal", "192.0.2.1:514"),
			Entry("IPv6 literal", "[2001:db8::1]:514"),
			Entry("IPv4-mapped IPv6", "[::ffff:192.0.2.1]:514"),
			Entry("zoned IPv6", "[fe80::1%eth0]:514"),
		)

		DescribeTable("refuses unparsable addresses with ErrBlockedAddress without calling checker",
			func(address string) {
				dialer := syslog.CheckedDialer(netConf)
				Expect(dialer.Control).ToNot(BeNil())

				err := dialer.Control("tcp", address, nil)
				Expect(err).To(MatchError(syslog.ErrBlockedAddress))
				Expect(fakeChecker.CheckBlacklistCallCount()).To(Equal(0))
			},
			Entry("not an address", "not-an-address"),
			Entry("IPv4 without port", "10.0.0.1"),
			Entry("host name without port", "example.com"),
			Entry("unparsable host with port", "not-an-ip:514"),
		)
	})

	Describe("multi-record fallback", func() {
		It("falls back to IPv4 when IPv6 is blocked and connects successfully", func() {
			addrs, err := net.LookupIP("localhost")
			Expect(err).ToNot(HaveOccurred())

			var hasV4, hasV6 bool
			for _, ip := range addrs {
				if ip.To4() != nil {
					hasV4 = true
				} else {
					hasV6 = true
				}
			}

			if !hasV4 || !hasV6 {
				Skip("localhost does not resolve to both IPv4 and IPv6")
			}

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(listener.Close)

			fakeChecker.CheckBlacklistStub = func(ip net.IP) error {
				if ip.Equal(net.IPv6loopback) {
					return errors.New("blocked ::1")
				}
				return nil
			}

			dialer := syslog.CheckedDialer(netConf)
			_, port, err := net.SplitHostPort(listener.Addr().String())
			Expect(err).ToNot(HaveOccurred())

			conn, err := dialer.DialContext(context.Background(), "tcp", net.JoinHostPort("localhost", port))
			Expect(err).ToNot(HaveOccurred())
			defer conn.Close()

			serverConn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			defer serverConn.Close()
		})
	})

	Describe("dial-time enforcement", func() {
		It("fails DialContext to a local listener with an error matching ErrBlockedAddress when dialing by hostname", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(listener.Close)

			fakeChecker.CheckBlacklistReturns(errors.New("blacklisted"))

			_, port, err := net.SplitHostPort(listener.Addr().String())
			Expect(err).ToNot(HaveOccurred())

			dialer := syslog.CheckedDialer(netConf)
			_, err = dialer.DialContext(context.Background(), "tcp", net.JoinHostPort("localhost", port))
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())
		})
	})
})
