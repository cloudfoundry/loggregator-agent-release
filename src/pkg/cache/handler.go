package cache

import (
	"encoding/json"
	"log"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/binding"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/metricbinding"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . Getter
type Getter interface {
	Get() []binding.Binding
}

//counterfeiter:generate . LegacyGetter
type LegacyGetter interface {
	Get() []binding.LegacyBinding
}

//counterfeiter:generate . AggregateGetter
type AggregateGetter interface {
	Get() []binding.Binding
	LegacyGet() []binding.LegacyBinding
}

//counterfeiter:generate . AggregateMetricGetter
type AggregateMetricGetter interface {
	Get() metricbinding.OtelExporterConfig
}

func Handler(store Getter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func LegacyHandler(store LegacyGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func AggregateHandler(store AggregateGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func LegacyAggregateHandler(store AggregateGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(store.LegacyGet())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}

func AggregateMetricHandler(store AggregateMetricGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(store.Get())
		if err != nil {
			log.Printf("failed to encode response body: %s", err)
			return
		}
	}
}
