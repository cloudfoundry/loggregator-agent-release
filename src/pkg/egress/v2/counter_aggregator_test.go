package v2_test

import (
	"fmt"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	egress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CounterAggregator", func() {
	tagger := egress.NewTagger(nil)

	It("calculates separate totals for envelopes with deprecated defaultTags", func() {
		tagger := egress.NewTagger(nil)

		aggregator := egress.NewCounterAggregator(tagger.TagEnvelope)
		env1 := buildCounterEnvelope(10, "name-1", "origin-1")
		env1.DeprecatedTags = map[string]*loggregator_v2.Value{
			"tag-1": {Data: &loggregator_v2.Value_Text{Text: "text-value"}},
		}

		env2 := buildCounterEnvelope(15, "name-1", "origin-1")
		env2.DeprecatedTags = map[string]*loggregator_v2.Value{
			"tag-2": {Data: &loggregator_v2.Value_Text{Text: "text-value"}},
		}

		env3 := buildCounterEnvelope(20, "name-1", "origin-1")
		env3.DeprecatedTags = map[string]*loggregator_v2.Value{
			"tag-3": {Data: &loggregator_v2.Value_Text{Text: "text-value"}},
		}

		Expect(aggregator.Process(env1)).ToNot(HaveOccurred())
		Expect(aggregator.Process(env2)).ToNot(HaveOccurred())
		Expect(aggregator.Process(env3)).ToNot(HaveOccurred())

		Expect(env1.GetCounter().GetTotal()).To(Equal(uint64(10)))
		Expect(env2.GetCounter().GetTotal()).To(Equal(uint64(15)))
		Expect(env3.GetCounter().GetTotal()).To(Equal(uint64(20)))
	})

	It("overwrites aggregated total when total is set", func() {
		aggregator := egress.NewCounterAggregator(tagger.TagEnvelope)
		env1 := buildCounterEnvelope(10, "name-1", "origin-1")
		env2 := buildCounterEnvelopeWithTotal(5000, "name-1", "origin-1")
		env3 := buildCounterEnvelope(10, "name-1", "origin-1")

		Expect(aggregator.Process(env1)).ToNot(HaveOccurred())
		Expect(aggregator.Process(env2)).ToNot(HaveOccurred())
		Expect(aggregator.Process(env3)).ToNot(HaveOccurred())

		Expect(env1.GetCounter().GetTotal()).To(Equal(uint64(10)))
		Expect(env2.GetCounter().GetTotal()).To(Equal(uint64(5000)))
		Expect(env3.GetCounter().GetTotal()).To(Equal(uint64(5010)))
	})

	It("overwrites total when both delta and total are 0", func() {
		aggregator := egress.NewCounterAggregator(tagger.TagEnvelope)
		env1 := buildCounterEnvelopeWithTotal(10, "name-1", "origin-1")
		env2 := buildCounterEnvelopeWithTotal(0, "name-1", "origin-1")

		Expect(aggregator.Process(env1)).ToNot(HaveOccurred())
		Expect(aggregator.Process(env2)).ToNot(HaveOccurred())

		Expect(env1.GetCounter().GetTotal()).To(Equal(uint64(10)))
		Expect(env2.GetCounter().GetTotal()).To(Equal(uint64(0)))
	})

	It("maintains separate totals for different source ids", func() {
		aggregator := egress.NewCounterAggregator(tagger.TagEnvelope)
		env1 := buildCounterEnvelope(10, "name-1", "origin-1")
		env1.SourceId = "source-id-1"
		env2 := buildCounterEnvelope(10, "name-1", "origin-1")
		env1.SourceId = "source-id-2"

		Expect(aggregator.Process(env1)).ToNot(HaveOccurred())
		Expect(aggregator.Process(env2)).ToNot(HaveOccurred())

		Expect(env1.GetCounter().GetTotal()).To(Equal(uint64(10)))
		Expect(env2.GetCounter().GetTotal()).To(Equal(uint64(10)))
	})

	It("prunes the cache of totals when there are too many unique counters", func() {
		aggregator := egress.NewCounterAggregator(tagger.TagEnvelope)

		env1 := buildCounterEnvelope(500, "unique-name", "origin-1")

		Expect(aggregator.Process(env1)).ToNot(HaveOccurred())
		Expect(env1.GetCounter().GetTotal()).To(Equal(uint64(500)))

		for i := 0; i < 10000; i++ {
			_ = aggregator.Process(buildCounterEnvelope(10, fmt.Sprint("name-", i), "origin-1"))
		}

		env2 := buildCounterEnvelope(10, "unique-name", "origin-1")
		_ = aggregator.Process(env2)

		Expect(env2.GetCounter().GetTotal()).To(Equal(uint64(10)))
	})

	It("keeps the delta as part of the message", func() {
		aggregator := egress.NewCounterAggregator(tagger.TagEnvelope)
		env1 := buildCounterEnvelope(10, "name-1", "origin-1")

		Expect(aggregator.Process(env1)).ToNot(HaveOccurred())
		Expect(env1.GetCounter().GetDelta()).To(Equal(uint64(10)))
	})
})

func buildCounterEnvelope(delta uint64, name, origin string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Delta: delta,
			},
		},
		Tags: map[string]string{
			"origin": origin,
		},
	}
}

func buildCounterEnvelopeWithTotal(total uint64, name, origin string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  name,
				Total: total,
			},
		},
		Tags: map[string]string{
			"origin": origin,
		},
	}
}
