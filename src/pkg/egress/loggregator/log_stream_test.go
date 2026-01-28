package loggregator_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/loggregator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Loggregator Emitter", func() {
	Describe("DefaultAppLogEmitter", func() {
		It("emits a log message", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := loggregator.NewAppLogStreamFactory()
			emitter := factory.NewLogEmitter(logClient, "0")

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

		It("does not emit a log message if the appIdentifier is empty", func() {
			logClient := testhelper.NewSpyLogClient()
			factory := loggregator.NewAppLogStreamFactory()
			emitter := factory.NewLogEmitter(logClient, "0")

			emitter.Emit("some-message", loggregator.ForPlatform())

			messages := logClient.Message()
			appIDs := logClient.AppID()
			sourceTypes := logClient.SourceType()
			Expect(messages).To(HaveLen(0))
			Expect(appIDs).To(HaveLen(0))
			Expect(sourceTypes).ToNot(HaveKey("LGR"))
			Expect(sourceTypes).ToNot(HaveKey("SYS"))
		})
	})

	Describe("DefaultLogEmitterFactory", func() {
		It("produces a LogStream", func() {
			factory := loggregator.NewAppLogStreamFactory()
			logClient := testhelper.NewSpyLogClient()
			sourceIndex := "test-index"

			emitter := factory.NewLogEmitter(logClient, sourceIndex)
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
