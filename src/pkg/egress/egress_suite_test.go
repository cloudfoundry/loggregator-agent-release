package egress_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestEgress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Egress Suite")
}
