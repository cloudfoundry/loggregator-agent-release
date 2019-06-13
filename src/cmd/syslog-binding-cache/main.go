package main

import (
	"log"
	"os"

	"code.cloudfoundry.org/loggregator-agent/cmd/syslog-binding-cache/app"
)

func main() {
	log := log.New(os.Stderr, "", log.LstdFlags)
	log.Println("starting syslog-binding-cache")
	defer log.Println("stopping syslog-binding-cache")

	cfg := app.LoadConfig()
	app.NewSyslogBindingCache(cfg, log).Run()
}
