package v2_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	v2 "code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("App Log Client", func() {
	Describe("Log Stream", func() {
		It("should emit a log message for an application with LGR source type with the provided app-ID", func() {
			logClient := testhelper.NewSpyLogClient()

			v2.EmitAppLog(logClient, "some-message", "app-id")

			messages := logClient.Message()
			appIDs := logClient.AppID()
			sourceTypes := logClient.SourceType()
			Expect(messages).To(HaveLen(1))
			Expect(messages[0]).To(Equal("some-message"))
			Expect(appIDs[0]).To(Equal("app-id"))
			Expect(sourceTypes).To(HaveKey("LGR"))
		})

		It("should not emit a log message when the logClient is empty", func() {
			logClient := testhelper.NewSpyLogClient()
			v2.EmitAppLog(nil, "some-message", "app-id")
			Expect(logClient.Message()).To(BeEmpty())
		})

		It("should not emit a log message when the app-ID is empty", func() {
			logClient := testhelper.NewSpyLogClient()
			v2.EmitAppLog(nil, "some-message", "")
			Expect(logClient.Message()).To(BeEmpty())
		})
	})

})
