package syslog_test

import (
	"context"
	"errors"
	"net"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/bindingfakes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding/blacklist"
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

	Describe("with a real BlacklistRanges", func() {
		var ranges *blacklist.BlacklistRanges

		BeforeEach(func() {
			var err error
			ranges, err = blacklist.NewBlacklistRanges(
				blacklist.BlacklistRange{Start: "10.0.0.0", End: "10.255.255.255"},
				blacklist.BlacklistRange{Start: "127.0.0.1", End: "127.0.0.1"},
				// "localhost" can also resolve to ::1 (it does on CI runners)
				blacklist.BlacklistRange{Start: "::1", End: "::1"},
				blacklist.BlacklistRange{Start: "169.254.169.254", End: "169.254.169.254"},
				blacklist.BlacklistRange{Start: "fd00::", End: "fd00::ffff"},
			)
			Expect(err).ToNot(HaveOccurred())
			netConf.IPChecker = ranges
		})

		DescribeTable("Control enforces the ranges on the 16-byte form of the address",
			func(address string, blocked bool) {
				err := syslog.CheckedDialer(netConf).Control("tcp", address, nil)
				if blocked {
					Expect(err).To(MatchError(syslog.ErrBlockedAddress))
				} else {
					Expect(err).ToNot(HaveOccurred())
				}
			},
			Entry("IPv4 inside a range", "10.1.2.3:514", true),
			Entry("IPv4 range start", "10.0.0.0:514", true),
			Entry("IPv4 range end", "10.255.255.255:514", true),
			Entry("IPv4 just below a range", "9.255.255.255:514", false),
			Entry("IPv4 just above a range", "11.0.0.0:514", false),
			Entry("single-address range (metadata service)", "169.254.169.254:80", true),
			Entry("neighbour of a single-address range", "169.254.169.253:80", false),
			Entry("public IPv4", "203.0.113.7:514", false),
			Entry("IPv4-mapped IPv6 of a blocked IPv4", "[::ffff:10.1.2.3]:514", true),
			Entry("IPv4-mapped IPv6 of an allowed IPv4", "[::ffff:203.0.113.7]:514", false),
			Entry("IPv6 inside a range", "[fd00::1]:514", true),
			Entry("IPv6 outside a range", "[fd01::1]:514", false),
			Entry("zoned IPv6 inside a range", "[fd00::1%eth0]:514", true),
		)

		It("refuses a dial to a blocked loopback listener and allows one outside the ranges", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(listener.Close)

			_, err = syslog.CheckedDialer(netConf).Dial("tcp", listener.Addr().String())
			Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue())

			allowed, err := blacklist.NewBlacklistRanges(blacklist.BlacklistRange{Start: "10.0.0.0", End: "10.255.255.255"})
			Expect(err).ToNot(HaveOccurred())
			netConf.IPChecker = allowed
			conn, err := syslog.CheckedDialer(netConf).Dial("tcp", listener.Addr().String())
			Expect(err).ToNot(HaveOccurred())
			_ = conn.Close()
		})

		It("makes the fasthttp dialer refuse a blocked loopback hostname and IP literal", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(listener.Close)

			for _, host := range []string{"localhost", "127.0.0.1"} {
				_, err = syslog.CheckedTCPDialer(netConf).Dial(net.JoinHostPort(host, portOf(listener)))
				Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue(), host)
			}
		})
	})

	Describe("checkedTCPDialer (fasthttp)", func() {
		It("refuses to dial when every resolved address is blacklisted", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(listener.Close)
			fakeChecker.CheckBlacklistReturns(errors.New("blacklisted"))

			for _, host := range []string{"localhost", "127.0.0.1"} {
				_, err = syslog.CheckedTCPDialer(netConf).Dial(net.JoinHostPort(host, portOf(listener)))
				Expect(errors.Is(err, syslog.ErrBlockedAddress)).To(BeTrue(), host)
			}
		})

		It("dials only allowed addresses and caches the lookup", func() {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(listener.Close)

			dialer := syslog.CheckedTCPDialer(netConf)
			for range 3 {
				conn, err := dialer.Dial(net.JoinHostPort("localhost", portOf(listener)))
				Expect(err).ToNot(HaveOccurred())
				_ = conn.Close()
			}
			// one blacklist pass over the resolved addresses, not one per dial
			Expect(fakeChecker.CheckBlacklistCallCount()).To(BeNumerically("<=", 2))
		})
	})
})

func portOf(l net.Listener) string {
	_, port, err := net.SplitHostPort(l.Addr().String())
	Expect(err).ToNot(HaveOccurred())
	return port
}
