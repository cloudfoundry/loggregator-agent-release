package cache_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent/pkg/cache"
)

var _ = Describe("Handler", func() {
	It("should write results from the store", func() {
		bindings := []binding.Binding{
			{
				AppID:    "app-1",
				Drains:   []string{"drain-1"},
				Hostname: "host-1",
			},
		}

		handler := cache.Handler(newStubStore(bindings))
		rw := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/bindings", nil)
		Expect(err).ToNot(HaveOccurred())
		handler.ServeHTTP(rw, req)

		j, err := json.Marshal(&bindings)
		Expect(err).ToNot(HaveOccurred())

		Expect(rw.Body.String()).To(MatchJSON(j))
	})
})

type stubStore struct {
	bindings []binding.Binding
}

func newStubStore(bindings []binding.Binding) *stubStore {
	return &stubStore{
		bindings: bindings,
	}
}

func (s *stubStore) Get() []binding.Binding {
	return s.bindings
}
