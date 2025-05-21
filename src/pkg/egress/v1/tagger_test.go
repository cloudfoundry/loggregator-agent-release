package v1_test

import (
	egress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v1"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/v1/v1fakes"
	"github.com/cloudfoundry/sonde-go/events"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tagger", func() {
	It("tags events with the given deployment name, job, index and IP address", func() {
		mockWriter := &v1fakes.FakeEnvelopeWriter{}
		t := egress.NewTagger(
			"test-deployment",
			"test-job",
			"2",
			"123.123.123.123",
			mockWriter,
		)
		envelope := &events.Envelope{
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("metricName"),
				Value: proto.Float64(2.0),
				Unit:  proto.String("seconds"),
			},
		}

		t.Write(envelope)

		Expect(mockWriter.WriteCallCount()).To(Equal(1))
		expected := &events.Envelope{
			EventType: events.Envelope_ValueMetric.Enum(),
			ValueMetric: &events.ValueMetric{
				Name:  proto.String("metricName"),
				Value: proto.Float64(2.0),
				Unit:  proto.String("seconds"),
			},
			Deployment: proto.String("test-deployment"),
			Job:        proto.String("test-job"),
			Index:      proto.String("2"),
			Ip:         proto.String("123.123.123.123"),
		}
		Expect(mockWriter.WriteArgsForCall(0)).To(Equal(expected))
	})

	Context("doesn't overwrite", func() {
		var (
			mockWriter *v1fakes.FakeEnvelopeWriter
			t          *egress.Tagger
			envelope   *events.Envelope
		)

		BeforeEach(func() {
			mockWriter = &v1fakes.FakeEnvelopeWriter{}
			t = egress.NewTagger(
				"test-deployment",
				"test-job",
				"2",
				"123.123.123.123",
				mockWriter,
			)

			envelope = &events.Envelope{
				EventType: events.Envelope_ValueMetric.Enum(),
				ValueMetric: &events.ValueMetric{
					Name:  proto.String("metricName"),
					Value: proto.Float64(2.0),
					Unit:  proto.String("seconds"),
				},
			}
		})

		It("when deployment is already set", func() {
			envelope.Deployment = proto.String("another-deployment")
			t.Write(envelope)

			Expect(mockWriter.WriteCallCount()).To(Equal(1))
			writtenEnvelope := mockWriter.WriteArgsForCall(0)
			Expect(*writtenEnvelope.Deployment).To(Equal("another-deployment"))
		})

		It("when job is already set", func() {
			envelope.Job = proto.String("another-job")
			t.Write(envelope)

			Expect(mockWriter.WriteCallCount()).To(Equal(1))
			writtenEnvelope := mockWriter.WriteArgsForCall(0)
			Expect(*writtenEnvelope.Job).To(Equal("another-job"))
		})

		It("when index is already set", func() {
			envelope.Index = proto.String("3")
			t.Write(envelope)

			Expect(mockWriter.WriteCallCount()).To(Equal(1))
			writtenEnvelope := mockWriter.WriteArgsForCall(0)
			Expect(*writtenEnvelope.Index).To(Equal("3"))
		})

		It("when ip is already set", func() {
			envelope.Ip = proto.String("1.1.1.1")
			t.Write(envelope)

			Expect(mockWriter.WriteCallCount()).To(Equal(1))
			writtenEnvelope := mockWriter.WriteArgsForCall(0)
			Expect(*writtenEnvelope.Ip).To(Equal("1.1.1.1"))
		})
	})
})
