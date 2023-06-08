package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var otelCollectorManagerPath string

func TestApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OTel Collector Manager Suite")
}

var _ = BeforeSuite(func() {
	var err error
	otelCollectorManagerPath, err = gexec.Build("code.cloudfoundry.org/loggregator-agent-release/src/cmd/otel-collector-manager")
	Expect(err).ShouldNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
