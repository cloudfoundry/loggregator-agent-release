package binding

import (
	"code.cloudfoundry.org/go-loggregator/metrics"
	"sync"
)

type Store struct {
	mu           sync.Mutex
	bindings     []Binding
	bindingCount metrics.Gauge
}

func NewStore(m Metrics) *Store {
	return &Store{
		bindings: make([]Binding, 0),
		bindingCount: m.NewGauge("cached_bindings"),
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
