package plumbing_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/internal/testhelper"

	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/plumbing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TLS", func() {
	loggregatorTestCerts := testhelper.GenerateCerts("loggregatorCA")

	Context("NewClientCredentials", func() {
		It("returns transport credentials", func() {
			creds, err := plumbing.NewClientCredentials(
				loggregatorTestCerts.Cert("doppler"),
				loggregatorTestCerts.Key("doppler"),
				loggregatorTestCerts.CA(),
				"doppler",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(creds).ToNot(BeNil())
		})

		It("returns an error with invalid certs", func() {
			creds, err := plumbing.NewClientCredentials(
				loggregatorTestCerts.Cert("doppler"),
				loggregatorTestCerts.Key("doppler"),
				loggregatorTestCerts.Key("doppler"),
				"doppler",
			)
			Expect(err).To(HaveOccurred())
			Expect(creds).To(BeNil())
		})
	})

	Context("NewServerCredentials", func() {
		It("returns transport credentials", func() {
			_, err := plumbing.NewServerCredentials(
				loggregatorTestCerts.Cert("doppler"),
				loggregatorTestCerts.Key("doppler"),
				loggregatorTestCerts.CA(),
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error with invalid certs", func() {
			creds, err := plumbing.NewServerCredentials(
				loggregatorTestCerts.Cert("doppler"),
				loggregatorTestCerts.Key("doppler"),
				loggregatorTestCerts.Key("doppler"),
			)
			Expect(err).To(HaveOccurred())
			Expect(creds).To(BeNil())
		})
	})
})
