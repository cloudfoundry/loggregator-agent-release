package v2_test

import (
	"errors"

	"code.cloudfoundry.org/go-loggregator/v8/rpc/loggregator_v2"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EnvelopeWriter", func() {
	It("processes envelopes before writing", func() {
		mockSingleWriter := newMockSingleWriter()
		close(mockSingleWriter.WriteOutput.Ret0)

		tagger := v2.NewTagger(nil)
		ew := v2.NewEnvelopeWriter(mockSingleWriter, v2.NewCounterAggregator(tagger.TagEnvelope))
		Expect(ew.Write(buildCounterEnvelope(10, "name-1", "origin-1"))).To(Succeed())

		var receivedEnvelope *loggregator_v2.Envelope
		Expect(mockSingleWriter.WriteInput.Msg).To(Receive(&receivedEnvelope))
		Expect(receivedEnvelope.GetCounter().GetDelta()).To(Equal(uint64(10)))
	})

	It("returns an error if the processor fails", func() {
		mockSingleWriter := newMockSingleWriter()
		close(mockSingleWriter.WriteOutput.Ret0)

		ew := v2.NewEnvelopeWriter(mockSingleWriter, &mockProcessor{processErr: errors.New("expected error")})
		Expect(ew.Write(buildCounterEnvelope(10, "name-1", "origin-1"))).ToNot(Succeed())
	})
})

type mockSingleWriter struct {
	WriteCalled chan bool
	WriteInput  struct {
		Msg chan *loggregator_v2.Envelope
	}
	WriteOutput struct {
		Ret0 chan error
	}
}

func newMockSingleWriter() *mockSingleWriter {
	m := &mockSingleWriter{}
	m.WriteCalled = make(chan bool, 100)
	m.WriteInput.Msg = make(chan *loggregator_v2.Envelope, 100)
	m.WriteOutput.Ret0 = make(chan error, 100)
	return m
}
func (m *mockSingleWriter) Write(msg *loggregator_v2.Envelope) error {
	m.WriteCalled <- true
	m.WriteInput.Msg <- msg
	return <-m.WriteOutput.Ret0
}

type mockProcessor struct {
	processErr error
}

func (p *mockProcessor) Process(*loggregator_v2.Envelope) error {
	return p.processErr
}
