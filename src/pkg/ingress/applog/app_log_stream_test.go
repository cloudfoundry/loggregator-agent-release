package applog_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/ingress/applog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Loggregator Egress", func() {
	Describe("Log Stream", func() {
		It("should emit a log message for an application with LGR and SYS source type with the provided app-ID", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := applog.NewAppLogStreamFactory()
			logStream := factory.NewAppLogStream(logClient, "0")

			logStream.Emit("some-message", "app-id")

			messages := logClient.Message()
			appIDs := logClient.AppID()
			sourceTypes := logClient.SourceType()
			Expect(messages).To(HaveLen(2))
			Expect(messages[0]).To(Equal("some-message"))
			Expect(messages[1]).To(Equal("some-message"))
			Expect(appIDs[0]).To(Equal("app-id"))
			Expect(appIDs[1]).To(Equal("app-id"))
			Expect(sourceTypes).To(HaveKey("LGR"))
			Expect(sourceTypes).To(HaveKey("SYS"))
		})

		It("should not emit a log message when the app-ID is empty", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := applog.NewAppLogStreamFactory()
			logStream := factory.NewAppLogStream(logClient, "0")

			logStream.Emit("some-message", "")

			Expect(logClient.Message()).To(BeEmpty())
		})
	})

	Describe("DefaultLogStreamFactory", func() {
		It("should produce a AppLogStream which emits to the provided LogClient", func() {
			factory := applog.NewAppLogStreamFactory()
			logClient := testhelper.NewSpyLogClient()
			sourceIndex := "test-index"

			logStream := factory.NewAppLogStream(logClient, sourceIndex)
			logStream.Emit("some-message", "app-id")

			messages := logClient.Message()
			appIDs := logClient.AppID()
			sourceTypes := logClient.SourceType()
			sourceInstance := logClient.SourceInstance()
			Expect(messages).To(HaveLen(2))
			Expect(messages[0]).To(Equal("some-message"))
			Expect(messages[1]).To(Equal("some-message"))
			Expect(appIDs[0]).To(Equal("app-id"))
			Expect(appIDs[1]).To(Equal("app-id"))
			Expect(sourceTypes).To(HaveKey("LGR"))
			Expect(sourceTypes).To(HaveKey("SYS"))
			Expect(sourceInstance).To(HaveKey(""))
			Expect(sourceInstance).To(HaveKey("test-index"))
		})
	})
})
