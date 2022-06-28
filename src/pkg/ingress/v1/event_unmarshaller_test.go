package v1_test

import (
	ingress "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v1"
	"github.com/cloudfoundry/sonde-go/events"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("EventUnmarshaller", func() {
	var (
		mockWriter   *MockEnvelopeWriter
		unmarshaller *ingress.EventUnmarshaller
		event        *events.Envelope
		message      []byte
	)

	BeforeEach(func() {
		mockWriter = &MockEnvelopeWriter{}

		unmarshaller = ingress.NewUnMarshaller(mockWriter)
		event = &events.Envelope{
			Origin:      proto.String("fake-origin-3"),
			EventType:   events.Envelope_ValueMetric.Enum(),
			ValueMetric: NewValueMetric("value-name", 1.0, "units"),
			Tags: map[string]string{
				"source_id": "my-source-id",
			},
		}
		message, _ = proto.Marshal(event)
	})

	Context("UnmarshallMessage", func() {
		It("unmarshalls bytes", func() {
			output, _ := unmarshaller.UnmarshallMessage(message)

			Expect(proto.Equal(output, event)).To(BeTrue())
		})

		It("handles bad input gracefully", func() {
			output, err := unmarshaller.UnmarshallMessage(make([]byte, 4))
			Expect(output).To(BeNil())
			Expect(err).To(HaveOccurred())
		})

		It("doesn't write unknown event types", func() {
			unknownEventTypeMessage := &events.Envelope{
				Origin:    proto.String("fake-origin-2"),
				EventType: events.Envelope_EventType(2000).Enum(),
				ValueMetric: &events.ValueMetric{
					Name:  proto.String("fake-metric-name"),
					Value: proto.Float64(42),
					Unit:  proto.String("fake-unit"),
				},
			}
			message, err := proto.Marshal(unknownEventTypeMessage)
			Expect(err).ToNot(HaveOccurred())

			output, err := unmarshaller.UnmarshallMessage(message)
			Expect(output).To(BeNil())
			Expect(err).To(HaveOccurred())
		})

		Context("with no source_id tag", func() {
			BeforeEach(func() {
				event = &events.Envelope{
					Origin:      proto.String("fake-origin-3"),
					EventType:   events.Envelope_ValueMetric.Enum(),
					ValueMetric: NewValueMetric("value-name", 1.0, "units"),
				}
				message, _ = proto.Marshal(event)
			})

			It("sets source_id tag to origin value", func() {
				output, _ := unmarshaller.UnmarshallMessage(message)

				eventWithSourceID := &events.Envelope{
					Origin:      proto.String("fake-origin-3"),
					EventType:   events.Envelope_ValueMetric.Enum(),
					ValueMetric: NewValueMetric("value-name", 1.0, "units"),
					Tags:        map[string]string{"source_id": "fake-origin-3"},
				}

				Expect(proto.Equal(output, eventWithSourceID)).To(BeTrue())
			})
		})
	})

	Context("Write", func() {
		It("unmarshalls byte arrays and writes to an EnvelopeWriter", func() {
			unmarshaller.Write(message)

			Expect(mockWriter.Events).To(HaveLen(1))
			Expect(proto.Equal(mockWriter.Events[0], event)).To(BeTrue())
		})

		It("returns an error when it can't unmarshal", func() {
			message = []byte("Bad Message")
			unmarshaller.Write(message)

			Expect(mockWriter.Events).To(HaveLen(0))
		})
	})
})

func NewValueMetric(name string, value float64, unit string) *events.ValueMetric {
	return &events.ValueMetric{
		Name:  proto.String(name),
		Value: proto.Float64(value),
		Unit:  proto.String(unit),
	}
}
