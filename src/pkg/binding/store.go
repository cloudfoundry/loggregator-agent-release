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

func (s *Store) Set(bindings []Binding) {
	if bindings == nil {
		bindings = []Binding{}
	}

	s.mu.Lock()
	s.bindings = bindings
	s.bindingCount.Set(float64(len(s.bindings)))
	s.mu.Unlock()
}

type AggregateStore struct {
	AggregateDrains []string
}

func (store *AggregateStore) Get() []Binding {
	return []Binding{
		{
			AppID:  "",
			Drains: store.AggregateDrains,
		},
	}
}
