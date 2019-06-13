package collector

import (
	"log"
	"time"
)

const envelopeOrigin = "system-metrics-agent"

type InputFunc func() (SystemStat, error)

type Processor struct {
	in       InputFunc
	senders  []StatsSender
	interval time.Duration
	log      *log.Logger
}

type StatsSender interface {
	Send(SystemStat)
}

func NewProcessor(
	in InputFunc,
	senders []StatsSender,
	interval time.Duration,
	log *log.Logger,
) *Processor {
	return &Processor{
		in:       in,
		senders:  senders,
		interval: interval,
		log:      log,
	}
}

func (p *Processor) Run() {
	t := time.NewTicker(p.interval)

	for range t.C {
		stat, err := p.in()
		if err != nil {
			p.log.Printf("failed to read stats: %s", err)
			continue
		}

		for _, s := range p.senders {
			s.Send(stat)
		}
	}
}
