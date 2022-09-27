package bindings_test

import (
	"errors"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/bindings"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Aggregate Drain Binding Fetcher", func() {
	var ()

	BeforeEach(func() {
	})

	Context("cache fetcher is nil", func() {
		It("returns drain bindings for the drain urls", func() {
			bs := []string{
				"syslog://aggregate-drain1.url.com",
				"syslog://aggregate-drain2.url.com?include-metrics-deprecated=true",
			}
			fetcher := bindings.NewAggregateDrainFetcher(bs, nil)

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
			fetcher := bindings.NewAggregateDrainFetcher(bs, nil)

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
			fetcher := bindings.NewAggregateDrainFetcher(bs, nil)

			b, err := fetcher.FetchBindings()
			Expect(err).ToNot(HaveOccurred())

			Expect(len(b)).To(Equal(0))
		})
	})
	Context("cache fetcher exists", func() {
		It("ignores fetcher if both are available", func() {
			bs := []string{
				"syslog://aggregate-drain1.url.com",
				"syslog://aggregate-drain2.url.com?include-metrics-deprecated=true",
			}
			cacheFetcher := mockCacheFetcher{bindings: []binding.Binding{{Drains: []string{"syslog://drain.url.com"}}}}
			fetcher := bindings.NewAggregateDrainFetcher(bs, &cacheFetcher)

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
		It("returns results from cache if no drains", func() {
			bs := []string{""}
			cacheFetcher := mockCacheFetcher{bindings: []binding.Binding{{Drains: []string{
				"syslog://aggregate-drain1.url.com",
				"syslog://aggregate-drain2.url.com?include-metrics-deprecated=true",
				"B@D/aggregate-d\rain1.//l.cm",
			}}}}
			fetcher := bindings.NewAggregateDrainFetcher(bs, &cacheFetcher)

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
		It("returns error if fetching fails", func() {
			bs := []string{""}
			cacheFetcher := mockCacheFetcher{err: errors.New("error")}
			fetcher := bindings.NewAggregateDrainFetcher(bs, &cacheFetcher)

			_, err := fetcher.FetchBindings()
			Expect(err).To(HaveOccurred())
		})
	})
})

type mockCacheFetcher struct {
	bindings []binding.Binding
	err      error
}

func (m *mockCacheFetcher) GetAggregate() ([]binding.Binding, error) {
	return m.bindings, m.err
}
