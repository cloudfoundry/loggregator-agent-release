package prom_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestProm(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prom Suite")
}
