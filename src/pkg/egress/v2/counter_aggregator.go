package v2

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
)

type counterID struct {
	name     string
	tagsHash string
}

type CounterAggregator struct {
	counterTotals map[counterID]uint64
	tagger Tagger
}

func NewCounterAggregator(tagger Tagger) *CounterAggregator {
	return &CounterAggregator{
		counterTotals: make(map[counterID]uint64),
		tagger: tagger,
	}
}

func (ca *CounterAggregator) Process(env *loggregator_v2.Envelope) error {
	ca.tagger.TagEnvelope(env)

	c := env.GetCounter()
	if c != nil {
		if len(ca.counterTotals) > 10000 {
			ca.resetTotals()
		}

		id := counterID{
			name:     c.Name,
			tagsHash: HashTags(env.GetTags()),
		}

		if c.GetTotal() == 0 {
			ca.counterTotals[id] = ca.counterTotals[id] + c.GetDelta()
		} else {
			ca.counterTotals[id] = c.GetTotal()
		}

		c.Total = ca.counterTotals[id]
	}

	return nil
}

func (ca *CounterAggregator) resetTotals() {
	ca.counterTotals = make(map[counterID]uint64)
}
