package agent_test

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/tlsconfig"

	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testservers"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	"github.com/cloudfoundry/dropsonde/emitter"
	"github.com/cloudfoundry/sonde-go/events"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/proto"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const eventuallyTimeout = 10 * time.Second

var _ = Describe("Agent", func() {
	var testCerts = testhelper.GenerateCerts("loggregatorCA")

	It("accepts connections on the v1 API", func() {
		consumerServer, err := NewServer(testCerts)
		Expect(err).ToNot(HaveOccurred())
		defer consumerServer.Stop()
		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("127.0.0.1", consumerServer.Port(), testCerts),
		)
		defer agentCleanup()

		udpEmitter, err := emitter.NewUdpEmitter(fmt.Sprintf("127.0.0.1:%d", agentPorts.UDP))
		Expect(err).ToNot(HaveOccurred())
		eventEmitter := emitter.NewEventEmitter(udpEmitter, "some-origin")

		done := make(chan struct{})
		go func() {
			event := &events.CounterEvent{
				Name:  proto.String("some-name"),
				Delta: proto.Uint64(5),
				Total: proto.Uint64(6),
			}

			for {
				select {
				case <-done:
					return
				default:
					eventEmitter.Emit(event)
				}
			}
		}()
		defer close(done)

		var rx plumbing.DopplerIngestor_PusherServer
		Eventually(consumerServer.V1.PusherInput.Arg0, eventuallyTimeout).Should(Receive(&rx))

		e := make(chan *plumbing.EnvelopeData)
		go func() {
			for {
				data, err := rx.Recv()
				if err != nil {
					return
				}
				e <- data
			}
		}()

		var data *plumbing.EnvelopeData
		Eventually(e, eventuallyTimeout).Should(Receive(&data))

		envelope := new(events.Envelope)
		Expect(proto.Unmarshal(data.Payload, envelope)).To(Succeed())
	})

	It("accepts connections on the v2 API", func() {
		consumerServer, err := NewServer(testCerts)
		Expect(err).ToNot(HaveOccurred())
		defer consumerServer.Stop()

		agentCleanup, agentPorts := testservers.StartAgent(
			testservers.BuildAgentConfig("127.0.0.1", consumerServer.Port(), testCerts),
		)
		defer agentCleanup()

		emitEnvelope := &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Log{
				Log: &loggregator_v2.Log{
					Payload: []byte("some-message"),
					Type:    loggregator_v2.Log_OUT,
				},
			},
		}

		client := agentClient(agentPorts.GRPC, testCerts)
		sender, err := client.Sender(context.Background())
		Expect(err).ToNot(HaveOccurred())

		go func() {
			for range time.Tick(time.Nanosecond) {
				sender.Send(emitEnvelope)
			}
		}()

		var rx loggregator_v2.Ingress_BatchSenderServer
		numDopplerConnections := 5
		for i := 0; i < numDopplerConnections; i++ {
			Eventually(consumerServer.V2.BatchSenderInput.Arg0, eventuallyTimeout).Should(Receive(&rx))
			consumerServer.V2.BatchSenderOutput.Ret0 <- nil
		}
		Eventually(consumerServer.V2.BatchSenderInput.Arg0, eventuallyTimeout).Should(Receive(&rx))

		var envBatch *loggregator_v2.EnvelopeBatch
		var idx int
		f := func() *loggregator_v2.Envelope {
			batch, err := rx.Recv()
			Expect(err).ToNot(HaveOccurred())

			defer func() { envBatch = batch }()

			for i, envelope := range batch.Batch {
				if envelope.GetLog() != nil {
					idx = i
					return envelope
				}
			}

			return nil
		}
		Eventually(f, 10).ShouldNot(BeNil())

		Expect(len(envBatch.Batch)).ToNot(BeZero())

		Expect(proto.Equal(envBatch.Batch[idx].GetLog(), &loggregator_v2.Log{Payload: []byte("some-message")}))
		Expect(envBatch.Batch[idx].Tags).To(Equal(map[string]string{
			"auto-tag-1": "auto-tag-value-1",
			"auto-tag-2": "auto-tag-value-2",
		}))
	})

	It("does not emit logs when LOGS_DISABLED is set to true", func() {
		consumerServer, err := NewServer(testCerts)
		Expect(err).ToNot(HaveOccurred())
		defer consumerServer.Stop()

		config := testservers.BuildAgentConfig("127.0.0.1", consumerServer.Port(), testCerts)
		config.LogsDisabled = true
		agentCleanup, agentPorts := testservers.StartAgent(config)
		defer agentCleanup()

		logEnvelope := &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Log{},
		}
		metricEnvelope := &loggregator_v2.Envelope{
			Message: &loggregator_v2.Envelope_Counter{},
		}

		client := agentClient(agentPorts.GRPC, testCerts)
		sender, err := client.Sender(context.Background())
		Expect(err).ToNot(HaveOccurred())

		go func() {
			for range time.Tick(time.Nanosecond) {
				sender.Send(logEnvelope)
				sender.Send(metricEnvelope)
			}
		}()

		var rx loggregator_v2.Ingress_BatchSenderServer
		numDopplerConnections := 5
		for i := 0; i < numDopplerConnections; i++ {
			Eventually(consumerServer.V2.BatchSenderInput.Arg0, eventuallyTimeout).Should(Receive(&rx))
			consumerServer.V2.BatchSenderOutput.Ret0 <- nil
		}
		Eventually(consumerServer.V2.BatchSenderInput.Arg0, eventuallyTimeout).Should(Receive(&rx))

		batch, err := rx.Recv()
		Expect(err).ToNot(HaveOccurred())

		for _, envelope := range batch.Batch {
			Expect(envelope.GetLog()).To(BeNil())
		}
	})
})

func agentClient(port int, testCerts *testhelper.TestCerts) loggregator_v2.IngressClient {
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(
			testCerts.Cert("metron"),
			testCerts.Key("metron"),
		),
	).Client(
		tlsconfig.WithAuthorityFromFile(testCerts.CA()),
		tlsconfig.WithServerName("metron"),
	)

	if err != nil {
		panic(err)
	}

	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		panic(err)
	}
	return loggregator_v2.NewIngressClient(conn)
}
