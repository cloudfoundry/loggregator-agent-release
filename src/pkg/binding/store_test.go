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
				Url: "drain-1",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "host-1", AppID: "app-1"}},
					},
				},
			},
		}

		store.Set(bindings, 1)
		Expect(store.Get()).To(Equal(bindings))

	})

	It("should store and retrieve legacy bindings", func() {
		legacyStore := binding.NewLegacyStore()
		bindings := []binding.LegacyBinding{
			{
				AppID:    "app-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		legacyStore.Set(bindings)
		Expect(legacyStore.Get()).To(Equal(bindings))

	})

	It("should not return nil bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
		Expect(store.Get()).ToNot(BeNil())
	})

	It("should not return nil legacy bindings", func() {
		store := binding.NewLegacyStore()
		Expect(store.Get()).ToNot(BeNil())
	})

	It("should not allow setting of bindings to nil", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())

		bindings := []binding.Binding{
			{
				Url: "drain-1",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "host-1", AppID: "app-1"}},
					},
				},
			},
		}

		store.Set(bindings, 1)
		store.Set(nil, 1)

		storedBindings := store.Get()
		Expect(storedBindings).ToNot(BeNil())
		Expect(storedBindings).To(BeEmpty())
	})

	It("should not allow setting of legacy bindings to nil", func() {
		store := binding.NewLegacyStore()

		bindings := []binding.LegacyBinding{
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
				store.Set([]binding.Binding{}, 0)
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
				Url: "drain-1",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{
							{Hostname: "host-1", AppID: "app-1"},
							{Hostname: "host-2", AppID: "app-2"},
						},
					},
				},
			},
		}

		store.Set(bindings, 2)

		Expect(metrics.GetMetric("cached_bindings", nil).Value()).
			To(BeNumerically("==", 2))
	})
})
