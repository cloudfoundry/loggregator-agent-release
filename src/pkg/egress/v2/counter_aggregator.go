package v2

import (
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
)

type counterID struct {
	name     string
	sourceID string
	tagsHash string
}

type CounterAggregator struct {
	counterTotals map[counterID]uint64
	processor     func(env *loggregator_v2.Envelope)
}

func NewCounterAggregator(processor func(env *loggregator_v2.Envelope)) *CounterAggregator {
	return &CounterAggregator{
		counterTotals: make(map[counterID]uint64),
		processor:     processor,
	}
}

func (ca *CounterAggregator) Process(env *loggregator_v2.Envelope) error {
	ca.processor(env)

	c := env.GetCounter()
	if c != nil {
		if len(ca.counterTotals) > 10000 {
			ca.resetTotals()
		}

		id := counterID{
			name:     c.Name,
			sourceID: env.GetSourceId(),
			tagsHash: HashTags(env.GetTags()),
		}

		if c.GetTotal() == 0 && c.GetDelta() != 0 {
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
