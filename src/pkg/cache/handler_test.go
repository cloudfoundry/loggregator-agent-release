package cache_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/cache"
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

		handler := cache.Handler(newStubStore(bindings))
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v2/bindings", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&bindings)
		Expect(err).ToNot(HaveOccurred())

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

		handler := cache.LegacyHandler(newStubLegacyStore(bindings))
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/bindings", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&bindings)
		Expect(err).ToNot(HaveOccurred())

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

		handler := cache.AggregateHandler(newStubAggregateStore(nil, aggregateDrains))
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v2/aggregate", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&aggregateDrains)
		Expect(err).ToNot(HaveOccurred())
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

		handler := cache.LegacyAggregateHandler(newStubAggregateStore(aggregateDrains, nil))
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/aggregate", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&aggregateDrains)
		Expect(err).ToNot(HaveOccurred())
		Expect(rw.Body.String()).To(MatchJSON(j))
	})
})

type stubStore struct {
	bindings []binding.Binding
}

type stubLegacyStore struct {
	bindings []binding.LegacyBinding
}

type stubAggregateStore struct {
	AggregateDrains       []binding.Binding
	LegacyAggregateDrains []binding.LegacyBinding
}

func newStubStore(bindings []binding.Binding) *stubStore {
	return &stubStore{
		bindings: bindings,
	}
}

func (s *stubStore) Get() []binding.Binding {
	return s.bindings
}

func newStubLegacyStore(bindings []binding.LegacyBinding) *stubLegacyStore {
	return &stubLegacyStore{
		bindings: bindings,
	}
}

func (s *stubLegacyStore) Get() []binding.LegacyBinding {
	return s.bindings
}

func newStubAggregateStore(legacyAggregateDrains []binding.LegacyBinding, aggregateDrains []binding.Binding) *stubAggregateStore {
	return &stubAggregateStore{LegacyAggregateDrains: legacyAggregateDrains, AggregateDrains: aggregateDrains}
}

func (as *stubAggregateStore) Get() []binding.Binding {
	return as.AggregateDrains
}

func (as *stubAggregateStore) LegacyGet() []binding.LegacyBinding {
	return as.LegacyAggregateDrains
}
