package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"code.cloudfoundry.org/loggregator-agent/cmd/agent/app"
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
	go a.Start()

	runPProf(config.DebugPort)
}

func runPProf(port uint32) {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Panicf("Error creating pprof listener: %s", err)
	}

	logger.Printf("pprof bound to: %s", lis.Addr())
	err = http.Serve(lis, nil)
	if err != nil {
		logger.Panicf("Error starting pprof server: %s", err)
	}
}
