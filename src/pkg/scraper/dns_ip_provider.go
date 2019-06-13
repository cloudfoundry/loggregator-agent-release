package scraper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type record []string

type dns struct {
	Records []record
}

func NewDNSScrapeTargetProvider(sourceID, dnsFile string, port int) TargetProvider {
	return func() []Target {
		file, err := os.Open(dnsFile)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		var d dns
		err = json.NewDecoder(file).Decode(&d)
		if err != nil {
			panic(err)
		}

		var targets []Target
		for _, r := range d.Records {
			ip := r[0]

			targets = append(targets, Target{
				ID: sourceID,
				MetricURL: fmt.Sprintf("https://%s:%d/metrics", ip, port),
			})
		}

		return targets
	}
}
