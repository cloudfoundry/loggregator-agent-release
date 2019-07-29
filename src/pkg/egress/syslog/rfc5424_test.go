package syslog_test

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/go-loggregator/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/syslog"
)

var _ = Describe("RFC5424", func() {
	It("converts a log envelope to a slice of slice of byte in RFC5424 format", func() {
		env := buildLogEnvelope("MY TASK", "2", "just a test", loggregator_v2.Log_OUT)

		Expect(syslog.ToRFC5424(env, "test-hostname")).To(Equal([][]byte{
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [MY-TASK/2] - [tags@47450 source_type=\"MY TASK\"] just a test\n"),
		}))
	})

	It("uses the correct priority for STDERR", func() {
		env := buildLogEnvelope("MY TASK", "2", "just a test", loggregator_v2.Log_ERR)

		Expect(syslog.ToRFC5424(env, "test-hostname")).To(Equal([][]byte{
			[]byte("<11>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [MY-TASK/2] - [tags@47450 source_type=\"MY TASK\"] just a test\n"),
		}))
	})

	It("uses the correct priority for unknown log type", func() {
		env := buildLogEnvelope("MY TASK", "2", "just a test", 20)

		Expect(syslog.ToRFC5424(env, "test-hostname")).To(Equal([][]byte{
			[]byte("<-1>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [MY-TASK/2] - [tags@47450 source_type=\"MY TASK\"] just a test\n"),
		}))
	})

	It("converts a gauge envelope to a slice of slice of byte in RFC5424 format", func() {
		env := buildGaugeEnvelope("1")

		result, err := syslog.ToRFC5424(env, "test-hostname")

		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ConsistOf(
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"cpu\" value=\"0.23\" unit=\"percentage\"] \n"),
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"disk\" value=\"1234\" unit=\"bytes\"] \n"),
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"disk_quota\" value=\"1024\" unit=\"bytes\"] \n"),
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"memory\" value=\"5423\" unit=\"bytes\"] \n"),
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"memory_quota\" value=\"8000\" unit=\"bytes\"] \n"),
		))
	})

	It("converts a counter envelope to a slice of slice of byte in RFC5424 format", func() {
		env := buildCounterEnvelope("1")

		Expect(syslog.ToRFC5424(env, "test-hostname")).To(Equal([][]byte{
			[]byte("<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [counter@47450 name=\"some-counter\" total=\"99\" delta=\"1\"] \n"),
		}))
	})

	It("converts a timer envelope to a slice of slice of byte in RFC5424 format", func() {
		env := buildTimerEnvelope("1")

		Expect(syslog.ToRFC5424(env, "test-hostname")).To(Equal([][]byte{
			[]byte(`<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [timer@47450 name="http" start="10" stop="20"] ` + "\n"),
		}))
	})

	It("converts an event envelope to a slice of slice of byte in RFC5424 format", func() {
		env := buildEventEnvelope("1")

		Expect(syslog.ToRFC5424(env, "test-hostname")).To(Equal([][]byte{
			[]byte(`<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [event@47450 title="event-title" body="event-body"] ` + "\n"),
		}))
	})

	It("converts tags to the RFC5424 format", func() {
		logEnv := buildLogEnvelope("MY TASK", "2", "just a test", loggregator_v2.Log_ERR)
		logEnv.Tags["log-tag"] = "oyster"

		metricEnv := buildCounterEnvelope("1")
		metricEnv.Tags = map[string]string{"metric-tag": "scallop"}

		receivedMsgs, _ := syslog.ToRFC5424(logEnv, "test-hostname")
		expectConversion(receivedMsgs, `<11>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [MY-TASK/2] - [tags@47450 log-tag="oyster" source_type="MY TASK"] just a test`+"\n")

		receivedMsgs, _ = syslog.ToRFC5424(metricEnv, "test-hostname")
		expectConversion(receivedMsgs, `<14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [counter@47450 name="some-counter" total="99" delta="1"][tags@47450 metric-tag="scallop"] `+"\n")
	})

	Describe("validation", func() {
		It("returns an error if hostname is longer than 255", func() {
			env := buildLogEnvelope("MY TASK", "2", "just a test", 20)
			_, err := syslog.ToRFC5424(env, invalidHostname)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error if app name is longer than 48", func() {
			env := buildLogEnvelope("MY TASK", "2", "just a test", 20)
			env.SourceId = invalidAppName
			_, err := syslog.ToRFC5424(env, "test-hostname")
			Expect(err).To(HaveOccurred())
		})

		It("returns an error if process ID is longer than 128", func() {
			env := buildLogEnvelope("MY TASK", invalidProcessID, "just a test", 20)
			_, err := syslog.ToRFC5424(env, "test-hostname")
			Expect(err).To(HaveOccurred())
		})
	})
})

func expectConversion(received [][]byte, expected string) bool {
	return Expect(received).To(Equal([][]byte{[]byte(expected)}), fmt.Sprintf("\n%s\n%s", string(received[0]), expected))
}

var (
	invalidHostname  = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Cras tortor elit, ultricies in suscipit et, euismod quis velit. Duis et auctor mauris. Suspendisse et aliquet justo. Nunc fermentum lorem dolor, eu fermentum quam vulputate id. Morbi gravida ut elit sed."
	invalidAppName   = "Lorem ipsum dolor sit amet, consectetur posuere. HA!"
	invalidProcessID = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Cras tortor elit, ultricies in suscipit et, euismod quis velit. Duis et auctor mauris. Suspendisse et aliquet justo. Nunc fermentum lorem dolor,"
)
