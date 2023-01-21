package app_test

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.cloudfoundry.org/go-loggregator/v9"
	"code.cloudfoundry.org/go-loggregator/v9/rpc/loggregator_v2"
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestApp(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Forwarder Agent App Suite")
}

func protoEqual(expected interface{}) types.GomegaMatcher {
	return &protoMatcher{
		expected: expected,
	}
}

type protoMatcher struct {
	expected interface{}
}

func (pm *protoMatcher) Match(actual interface{}) (success bool, err error) {
	msg1, ok := actual.(protoreflect.ProtoMessage)
	if !ok {
		return false, fmt.Errorf("protoEqual matcher expects an protobuf message")
	}
	msg2, ok := pm.expected.(protoreflect.ProtoMessage)
	if !ok {
		return false, fmt.Errorf("Failed to convert to a protobuf message: %#v", msg2)
	}
	return proto.Equal(msg1, msg2), nil
}

func (pm *protoMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nto be the protobuf equal of\n\t%#v", actual, pm.expected)
}

func (pm *protoMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%#v\nnot to be the protobuf equal of\n\t%#v", actual, pm.expected)
}

func newIngressClient(port int, testCerts *testhelper.TestCerts, batchSize uint) *loggregator.IngressClient {
	tlsConfig, err := loggregator.NewIngressTLSConfig(
		testCerts.CA(),
		testCerts.Cert("metron"),
		testCerts.Key("metron"),
	)
	Expect(err).ToNot(HaveOccurred())
	ingressClient, err := loggregator.NewIngressClient(
		tlsConfig,
		loggregator.WithAddr(fmt.Sprintf("127.0.0.1:%d", port)),
		loggregator.WithLogger(log.New(GinkgoWriter, "[TEST INGRESS CLIENT] ", 0)),
		loggregator.WithBatchMaxSize(batchSize),
	)
	Expect(err).ToNot(HaveOccurred())
	return ingressClient
}

const configTempl = `---
ingress: %s
`

type spyLoggregatorV2Ingress struct {
	loggregator_v2.UnimplementedIngressServer

	blocking bool

	addr      string
	srv       *grpc.Server
	close     func()
	envelopes chan *loggregator_v2.Envelope
}

func startSpyLoggregatorV2Ingress(testCerts *testhelper.TestCerts, commonName string, cfgPath string) *spyLoggregatorV2Ingress {
	s := &spyLoggregatorV2Ingress{
		envelopes: make(chan *loggregator_v2.Envelope, 10000),
	}

	serverCreds, err := plumbing.NewServerCredentials(
		testCerts.Cert(commonName),
		testCerts.Key(commonName),
		testCerts.CA(),
	)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	lis, err := net.Listen("tcp", "127.0.0.1:")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	s.srv = grpc.NewServer(grpc.Creds(serverCreds))
	loggregator_v2.RegisterIngressServer(s.srv, s)

	s.close = func() {
		s.srv.Stop()
		_ = lis.Close()
	}

	s.addr = lis.Addr().String()
	port := strings.Split(s.addr, ":")

	dir, err := os.MkdirTemp(cfgPath, "")
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	tmpfn := filepath.Join(dir, "ingress_port.yml")
	tmpfn, err = filepath.Abs(tmpfn)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	contents := fmt.Sprintf(configTempl, port[len(port)-1])
	err = os.WriteFile(tmpfn, []byte(contents), 0600)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	go s.srv.Serve(lis) // nolint:errcheck

	return s
}

func (s *spyLoggregatorV2Ingress) Sender(loggregator_v2.Ingress_SenderServer) error {
	panic("not implemented")
}

func (s *spyLoggregatorV2Ingress) Send(context.Context, *loggregator_v2.EnvelopeBatch) (*loggregator_v2.SendResponse, error) {
	panic("not implemented")
}

func (s *spyLoggregatorV2Ingress) BatchSender(srv loggregator_v2.Ingress_BatchSenderServer) error {
	for {
		batch, err := srv.Recv()
		if err != nil {
			return err
		}

		for _, e := range batch.Batch {
			s.envelopes <- e
		}

		if s.blocking {
			time.Sleep(2 * time.Second)
		}
	}
}

var sampleEnvelope = &loggregator_v2.Envelope{
	Timestamp: time.Now().UnixNano(),
	SourceId:  "some-id",
	Message: &loggregator_v2.Envelope_Log{
		Log: &loggregator_v2.Log{
			Payload: []byte("hello"),
		},
	},
	Tags: map[string]string{
		"some-tag": "some-value",
	},
}

var sampleCounter = &loggregator_v2.Envelope{
	Timestamp: time.Now().UnixNano(),
	SourceId:  "some-id",
	Message: &loggregator_v2.Envelope_Counter{
		Counter: &loggregator_v2.Counter{
			Delta: 20,
			Total: 0,
		},
	},
}

func MakeSampleBigEnvelope() *loggregator_v2.Envelope {
	return &loggregator_v2.Envelope{
		Timestamp: time.Now().UnixNano(),
		SourceId:  "some-id",
		Message: &loggregator_v2.Envelope_Log{
			Log: &loggregator_v2.Log{
				Payload: []byte(strings.Repeat("A", 61440)),
			},
		},
		Tags: map[string]string{
			"some-tag": "some-value",
		},
	}
}
