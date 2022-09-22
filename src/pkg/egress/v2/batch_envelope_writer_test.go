package v2_test

import (
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BatchEnvelopeWriter", func() {
	It("processes each envelope before writing", func() {
		mockWriter := newMockBatchWriter()
		close(mockWriter.WriteOutput.Ret0)

		tagger := v2.NewTagger(nil)
		ew := v2.NewBatchEnvelopeWriter(mockWriter, v2.NewCounterAggregator(tagger.TagEnvelope))
		envs := []*loggregator_v2.Envelope{
			buildCounterEnvelope(10, "name-1", "origin-1"),
			buildCounterEnvelope(14, "name-2", "origin-1"),
		}

		Expect(ew.Write(envs)).ToNot(HaveOccurred())

		var batch []*loggregator_v2.Envelope
		Eventually(mockWriter.WriteInput.Msgs).Should(Receive(&batch))

		Expect(batch).To(HaveLen(2))
		Expect(batch[0].GetCounter().GetTotal()).To(Equal(uint64(10)))
		Expect(batch[1].GetCounter().GetTotal()).To(Equal(uint64(14)))
	})
})
