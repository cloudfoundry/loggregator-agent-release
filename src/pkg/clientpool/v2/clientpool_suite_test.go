package v2_test

import (
	"log"
	"testing"

	"google.golang.org/grpc/grpclog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestClientpool(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	grpclog.SetLogger(log.New(GinkgoWriter, "", 0))

	RegisterFailHandler(Fail)
	RunSpecs(t, "Clientpool V2 Suite")
}
