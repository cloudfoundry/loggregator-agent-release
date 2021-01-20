package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	_ "net/http/pprof"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/agent/app"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"google.golang.org/grpc/grpclog"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))

	config, err := app.LoadConfig()
	if config.UseRFC3339 {
		log.SetOutput(new(plumbing.LogWriter))
		log.SetFlags(0)
	} else {
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	}

	if err != nil {
		log.Fatalf("Unable to parse config: %s", err)
	}

	a := app.NewAgent(config)
	a.Start()
}
