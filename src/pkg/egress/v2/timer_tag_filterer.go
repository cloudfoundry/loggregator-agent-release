package v2

import (
	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
)

type TimerTagFilterer struct {
	whitelist []string
	processor func(env *loggregator_v2.Envelope)
}

func NewTimerTagFilterer(whitelist []string, processor func(env *loggregator_v2.Envelope)) TimerTagFilterer {
	return TimerTagFilterer{
		whitelist: whitelist,
		processor: processor,
	}
}

func (t TimerTagFilterer) Filter(env *loggregator_v2.Envelope) {
	t.processor(env)

	_, ok := env.GetMessage().(*loggregator_v2.Envelope_Timer)
	if !ok {
		return
	}

	for tag := range env.GetTags() {
		if !t.whitelisted(tag) {
			delete(env.Tags, tag)
		}
	}
}

func (t TimerTagFilterer) whitelisted(tag string) bool {
	for _, whitelistedTag := range t.whitelist {
		if tag == whitelistedTag {
			return true
		}
	}

	return false
}
