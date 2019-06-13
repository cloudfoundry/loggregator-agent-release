package cache

import (
	"encoding/json"
	"log"
	"net/http"

	"code.cloudfoundry.org/loggregator-agent/pkg/binding"
)

type Getter interface {
	Get() []binding.Binding
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
