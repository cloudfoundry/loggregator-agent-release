package cache_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache/cachefakes"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/metricbinding"
)

var _ = Describe("Handler", func() {
	It("should write results from the store", func() {
		bindings := []binding.Binding{
			{
				Url: "drain-1",
				Credentials: []binding.Credentials{
					{
						Cert: "cert", Key: "key", Apps: []binding.App{{Hostname: "host-1", AppID: "app-id-1"}},
					},
				},
			},
		}
		getter := &cachefakes.FakeGetter{}
		getter.GetReturns(bindings)
		handler := cache.Handler(getter)
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&bindings)
		Expect(err).ToNot(HaveOccurred())

		Expect(rw.HeaderMap["Content-Type"]).To(Equal([]string{"application/json"}))
		Expect(rw.Body.String()).To(MatchJSON(j))
	})

	It("should write results from the legacy store", func() {
		bindings := []binding.LegacyBinding{
			{
				AppID:    "app-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		legacyGetter := &cachefakes.FakeLegacyGetter{}
		legacyGetter.GetReturns(bindings)
		handler := cache.LegacyHandler(legacyGetter)
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&bindings)
		Expect(err).ToNot(HaveOccurred())

		Expect(rw.HeaderMap["Content-Type"]).To(Equal([]string{"application/json"}))
		Expect(rw.Body.String()).To(MatchJSON(j))
	})

	It("should write results from the v2 aggregateStore", func() {
		aggregateDrains := []binding.Binding{
			{
				Url: "test",
				Credentials: []binding.Credentials{
					{
						Cert: "cert",
						Key:  "key",
						CA:   "ca",
					},
				},
			},
		}

		aggregateGetter := &cachefakes.FakeAggregateGetter{}
		aggregateGetter.GetReturns(aggregateDrains)
		handler := cache.AggregateHandler(aggregateGetter)
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&aggregateDrains)
		Expect(err).ToNot(HaveOccurred())

		Expect(rw.HeaderMap["Content-Type"]).To(Equal([]string{"application/json"}))
		Expect(rw.Body.String()).To(MatchJSON(j))
	})

	It("should write results from the legacy aggregateStore", func() {
		aggregateDrains := []binding.LegacyBinding{
			{
				AppID:       "",
				Drains:      []string{"drain-1", "drain-2"},
				V2Available: true,
			},
		}

		aggregateGetter := &cachefakes.FakeAggregateGetter{}
		aggregateGetter.LegacyGetReturns(aggregateDrains)
		handler := cache.LegacyAggregateHandler(aggregateGetter)
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&aggregateDrains)
		Expect(err).ToNot(HaveOccurred())

		Expect(rw.HeaderMap["Content-Type"]).To(Equal([]string{"application/json"}))
		Expect(rw.Body.String()).To(MatchJSON(j))
	})

	Describe("AggregateMetricHandler", func() {
		It("returns the metric drains as json", func() {
			aggregateMetricDrains := metricbinding.OtelExporterConfig{
				"otlp": metricbinding.OtelExporterConfig{
					"endpoint": "otelcol:4317",
				},
				"otlp/2": metricbinding.OtelExporterConfig{
					"endpoint": "otelcol2:4317",
				},
			}
			metricDrainGetter := &cachefakes.FakeAggregateMetricGetter{}
			metricDrainGetter.GetReturns(aggregateMetricDrains)
			handler := cache.AggregateMetricHandler(metricDrainGetter)

			rw := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "", nil)
			Expect(err).ToNot(HaveOccurred())
			handler.ServeHTTP(rw, req)

			j, err := json.Marshal(&aggregateMetricDrains)
			Expect(err).ToNot(HaveOccurred())

			Expect(rw.HeaderMap["Content-Type"]).To(Equal([]string{"application/json"}))
			Expect(rw.Body.String()).To(MatchJSON(j))
		})
	})
})
