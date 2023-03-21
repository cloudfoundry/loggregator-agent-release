package cache

import (
	"encoding/json"
	"log"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
)

type Getter interface {
	Get() []binding.Binding
}

type LegacyGetter interface {
	Get() []binding.LegacyBinding
}

type AggregateGetter interface {
	Get() []binding.Binding
	LegacyGet() []binding.LegacyBinding
}

func Handler(store Getter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func LegacyHandler(store LegacyGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func AggregateHandler(store AggregateGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func LegacyAggregateHandler(store AggregateGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := json.NewEncoder(w).Encode(store.LegacyGet())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}
