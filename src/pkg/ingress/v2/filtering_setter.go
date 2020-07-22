package v2

import "code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"

type setter interface {
	Set(e *loggregator_v2.Envelope)
}

type FilteringSetter struct {
	s setter
}

func NewFilteringSetter(s setter) *FilteringSetter {
	return &FilteringSetter{
		s: s,
	}
}

func (fs *FilteringSetter) Set(e *loggregator_v2.Envelope) {
	switch (e.Message).(type) {
	case *loggregator_v2.Envelope_Log:
		return
	default:
		fs.s.Set(e)
	}
}
