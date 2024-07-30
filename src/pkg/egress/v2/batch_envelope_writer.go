package v2

import "code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"

type BatchEnvelopeWriter struct {
	writer     BatchWriter
	processors []EnvelopeProcessor
}

func NewBatchEnvelopeWriter(w BatchWriter, ps ...EnvelopeProcessor) BatchEnvelopeWriter {
	return BatchEnvelopeWriter{
		writer:     w,
		processors: ps,
	}
}

func (bw BatchEnvelopeWriter) Write(envs []*loggregator_v2.Envelope) error {
	for _, env := range envs {
		for _, processor := range bw.processors {
			_ = processor.Process(env)
		}
	}

	return bw.writer.Write(envs)
}
