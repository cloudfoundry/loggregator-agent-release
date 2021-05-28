package v2_test

import (
	"errors"
	"time"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"
	egress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Transponder", func() {
	It("reads from the buffer to the writer", func() {
		envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
		nexter := newMockNexter()
		nexter.TryNextOutput.Ret0 <- envelope
		nexter.TryNextOutput.Ret1 <- true
		writer := newMockWriter()
		close(writer.WriteOutput.Ret0)

		spy := metricsHelpers.NewMetricsRegistry()

		tx := egress.NewTransponder(nexter, writer, 1, time.Nanosecond, spy)
		go tx.Start()

		Eventually(nexter.TryNextCalled).Should(Receive())
		Eventually(writer.WriteInput.Msg).Should(Receive(Equal([]*loggregator_v2.Envelope{envelope})))
	})

	Describe("batching", func() {
		It("emits once the batch count has been reached", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := newMockNexter()
			writer := newMockWriter()
			close(writer.WriteOutput.Ret0)

			for i := 0; i < 6; i++ {
				nexter.TryNextOutput.Ret0 <- envelope
				nexter.TryNextOutput.Ret1 <- true
			}

			spy := metricsHelpers.NewMetricsRegistry()

			tx := egress.NewTransponder(nexter, writer, 5, time.Minute, spy)
			go tx.Start()

			var batch []*loggregator_v2.Envelope
			Eventually(writer.WriteInput.Msg).Should(Receive(&batch))
			Expect(batch).To(HaveLen(5))
		})

		It("emits once the batch interval has been reached", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := newMockNexter()
			writer := newMockWriter()
			close(writer.WriteOutput.Ret0)

			nexter.TryNextOutput.Ret0 <- envelope
			nexter.TryNextOutput.Ret1 <- true
			close(nexter.TryNextOutput.Ret0)
			close(nexter.TryNextOutput.Ret1)

			spy := metricsHelpers.NewMetricsRegistry()

			tx := egress.NewTransponder(nexter, writer, 5, time.Millisecond, spy)
			go tx.Start()

			var batch []*loggregator_v2.Envelope
			Eventually(writer.WriteInput.Msg).Should(Receive(&batch))
			Expect(batch).To(HaveLen(1))
		})

		It("clears batch upon egress failure", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := newMockNexter()
			writer := newMockWriter()

			go func() {
				for {
					writer.WriteOutput.Ret0 <- errors.New("some-error")
				}
			}()

			for i := 0; i < 6; i++ {
				nexter.TryNextOutput.Ret0 <- envelope
				nexter.TryNextOutput.Ret1 <- true
			}

			spy := metricsHelpers.NewMetricsRegistry()

			tx := egress.NewTransponder(nexter, writer, 5, time.Minute, spy)
			go tx.Start()

			Eventually(writer.WriteCalled).Should(HaveLen(1))
			Consistently(writer.WriteCalled).Should(HaveLen(1))
		})

		It("emits egress and dropped metric", func() {
			envelope := &loggregator_v2.Envelope{SourceId: "uuid"}
			nexter := newMockNexter()
			writer := newMockWriter()
			close(writer.WriteOutput.Ret0)

			for i := 0; i < 6; i++ {
				nexter.TryNextOutput.Ret0 <- envelope
				nexter.TryNextOutput.Ret1 <- true
			}

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
