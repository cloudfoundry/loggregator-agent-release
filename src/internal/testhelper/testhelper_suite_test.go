package testhelper_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestTesthelper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testhelper Suite")
}
