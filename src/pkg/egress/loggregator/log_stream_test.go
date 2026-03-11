package loggregator_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/loggregator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Loggregator Egress", func() {
	Describe("Log Stream", func() {
		It("should emit a log message for an application with LGR and SYS source type with the provided app-ID", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := loggregator.NewAppLogStreamFactory()
			emitter := factory.NewLogStream(logClient, "0")

			emitter.Emit("some-message", loggregator.ForApp("app-id"))

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

		It("should emit a log message for the platform with LGR and SYS source type without an app-ID", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := loggregator.NewAppLogStreamFactory()
			emitter := factory.NewLogStream(logClient, "0")

			emitter.Emit("some-message", loggregator.ForPlatform())

			messages := logClient.Message()
			appIDs := logClient.AppID()
			sourceTypes := logClient.SourceType()
			Expect(messages).To(HaveLen(2))
			Expect(messages[0]).To(Equal("some-message"))
			Expect(messages[1]).To(Equal("some-message"))
			Expect(appIDs[0]).To(Equal(""))
			Expect(appIDs[1]).To(Equal(""))
			Expect(sourceTypes).To(HaveKey("LGR"))
			Expect(sourceTypes).To(HaveKey("SYS"))
		})
	})

	Describe("DefaultLogEmitterFactory", func() {
		It("should produce a LogStream which emits to the provided LogClient", func() {
			factory := loggregator.NewAppLogStreamFactory()
			logClient := testhelper.NewSpyLogClient()
			sourceIndex := "test-index"

			emitter := factory.NewLogStream(logClient, sourceIndex)
			emitter.Emit("some-message", loggregator.ForApp("app-id"))

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
