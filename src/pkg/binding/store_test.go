package binding_test

import (
	"os"

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

	It("should not return nil bindings", func() {
		store := binding.NewStore(metricsHelpers.NewMetricsRegistry())
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
	It("can read and store drains using the new file format with certs", func() {
		aggDrainFile := makeAggDrainFile(`---
- url: "syslog://test-hostname:1000"
  ca: ca
  cert: cert
  key: key
- url: "syslog://test2:1000"
  ca: ca2
  cert: cert2
  key: key2
`)
		aggStore := binding.NewAggregateStore(aggDrainFile)

		Expect(aggStore.Get()).To(ConsistOf(
			binding.Binding{
				Url: "syslog://test-hostname:1000",
				Credentials: []binding.Credentials{
					{
						Cert: "cert",
						Key:  "key",
						CA:   "ca",
					},
				},
			},
			binding.Binding{
				Url: "syslog://test2:1000",
				Credentials: []binding.Credentials{{
					Cert: "cert2",
					Key:  "key2",
					CA:   "ca2",
				},
				},
			},
		))
	})
})

func makeAggDrainFile(write string) string {
	aggDrainFile, err := os.CreateTemp("", "aggregate-drains")
	Expect(err).ToNot(HaveOccurred())
	_, err = aggDrainFile.WriteString(write)
	Expect(err).ToNot(HaveOccurred())
	err = aggDrainFile.Close()
	Expect(err).ToNot(HaveOccurred())
	return aggDrainFile.Name()
}
