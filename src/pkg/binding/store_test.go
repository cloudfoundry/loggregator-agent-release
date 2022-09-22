package binding_test

import (
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Store", func() {
	It("should store and retrieve bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
		bindings := []binding.Binding{
			{
				AppID:    "app-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		store.Set(bindings)
		Expect(store.Get()).To(Equal(bindings))
	})

	It("should not return nil bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
		Expect(store.Get()).ToNot(BeNil())
	})

	It("should not allow setting of bindings to nil", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())

		bindings := []binding.Binding{
			{
				AppID:    "app-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		store.Set(bindings)
		store.Set(nil)

		storedBindings := store.Get()
		Expect(storedBindings).ToNot(BeNil())
		Expect(storedBindings).To(BeEmpty())
	})

	// The race detector will cause a failure here
	// if the store is not thread safe
	It("should be thread safe", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())

		go func() {
			for i := 0; i < 1000; i++ {
				store.Set([]binding.Binding{})
			}
		}()

		for i := 0; i < 1000; i++ {
			_ = store.Get()
		}
	})

	It("tracks the number of bindings", func() {
		metrics := metricsHelpers.NewMetricsRegistry()
		store := binding.NewStore(metrics)
		bindings := []binding.Binding{
			{
				AppID:    "app-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		store.Set(bindings)

		Expect(metrics.GetMetric("cached_bindings", nil).Value()).
			To(BeNumerically("==", 1))
	})
})
