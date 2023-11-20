package binding

import (
	"io"
	"os"
	"sync"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"gopkg.in/yaml.v2"
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

type AggregateStore struct {
	Drains []Binding
}

func NewAggregateStore(drainFileName string) *AggregateStore {
	drainFile, err := os.Open(drainFileName)
	if err != nil {
		panic(err)
	}
	contents, err := io.ReadAll(drainFile)
	if err != nil {
		panic(err)
	}

	var bindings []Binding
	var aggBindings []AggBinding
	err = yaml.Unmarshal(contents, &aggBindings)
	if err != nil {
		panic(err)
	}
	for _, binding := range aggBindings {
		bindings = append(bindings, Binding{
			Url: binding.Url,
			Credentials: []Credentials{
				{
					Cert: binding.Cert,
					Key:  binding.Key,
					CA:   binding.CA,
				},
			},
		})
	}
	return &AggregateStore{Drains: bindings}
}

func (store *AggregateStore) Get() []Binding {
	return store.Drains
}
