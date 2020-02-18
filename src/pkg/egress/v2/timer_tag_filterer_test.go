package v2_test

import (
	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Timer Tag Filterer", func() {
	It("removes all tags not in whitelist", func() {
		tags := map[string]string{
			"tag-one":       "value-one",
			"whitelist-one": "whitelist-one",
			"tag-two":       "value-two",
			"whitelist-two": "whitelist-two",
		}
		env := &loggregator_v2.Envelope{
			SourceId: "uuid",
			Tags:     tags,
			Message: &loggregator_v2.Envelope_Timer{
				Timer: &loggregator_v2.Timer{Name: "http"},
			},
		}

		whitelist := []string{
			"whitelist-one",
			"whitelist-two",
		}
		tagger := v2.NewTimerTagFilterer(whitelist, func(*loggregator_v2.Envelope) {})
		tagger.Filter(env)

		Expect(env.Tags).To(Equal(map[string]string{
			"whitelist-one": "whitelist-one",
			"whitelist-two": "whitelist-two",
		}))
	})

	It("only applies to timers", func() {
		tags := map[string]string{
			"tag-one":       "value-one",
			"whitelist-one": "whitelist-one",
			"tag-two":       "value-two",
			"whitelist-two": "whitelist-two",
		}
		env := &loggregator_v2.Envelope{
			SourceId: "uuid",
			Tags:     tags,
		}

		whitelist := []string{
			"whitelist-one",
			"whitelist-two",
		}
		tagger := v2.NewTimerTagFilterer(whitelist, func(*loggregator_v2.Envelope) {})
		tagger.Filter(env)

		Expect(env.Tags).To(Equal(map[string]string{
			"tag-one":       "value-one",
			"whitelist-one": "whitelist-one",
			"tag-two":       "value-two",
			"whitelist-two": "whitelist-two",
		}))
	})

	It("applies processor to all envelope types", func() {
		tags := map[string]string{
			"tag-one":       "value-one",
			"whitelist-one": "whitelist-one",
			"tag-two":       "value-two",
			"whitelist-two": "whitelist-two",
		}
		env := &loggregator_v2.Envelope{
			SourceId: "uuid",
			Tags:     tags,
		}

		whitelist := []string{
			"whitelist-one",
			"whitelist-two",
		}
		tagger := v2.NewTimerTagFilterer(whitelist, func(env *loggregator_v2.Envelope) {
			env.Tags["processor"] = "processor"
		})
		tagger.Filter(env)

		Expect(env.Tags).To(Equal(map[string]string{
			"tag-one":       "value-one",
			"whitelist-one": "whitelist-one",
			"tag-two":       "value-two",
			"whitelist-two": "whitelist-two",
			"processor":     "processor",
		}))
	})
})
