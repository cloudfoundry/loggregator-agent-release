package v2

import (
	"strconv"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
)

type Tagger struct {
	defaultTags map[string]string
}

func NewTagger(ts map[string]string) Tagger {
	return Tagger{
		defaultTags: ts,
	}
}

func (t Tagger) TagEnvelope(env *loggregator_v2.Envelope) {
	if env.Tags == nil {
		env.Tags = make(map[string]string)
	}

	t.moveDeprecatedTags(env)
	t.addDefaultTags(env)
}

func (t Tagger) moveDeprecatedTags(env *loggregator_v2.Envelope) {
	// Move deprecated defaultTags to defaultTags.
	for k, v := range env.GetDeprecatedTags() {
		switch v.Data.(type) {
		case *loggregator_v2.Value_Text:
			env.Tags[k] = v.GetText()
		case *loggregator_v2.Value_Integer:
			env.Tags[k] = strconv.FormatInt(v.GetInteger(), 10)
		case *loggregator_v2.Value_Decimal:
			env.Tags[k] = strconv.FormatFloat(v.GetDecimal(), 'f', -1, 64)
		default:
			env.Tags[k] = v.String()
		}
	}
}

func (t Tagger) addDefaultTags(env *loggregator_v2.Envelope) {
	for k, v := range t.defaultTags {
		if _, ok := env.Tags[k]; !ok {
			env.Tags[k] = v
		}
	}
}
