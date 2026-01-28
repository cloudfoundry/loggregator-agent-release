package syslog_test

import (
	"bufio"

	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/loggregator"

	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	metricsHelpers "code.cloudfoundry.org/go-metric-registry/testhelpers"

	"code.cloudfoundry.org/go-loggregator/v10/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TCPWriter", func() {
	var (
		listener net.Listener
		binding  = &syslog.URLBinding{
			AppID:    "test-app-id",
			Hostname: "test-hostname",
		}
		netConf = syslog.NetworkTimeoutConfig{
			WriteTimeout: time.Second,
			DialTimeout:  100 * time.Millisecond,
		}
	)

	BeforeEach(func() {
		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:")
		Expect(err).ToNot(HaveOccurred())
		binding.URL, _ = url.Parse(fmt.Sprintf("syslog://%s", listener.Addr()))
	})

	AfterEach(func() {
		listener.Close()
	})

	Describe("Write()", func() {
		var (
			writer        egress.WriteCloser
			egressCounter *metricsHelpers.SpyMetric
		)

		BeforeEach(func() {
			var err error
			egressCounter = &metricsHelpers.SpyMetric{}
			factory := loggregator.NewAppLogStreamFactory()
			logClient := testhelper.NewSpyLogClient()
			emitter := factory.NewLogEmitter(logClient, "3")

			writer = syslog.NewTCPWriter(
				binding,
				netConf,
				egressCounter,
				syslog.NewConverter(),
				emitter,
			)
			Expect(err).ToNot(HaveOccurred())
		})

		DescribeTable("envelopes are written out with proper priority", func(logType loggregator_v2.Log_Type, expectedPriority int) {
			env := buildLogEnvelope("APP", "2", "just a test", logType)
			Expect(writer.Write(env)).To(Succeed())

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			buf := bufio.NewReader(conn)

			actual, err := buf.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())

			expected := fmt.Sprintf(`118 <%d>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [APP/2] - [tags@47450 source_type="APP"] just a test`+"\n", expectedPriority)
			Expect(actual).To(Equal(expected))
		},
			Entry("stdout", loggregator_v2.Log_OUT, 14),
			Entry("stderr", loggregator_v2.Log_ERR, 11),
			Entry("undefined-type", loggregator_v2.Log_Type(-20), -1),
		)

		DescribeTable("envelopes are written out with proper process id", func(sourceType, sourceInstance, expectedProcessID string, expectedLength int) {
			env := buildLogEnvelope(sourceType, sourceInstance, "just a test", loggregator_v2.Log_OUT)
			Expect(writer.Write(env)).To(Succeed())

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			buf := bufio.NewReader(conn)

			actual, err := buf.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())

			expected := fmt.Sprintf(`%d <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [%s] - [tags@47450 source_type="%s"] just a test`+"\n", expectedLength, expectedProcessID, sourceType)
			Expect(actual).To(Equal(expected))
		},
			Entry("app source type", "app/foo/bar", "26", "APP/FOO/BAR/26", 135),
			Entry("other source type", "other", "1", "OTHER/1", 122),
		)

		It("writes gauge metrics to the tcp drain", func() {
			env := buildGaugeEnvelope("1")
			Expect(writer.Write(env)).To(Succeed())

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			buf := bufio.NewReader(conn)

			var msgs []string
			for i := 0; i < 5; i++ {
				actual, err := buf.ReadString('\n')
				Expect(err).ToNot(HaveOccurred())

				msgs = append(msgs, actual)
			}

			Expect(msgs).To(ConsistOf(
				"128 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"cpu\" value=\"0.23\" unit=\"percentage\"] \n",
				"124 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"disk\" value=\"1234\" unit=\"bytes\"] \n",
				"130 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"disk_quota\" value=\"1024\" unit=\"bytes\"] \n",
				"126 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"memory\" value=\"5423\" unit=\"bytes\"] \n",
				"132 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [gauge@47450 name=\"memory_quota\" value=\"8000\" unit=\"bytes\"] \n",
			))
		})

		It("writes counter metrics to tcp drain", func() {
			env := buildCounterEnvelope("1")
			Expect(writer.Write(env)).To(Succeed())

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			buf := bufio.NewReader(conn)

			actual, err := buf.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())

			Expect(actual).To(Equal(
				"129 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [1] - [counter@47450 name=\"some-counter\" total=\"99\" delta=\"1\"] \n",
			))
		})

		It("strips null termination char from message", func() {
			env := buildLogEnvelope("OTHER", "1", "no null `\x00` please", loggregator_v2.Log_OUT)
			Expect(writer.Write(env)).To(Succeed())

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			buf := bufio.NewReader(conn)

			actual, err := buf.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())

			expected := "128 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [OTHER/1] - [tags@47450 source_type=\"OTHER\"] no null `` please\n"
			Expect(actual).To(Equal(expected))
		})

		It("emits an egress metric for each message", func() {
			env := buildLogEnvelope("OTHER", "1", "no null `\x00` please", loggregator_v2.Log_OUT)
			err := writer.Write(env)
			Expect(err).To(BeNil())

			Expect(egressCounter.Value()).To(BeNumerically("==", 1))
		})

		It("replaces spaces with dashes in the process ID", func() {
			env := buildLogEnvelope("MY TASK", "2", "just a test", loggregator_v2.Log_OUT)
			Expect(writer.Write(env)).To(Succeed())

			conn, err := listener.Accept()
			Expect(err).ToNot(HaveOccurred())
			buf := bufio.NewReader(conn)

			actual, err := buf.ReadString('\n')
			Expect(err).ToNot(HaveOccurred())

			Expect(actual).To(Equal(
				`126 <14>1 1970-01-01T00:00:00.012345+00:00 test-hostname test-app-id [MY-TASK/2] - [tags@47450 source_type="MY TASK"] just a test` + "\n",
			))
		})
	})

	Describe("when write fails to connect", func() {
		It("write returns an error", func() {
			env := buildLogEnvelope("APP", "2", "just a test", loggregator_v2.Log_OUT)
			binding.URL, _ = url.Parse("syslog://localhost-garbage:9999")
			factory := loggregator.NewAppLogStreamFactory()
			logClient := testhelper.NewSpyLogClient()
			emitter := factory.NewLogEmitter(logClient, "3")

			writer := syslog.NewTCPWriter(
				binding,
				netConf,
				&metricsHelpers.SpyMetric{},
				syslog.NewConverter(),
				emitter,
			)

			errs := make(chan error, 1)
			go func() {
				errs <- writer.Write(env)
			}()
			Eventually(errs).Should(Receive(HaveOccurred()))
		})
	})

	Describe("Cancel Context", func() {
		var (
			writer egress.WriteCloser
			conn   net.Conn
		)

		Context("with a happy dialer", func() {
			BeforeEach(func() {
				var err error
				factory := loggregator.NewAppLogStreamFactory()
				logClient := testhelper.NewSpyLogClient()
				emitter := factory.NewLogEmitter(logClient, "3")
				writer = syslog.NewTCPWriter(
					binding,
					netConf,
					&metricsHelpers.SpyMetric{},
					syslog.NewConverter(),
					emitter,
				)
				Expect(err).ToNot(HaveOccurred())

				By("writing to establish connection")
				logEnv := buildLogEnvelope("APP", "2", "just a test", loggregator_v2.Log_OUT)
				err = writer.Write(logEnv)
				Expect(err).ToNot(HaveOccurred())

				conn, err = listener.Accept()
				Expect(err).ToNot(HaveOccurred())

				b := make([]byte, 256)
				_, err = conn.Read(b)
				Expect(err).ToNot(HaveOccurred())
			})

			It("closes the writer connection", func() {
				Expect(writer.Close()).To(Succeed())

				b := make([]byte, 256)
				_, err := conn.Read(b)
				Expect(err).To(Equal(io.EOF))
			})
		})
	})
})

func buildLogEnvelope(srcType, srcInstance, payload string, logType loggregator_v2.Log_Type) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Tags: map[string]string{
			"source_type": srcType,
		},
		InstanceId: srcInstance,
		Timestamp:  12345678,
		SourceId:   "test-app-id",
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte(payload),
				Type:    logType,
			},
		},
	}
}

func buildGaugeEnvelope(srcInstance string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		InstanceId: srcInstance,
		Timestamp:  12345678,
		SourceId:   "test-app-id",
		Message: &loggregator_v2.Envelope_Gauge{
			Gauge: &loggregator_v2.Gauge{
				Metrics: map[string]*loggregator_v2.GaugeValue{
					"cpu": {
						Unit:  "percentage",
						Value: 0.23,
					},
					"disk": {
						Unit:  "bytes",
						Value: 1234.0,
					},
					"disk_quota": {
						Unit:  "bytes",
						Value: 1024.0,
					},
					"memory": {
						Unit:  "bytes",
						Value: 5423.0,
					},
					"memory_quota": {
						Unit:  "bytes",
						Value: 8000.0,
					},
				},
			},
		},
	}
}

func buildTimerEnvelope(srcInstance string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Timestamp:  12345678,
		SourceId:   "test-app-id",
		InstanceId: srcInstance,
		Message: &loggregator_v2.Envelope_Timer{
			Timer: &loggregator_v2.Timer{
				Name:  "http",
				Start: 10,
				Stop:  20,
			},
		},
	}
}

func buildEventEnvelope(srcInstance string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Timestamp:  12345678,
		SourceId:   "test-app-id",
		InstanceId: srcInstance,
		Message: &loggregator_v2.Envelope_Event{
			Event: &loggregator_v2.Event{
				Title: "event-title",
				Body:  "event-body",
			},
		},
	}
}

func buildCounterEnvelope(srcInstance string) *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Timestamp:  12345678,
		SourceId:   "test-app-id",
		InstanceId: srcInstance,
		Message: &loggregator_v2.Envelope_Counter{
			Counter: &loggregator_v2.Counter{
				Name:  "some-counter",
				Total: 99,
				Delta: 1,
			},
		},
	}
}
