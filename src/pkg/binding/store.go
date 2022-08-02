package binding

import (
	"sync"

	metrics "code.cloudfoundry.org/go-metric-registry"
)

type Store struct {
	mu           sync.Mutex
	bindings     []Binding
	bindingCount metrics.Gauge
}

func NewStore(m Metrics) *Store {
	return &Store{
		bindings: make([]Binding, 0),
		bindingCount: m.NewGauge(
			"cached_bindings",
			"Current number of bindings stored in the binding cache.",
		),
	}
}

func (s *Store) Get() []Binding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindings
}

func (s *Store) Set(bindings []Binding, bindingCount int) {
	if bindings == nil {
		bindings = []Binding{}
		bindingCount = 0
	}

	s.mu.Lock()
	s.bindings = bindings
	s.bindingCount.Set(float64(bindingCount))
	s.mu.Unlock()
}

type LegacyStore struct {
	mu             sync.Mutex
	legacyBindings []LegacyBinding
}

func (s *LegacyStore) Get() []LegacyBinding {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.legacyBindings
}

func (s *LegacyStore) Set(bindings []LegacyBinding) {
	if bindings == nil {
		bindings = []LegacyBinding{}
	}
	s.mu.Lock()
	s.legacyBindings = bindings
	s.mu.Unlock()
}

func NewLegacyStore() *LegacyStore {
	return &LegacyStore{
		legacyBindings: make([]LegacyBinding, 0),
	}
}

type AggregateStore struct {
	AggregateDrains []string
}

func (store *AggregateStore) Get() []string {
	return store.AggregateDrains
}
