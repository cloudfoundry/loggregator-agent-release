package bindings_test

import (
	"net"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BlacklistRanges", func() {
	Describe("validates", func() {
		It("accepts valid IP address range", func() {
			_, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "127.0.2.2", End: "127.0.2.4"},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error with an invalid start address", func() {
			_, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "127.0.2.2.1", End: "127.0.2.4"},
			)
			Expect(err).To(MatchError("invalid IP Address for Blacklist IP Range: 127.0.2.2.1"))
		})

		It("returns an error with an invalid end address", func() {
			_, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "127.0.2.2", End: "127.0.2.4.3"},
			)
			Expect(err).To(HaveOccurred())
		})

		It("validates multiple blacklist ranges", func() {
			_, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "127.0.2.2", End: "127.0.2.4"},
				bindings.BlacklistRange{Start: "127.0.2.2", End: "127.0.2.4.5"},
			)
			Expect(err).To(HaveOccurred())
		})

		It("validates start IP is before end IP", func() {
			_, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "10.10.10.10", End: "10.8.10.12"},
			)
			Expect(err).To(MatchError("invalid Blacklist IP Range: Start 10.10.10.10 has to be before End 10.8.10.12"))
		})

		It("accepts start and end as the same", func() {
			_, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "127.0.2.2", End: "127.0.2.2"},
			)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("CheckBlacklist()", func() {
		It("allows all urls for empty blacklist range", func() {
			ranges, _ := bindings.NewBlacklistRanges()

			err := ranges.CheckBlacklist(net.ParseIP("127.0.0.1"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error when the IP is in the blacklist range", func() {
			ranges, err := bindings.NewBlacklistRanges(
				bindings.BlacklistRange{Start: "127.0.1.2", End: "127.0.3.4"},
			)
			Expect(err).ToNot(HaveOccurred())

			err = ranges.CheckBlacklist(net.ParseIP("127.0.2.2"))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ResolveAddr()", func() {
		It("does not return an error when able to resolve", func() {
			ranges, _ := bindings.NewBlacklistRanges()

			ip, err := ranges.ResolveAddr("localhost")
			Expect(err).ToNot(HaveOccurred())
			Expect(ip.String()).To(Equal("127.0.0.1"))
		})

		It("can resolve addresses with port", func() {
			ranges, _ := bindings.NewBlacklistRanges()

			ip, err := ranges.ResolveAddr("localhost:8080")
			Expect(err).ToNot(HaveOccurred())
			Expect(ip.String()).To(Equal("127.0.0.1"))
		})

		It("returns an error when it fails to resolve", func() {
			ranges, _ := bindings.NewBlacklistRanges()

			_, err := ranges.ResolveAddr("vcap.me.junky-garbage")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UnmarshalEnv", func() {
		It("returns an error for non-valid input", func() {
			bl := &bindings.BlacklistRanges{}
			Expect(bl.UnmarshalEnv("invalid")).ToNot(Succeed())

			Expect(bl.UnmarshalEnv("10.244.0.32-10")).ToNot(Succeed())
		})

		It("parses the given IP ranges", func() {
			bl := &bindings.BlacklistRanges{}
			Expect(bl.UnmarshalEnv("10.0.0.4-10.0.0.8,123.4.5.6-123.4.5.7")).To(Succeed())

			Expect(bl.Ranges).To(Equal([]bindings.BlacklistRange{
				{Start: "10.0.0.4", End: "10.0.0.8"},
				{Start: "123.4.5.6", End: "123.4.5.7"},
			}))
		})

		It("does not return an error for an empty list", func() {
			bl := &bindings.BlacklistRanges{}
			Expect(bl.UnmarshalEnv("")).To(Succeed())
		})
	})
})
