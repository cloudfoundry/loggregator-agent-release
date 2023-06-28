package collector_test

import (
	"errors"

	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/collector"
	"code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager/app/collector/collectorfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ChangeGetter", func() {
	var (
		g *collector.ChangeGetter
		c = &collectorfakes.FakeGetter{}
	)

	Describe("GetAggregateMetric", func() {
		BeforeEach(func() {
			c.GetAggregateMetricReturns(map[string]any{"foo": "bar"}, nil)
		})
		It("calls GetAggregateMetric on the underlying client", func() {
			g = collector.NewChangeGetter(c)
			cfg, err := g.GetAggregateMetric()
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg).To(Equal(map[string]any{"foo": "bar"}))
		})
		Context("when the client errors", func() {
			BeforeEach(func() {
				c.GetAggregateMetricReturns(nil, errors.New("some error"))
			})
			It("errors", func() {
				g = collector.NewChangeGetter(c)
				cfg, err := g.GetAggregateMetric()
				Expect(err).To(MatchError("some error"))
				Expect(cfg).To(BeNil())
			})
		})
	})

	Describe("Changed", func() {
		BeforeEach(func() {
			c.GetAggregateMetricReturns(map[string]any{"some": "config"}, nil)
		})

		Context("when it is first called", func() {
			It("returns true", func() {
				g = collector.NewChangeGetter(c)
				_, err := g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeTrue())
			})
		})

		Context("when the config is not changing", func() {
			It("returns false", func() {
				g = collector.NewChangeGetter(c)
				_, err := g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeTrue())
				Consistently(func() bool {
					_, err := g.GetAggregateMetric()
					Expect(err).ToNot(HaveOccurred())
					return g.Changed()
				}).Should(BeFalse())
			})
		})

		Context("when the config changes some of the time", func() {
			BeforeEach(func() {
				c = &collectorfakes.FakeGetter{}
				c.GetAggregateMetricReturnsOnCall(0, map[string]any{"some": "config"}, nil)
				c.GetAggregateMetricReturnsOnCall(1, map[string]any{"different": "config"}, nil)
				c.GetAggregateMetricReturnsOnCall(2, map[string]any{"some": "config"}, nil)
				c.GetAggregateMetricReturnsOnCall(3, map[string]any{"some": "config"}, nil)
				c.GetAggregateMetricReturnsOnCall(4, nil, errors.New("some error"))
				c.GetAggregateMetricReturnsOnCall(5, map[string]any{"some": "config"}, nil)
			})
			It("reports when it has changed", func() {
				g = collector.NewChangeGetter(c)

				By("returning true for the first call")
				_, err := g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeTrue())

				By("returning true because the config has changed")
				_, err = g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeTrue())

				By("returning true because the config has changed back")
				_, err = g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeTrue())

				By("returning false because the config has not changed")
				_, err = g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeFalse())

				By("returning false when an error occurred")
				_, err = g.GetAggregateMetric()
				Expect(err).To(MatchError(errors.New("some error")))
				Expect(g.Changed()).To(BeFalse())

				By("returning false because the config has not changed")
				_, err = g.GetAggregateMetric()
				Expect(err).ToNot(HaveOccurred())
				Expect(g.Changed()).To(BeFalse())
			})
		})

	})

})
