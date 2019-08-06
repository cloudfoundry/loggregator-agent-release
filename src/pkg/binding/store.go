package binding

import "sync"

type Store struct {
	mu       sync.Mutex
	bindings []Binding
}

func NewStore() *Store {
	return &Store{
		bindings: make([]Binding, 0),
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
	s.mu.Unlock()
}
