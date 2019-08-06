package binding_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
)

var _ = Describe("Poller", func() {
	var (
		apiClient *fakeAPIClient
		store     *fakeStore
	)

	BeforeEach(func() {
		apiClient = newFakeAPIClient()
		store = newFakeStore()
	})

	It("polls for bindings on an interval", func() {
		p := binding.NewPoller(apiClient, 10*time.Millisecond, store)
		go p.Poll()

		Eventually(apiClient.called).Should(BeNumerically(">=", 2))
	})

	It("calls the api client and stores the result", func() {
		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {
					Drains:   []string{"drain-1", "drain-2"},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store)
		go p.Poll()

		var expected []binding.Binding
		Eventually(store.bindings).Should(Receive(&expected))
		Expect(expected).To(ConsistOf(binding.Binding{
			AppID:    "app-id-1",
			Drains:   []string{"drain-1", "drain-2"},
			Hostname: "app-hostname",
		}))
	})

	It("fetches the next page of bindings and stores the result", func() {
		apiClient.bindings <- response{
			NextID: 2,
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-1": {
					Drains:   []string{"drain-1", "drain-2"},
					Hostname: "app-hostname",
				},
			},
		}

		apiClient.bindings <- response{
			Results: map[string]struct {
				Drains   []string
				Hostname string
			}{
				"app-id-2": {
					Drains:   []string{"drain-3", "drain-4"},
					Hostname: "app-hostname",
				},
			},
		}

		p := binding.NewPoller(apiClient, 10*time.Millisecond, store)
		go p.Poll()

		var expected []binding.Binding
		Eventually(store.bindings).Should(Receive(&expected))
		Expect(expected).To(ConsistOf(
			binding.Binding{
				AppID:    "app-id-1",
				Drains:   []string{"drain-1", "drain-2"},
				Hostname: "app-hostname",
			},
			binding.Binding{
				AppID:    "app-id-2",
				Drains:   []string{"drain-3", "drain-4"},
				Hostname: "app-hostname",
			},
		))

		Expect(apiClient.requestedIDs).To(ConsistOf(0, 2))
	})
})

type fakeAPIClient struct {
	numRequests  int64
	bindings     chan response
	requestedIDs []int
}

func newFakeAPIClient() *fakeAPIClient {
	return &fakeAPIClient{
		bindings: make(chan response, 100),
	}
}

func (c *fakeAPIClient) Get(nextID int) (*http.Response, error) {
	atomic.AddInt64(&c.numRequests, 1)

	var binding response
	select {
	case binding = <-c.bindings:
		c.requestedIDs = append(c.requestedIDs, nextID)
	default:
	}

	b, err := json.Marshal(&binding)
	Expect(err).ToNot(HaveOccurred())
	resp := &http.Response{
		Body: ioutil.NopCloser(bytes.NewReader(b)),
	}

	return resp, nil
}

func (c *fakeAPIClient) called() int64 {
	return atomic.LoadInt64(&c.numRequests)
}

type fakeStore struct {
	bindings chan []binding.Binding
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		bindings: make(chan []binding.Binding, 100),
	}
}

func (c *fakeStore) Set(b []binding.Binding) {
	c.bindings <- b
}

type response struct {
	Results map[string]struct {
		Drains   []string
		Hostname string
	}
	NextID int `json:"next_id"`
}
