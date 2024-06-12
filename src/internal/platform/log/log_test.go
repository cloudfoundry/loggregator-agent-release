package log_test

import (
	"bytes"

	"code.cloudfoundry.org/loggregator-agent-release/src/internal/platform/log"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Logger", func() {
	var (
		logr *log.Logger
		buf  *bytes.Buffer
	)

	BeforeEach(func() {
		logr = log.New()

		buf = new(bytes.Buffer)
		logr.SetOutput(buf)
		logr.SetLevel(log.DebugLevel)
	})

	Context("Debugf", func() {
		BeforeEach(func() {
			logr = logr.Session("debug-session")
		})

		It("should log", func() {
			logr.Debugf("testing")
			Expect(buf.String()).To(ContainSubstring("DEBUG debug-session: testing\n"))
		})
	})

	Context("Infof", func() {
		BeforeEach(func() {
			logr = logr.Session("info-session")
		})

		It("should log", func() {
			logr.Infof("testing")
			Expect(buf.String()).To(ContainSubstring("INFO info-session: testing\n"))
		})
	})

	Context("Warnf", func() {
		BeforeEach(func() {
			logr = logr.Session("warn-session")
		})

		It("should log", func() {
			logr.Warnf("testing")
			Expect(buf.String()).To(ContainSubstring("WARN warn-session: testing\n"))
		})
	})

	Context("Errorf", func() {
		BeforeEach(func() {
			logr = logr.Session("error-session")
		})

		It("should log", func() {
			logr.Errorf("testing")
			Expect(buf.String()).To(ContainSubstring("ERROR error-session: testing\n"))
		})
	})

	Context("Fatalf", func() {
		BeforeEach(func() {
			logr = logr.Session("fatal-session")
		})

		It("should log and panic", func() {
			Expect(func() { logr.Fatalf("testing") }).To(Panic())
			Expect(buf.String()).To(ContainSubstring("FATAL fatal-session: testing\n"))
		})
	})
})
