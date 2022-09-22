package clientpool_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestClientpool(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clientpool Suite")
}
