package simplecache

import (
	"sync"
	"time"
)

type SimpleCache[K comparable, V any] struct {
	items      map[K]V
	expiration time.Duration
	mu         sync.RWMutex
}

func New[K comparable, V any](expiration time.Duration) *SimpleCache[K, V] {
	return &SimpleCache[K, V]{
		items:      make(map[K]V),
		expiration: expiration,
	}
}

func (sc *SimpleCache[K, V]) Set(key K, value V) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.items[key] = value

	time.AfterFunc(sc.expiration, func() {
		sc.mu.Lock()
		defer sc.mu.Unlock()
		delete(sc.items, key)
	})
}

func (sc *SimpleCache[K, V]) Get(key K) (V, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	value, exists := sc.items[key]
	return value, exists
}
