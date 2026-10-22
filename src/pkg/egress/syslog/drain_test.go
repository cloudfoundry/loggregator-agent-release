package syslog_test

import (
	"fmt"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/egress/syslog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TNZGOV-11532: formatting a Drain must never expose its credentials.
var _ = Describe("Drain", func() {
	var drain syslog.Drain

	BeforeEach(func() {
		drain = syslog.Drain{
			Url: "syslog-tls://logs.example:6514",
			Credentials: syslog.Credentials{
				Cert: "-----BEGIN CERTIFICATE-----secret-cert-----END CERTIFICATE-----",
				Key:  "-----BEGIN PRIVATE KEY-----secret-key-----END PRIVATE KEY-----",
				CA:   "-----BEGIN CERTIFICATE-----secret-ca-----END CERTIFICATE-----",
			},
		}
	})

	It("formats as only its URL", func() {
		Expect(drain.String()).To(Equal("syslog-tls://logs.example:6514"))
	})

	DescribeTable("does not expose credentials with fmt verbs",
		func(format string) {
			out := fmt.Sprintf(format, drain)
			Expect(out).To(ContainSubstring("syslog-tls://logs.example:6514"))
			Expect(out).ToNot(ContainSubstring("PRIVATE KEY"))
			Expect(out).ToNot(ContainSubstring("secret"))
		},
		Entry("%s", "%s"),
		Entry("%v", "%v"),
		Entry("%+v", "%+v"),
	)

	It("redacts userinfo and query parameters from the URL", func() {
		drain.Url = "https://user:s3cret@logs.example/path?apikey=hunter2"
		Expect(drain.String()).To(Equal("https://logs.example/path"))
	})

	It("does not echo an unparseable URL", func() {
		drain.Url = "https://user:s3cret@logs.example:bad port/"
		Expect(drain.String()).ToNot(ContainSubstring("s3cret"))
		Expect(drain.String()).To(Equal("<unparseable drain url>"))
	})

	It("does not expose credentials when embedded in a Binding", func() {
		out := fmt.Sprintf("%v", syslog.Binding{AppId: "app-1", Drain: drain})
		Expect(out).ToNot(ContainSubstring("secret"))
	})
})
