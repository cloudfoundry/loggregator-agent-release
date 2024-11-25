package syslog_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Loggregator Emitter", func() {
	Describe("WriteLog()", func() {
		It("emits a log message", func() {
			logClient := NewSpyLogClient()
			emitter := syslog.NewLoggregatorEmitter(logClient, "0")

			emitter.WriteLog("app-id", "some-message")

			messages := logClient.message()
			appIDs := logClient.appID()
			sourceTypes := logClient.sourceType()
			Expect(messages).To(HaveLen(2))
			Expect(messages[0]).To(Equal("some-message"))
			Expect(messages[1]).To(Equal("some-message"))
			Expect(appIDs[0]).To(Equal("app-id"))
			Expect(appIDs[1]).To(Equal("app-id"))
			Expect(sourceTypes).To(HaveKey("LGR"))
			Expect(sourceTypes).To(HaveKey("SYS"))
		})

		It("does not emit a log message if the appID is empty", func() {
			logClient := NewSpyLogClient()
			emitter := syslog.NewLoggregatorEmitter(logClient, "0")

			emitter.WriteLog("", "some-message")

			messages := logClient.message()
			appIDs := logClient.appID()
			sourceTypes := logClient.sourceType()
			Expect(messages).To(HaveLen(0))
			Expect(appIDs).To(HaveLen(0))
			Expect(sourceTypes).ToNot(HaveKey("LGR"))
			Expect(sourceTypes).ToNot(HaveKey("SYS"))
		})
	})
})
