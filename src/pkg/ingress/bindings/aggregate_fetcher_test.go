package bindings_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Aggregate Drain Binding Fetcher", func() {
	var ()

	BeforeEach(func() {
	})

	It("returns drain bindings for the drain urls", func() {
		bs := []string{
			"syslog://aggregate-drain1.url.com",
			"syslog://aggregate-drain2.url.com?include-metrics-deprecated=true",
		}
		fetcher := bindings.NewAggregateDrainFetcher(bs)

		b, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(b).To(ConsistOf(
			syslog.Binding{
				AppId: "",
				Drain: "syslog://aggregate-drain1.url.com",
				Type:  syslog.BINDING_TYPE_LOG,
			},
			syslog.Binding{
				AppId: "",
				Drain: "syslog://aggregate-drain2.url.com?include-metrics-deprecated=true",
				Type:  syslog.BINDING_TYPE_AGGREGATE,
			},
		))
	})

	It("only returns valid drain bindings for the drain urls", func() {
		bs := []string{
			"syslog://aggregate-drain1.url.com",
			"B@D/aggregate-d\rain1.//l.cm",
		}
		fetcher := bindings.NewAggregateDrainFetcher(bs)

		b, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(b).To(ConsistOf(
			syslog.Binding{
				AppId: "",
				Drain: "syslog://aggregate-drain1.url.com",
				Type:  syslog.BINDING_TYPE_LOG,
			},
		))
	})

	It("handles empty drain bindings", func() {
		bs := []string{""}
		fetcher := bindings.NewAggregateDrainFetcher(bs)

		b, err := fetcher.FetchBindings()
		Expect(err).ToNot(HaveOccurred())

		Expect(len(b)).To(Equal(0))
	})
})
