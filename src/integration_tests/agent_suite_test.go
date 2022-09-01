package agent_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"google.golang.org/grpc/grpclog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestAgent(t *testing.T) {
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(GinkgoWriter, GinkgoWriter, GinkgoWriter))

	// grpclog.SetLogger(log.New(GinkgoWriter, "", log.LstdFlags))
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent Integration Test Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	bp, err := Build()
	Expect(err).ToNot(HaveOccurred())
	text, err := bp.Marshal()
	Expect(err).ToNot(HaveOccurred())
	return text
}, func(bpText []byte) {
	var bp BuildPaths
	err := bp.Unmarshal(bpText)
	Expect(err).ToNot(HaveOccurred())
	bp.SetEnv()
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	Cleanup()
})

type BuildPaths struct {
	Agent string `json:"agent"`
}

func (bp BuildPaths) Marshal() ([]byte, error) {
	return json.Marshal(bp)
}

func (bp *BuildPaths) Unmarshal(text []byte) error {
	return json.Unmarshal(text, bp)
}

func (bp BuildPaths) SetEnv() {
	os.Setenv("AGENT_BUILD_PATH", bp.Agent)
}

func Build() (BuildPaths, error) {
	var bp BuildPaths

	if os.Getenv("SKIP_BUILD") != "" {
		fmt.Println("Skipping building of binaries")
		bp.Agent = os.Getenv("AGENT_BUILD_PATH")
		return bp, nil
	}

	path, err := gexec.Build("code.cloudfoundry.org/loggregator-agent-release/src/cmd/loggregator-agent", "-race")
	if err != nil {
		return bp, err
	}

	bp.Agent = path

	return bp, nil
}

func Cleanup() {
	gexec.CleanupBuildArtifacts()
}
