package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	_ "net/http/pprof"
	"time"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/agent/app"
	"google.golang.org/grpc/grpclog"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	rand.Seed(time.Now().UnixNano())
	grpclog.SetLogger(log.New(ioutil.Discard, "", 0))

	config, err := app.LoadConfig()
	if err != nil {
		log.Fatalf("Unable to parse config: %s", err)
	}

	a := app.NewAgent(config)
	a.Start()
}
