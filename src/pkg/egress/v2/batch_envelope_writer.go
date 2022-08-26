package v2

import (
	"fmt"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
)

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
			err := processor.Process(env)
			if err != nil {
				return fmt.Errorf("write error: %v", err)
			}
		}
	}

	return bw.writer.Write(envs)
}
