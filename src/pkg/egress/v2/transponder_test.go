package v2_test

import (
	"errors"
	"time"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	egress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2/v2fakes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Transponder", func() {
	It("reads from the buffer to the writer", func() {
		envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
		nexter := &v2fakes.FakeNexter{}
		nexter.TryNextReturnsOnCall(0, envelope, true)
		nexter.TryNextReturnsOnCall(1, nil, false)
		writer := &v2fakes.FakeBatchWriter{}

		spy := metricsHelpers.NewMetricsRegistry()

		tx := egress.NewTransponder(nexter, writer, 1, time.Nanosecond, spy)
		go tx.Start()

		Eventually(func() int { return nexter.TryNextCallCount() }).Should(BeNumerically(">", 0))
		Eventually(func() int { return writer.WriteCallCount() }).Should(BeNumerically(">", 0))
		Eventually(func() []*loggregator_v2.Envelope {
			if writer.WriteCallCount() > 0 {
				return writer.WriteArgsForCall(0)
			}
			return nil
		}).Should(Equal([]*loggregator_v2.Envelope{envelope}))
	})

	Describe("batching", func() {
		It("emits once the batch count has been reached", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := &v2fakes.FakeNexter{}
			// Set up to return envelope 6 times to trigger batch of 5
			for i := 0; i < 6; i++ {
				nexter.TryNextReturnsOnCall(i, envelope, true)
			}
			nexter.TryNextReturnsOnCall(6, nil, false)
			writer := &v2fakes.FakeBatchWriter{}

			spy := metricsHelpers.NewMetricsRegistry()

			tx := egress.NewTransponder(nexter, writer, 5, time.Minute, spy)
			go tx.Start()

			Eventually(func() int { return writer.WriteCallCount() }).Should(BeNumerically(">", 0))
			var batch []*loggregator_v2.Envelope
			Eventually(func() []*loggregator_v2.Envelope {
				if writer.WriteCallCount() > 0 {
					batch = writer.WriteArgsForCall(0)
					return batch
				}
				return nil
			}).Should(HaveLen(5))
		})

		It("emits once the batch interval has been reached", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := &v2fakes.FakeNexter{}
			nexter.TryNextReturnsOnCall(0, envelope, true)
			nexter.TryNextReturnsOnCall(1, nil, false)
			writer := &v2fakes.FakeBatchWriter{}

			spy := metricsHelpers.NewMetricsRegistry()

			tx := egress.NewTransponder(nexter, writer, 5, time.Millisecond, spy)
			go tx.Start()

			Eventually(func() int { return writer.WriteCallCount() }).Should(BeNumerically(">", 0))
			var batch []*loggregator_v2.Envelope
			Eventually(func() []*loggregator_v2.Envelope {
				if writer.WriteCallCount() > 0 {
					batch = writer.WriteArgsForCall(0)
					return batch
				}
				return nil
			}).Should(HaveLen(1))
		})

		It("clears batch upon egress failure", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := &v2fakes.FakeNexter{}
			writer := &v2fakes.FakeBatchWriter{}
			writer.WriteReturns(errors.New("some-error"))

			// Set up nexter to return envelope 6 times, then stop
			for i := 0; i < 6; i++ {
				nexter.TryNextReturnsOnCall(i, envelope, true)
			}
			nexter.TryNextReturnsOnCall(6, nil, false)

			spy := metricsHelpers.NewMetricsRegistry()

			tx := egress.NewTransponder(nexter, writer, 5, time.Minute, spy)
			go tx.Start()

			Eventually(func() int { return writer.WriteCallCount() }).Should(Equal(1))
			Consistently(func() int { return writer.WriteCallCount() }).Should(Equal(1))
		})

		It("emits egress and dropped metric", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := &v2fakes.FakeNexter{}
			// Set up to return envelope 6 times to trigger batch of 5
			for i := 0; i < 6; i++ {
				nexter.TryNextReturnsOnCall(i, envelope, true)
			}
			nexter.TryNextReturnsOnCall(6, nil, false)
			writer := &v2fakes.FakeBatchWriter{}

			spy := metricsHelpers.NewMetricsRegistry()
			tx := egress.NewTransponder(nexter, writer, 5, time.Minute, spy)
			go tx.Start()

			Eventually(hasMetric(spy, "egress", map[string]string{"metric_version": "2.0"}))
			Eventually(hasMetric(spy, "dropped", map[string]string{"direction": "egress", "metric_version": "2.0"}))

		})
	})
})

func hasMetric(mc *metricsHelpers.SpyMetricsRegistry, metricName string, tags map[string]string) func() bool {
	return func() bool {
		return mc.HasMetric(metricName, tags)
	}
}
