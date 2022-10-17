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

	It("should write results from the aggregateStore", func() {
		aggregateDrains := []binding.LegacyBinding{
			{
				AppID:  "",
				Drains: []string{"drain-1", "drain-2"},
			},
		}

		handler := cache.AggregateHandler(newStubAggregateStore(aggregateDrains))
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
	AggregateDrains []binding.LegacyBinding
}

func newStubStore(bindings []binding.Binding) *stubStore {
	return &stubStore{
		bindings: bindings,
	}
}

func newStubLegacyStore(bindings []binding.LegacyBinding) *stubLegacyStore {
	return &stubLegacyStore{
		bindings: bindings,
	}
}

func newStubAggregateStore(aggregateDrains []binding.LegacyBinding) *stubAggregateStore {
	return &stubAggregateStore{aggregateDrains}
}

func (s *stubStore) Get() []binding.Binding {
	return s.bindings
}

func (s *stubLegacyStore) Get() []binding.LegacyBinding {
	return s.bindings
}

func (as *stubAggregateStore) Get() []binding.LegacyBinding {
	return as.AggregateDrains
}
