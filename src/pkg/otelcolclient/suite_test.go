package otelcolclient_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOtelColClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OTel Collector Client Suite")
}
