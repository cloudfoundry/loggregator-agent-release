package v2_test

import (
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Filtering Setter", func() {
	It("filters out logs", func() {
		spySetter := newSpySetter()
		filteringSetter := v2.NewFilteringSetter(spySetter)
		filteringSetter.Set(counterEnvelope)
		filteringSetter.Set(gaugeEnvelope)
		filteringSetter.Set(logEnvelope)
		filteringSetter.Set(timerEnvelope)
		filteringSetter.Set(eventEnvelope)

		Expect(spySetter.envelopes).To(ConsistOf(
			counterEnvelope,
			gaugeEnvelope,
			timerEnvelope,
			eventEnvelope,
		))
	})

})

type spySetter struct {
	envelopes []*loggregator_v2.Envelope
}

func newSpySetter() *spySetter {
	return &spySetter{}
}

func (s *spySetter) Set(e *loggregator_v2.Envelope) {
	s.envelopes = append(s.envelopes, e)
}

var logEnvelope = &loggregator_v2.Envelope{
	Message: &loggregator_v2.Envelope_Log{
		Log: &loggregator_v2.Log{
			Payload: []byte("hello"),
		},
	},
}

var counterEnvelope = &loggregator_v2.Envelope{
	Message: &loggregator_v2.Envelope_Counter{
		Counter: &loggregator_v2.Counter{
			Name: "the-counter",
		},
	},
}

var gaugeEnvelope = &loggregator_v2.Envelope{
	Message: &loggregator_v2.Envelope_Gauge{
		Gauge: &loggregator_v2.Gauge{
			Metrics: map[string]*loggregator_v2.GaugeValue{
				"metric-1": &loggregator_v2.GaugeValue{
					Unit:  "ms",
					Value: 0.0,
				},
			},
		},
	},
}

var timerEnvelope = &loggregator_v2.Envelope{
	Message: &loggregator_v2.Envelope_Timer{
		Timer: &loggregator_v2.Timer{
			Name: "The Timer",
		},
	},
}

var eventEnvelope = &loggregator_v2.Envelope{
	Message: &loggregator_v2.Envelope_Event{
		Event: &loggregator_v2.Event{
			Title: "The Timer",
		},
	},
}
