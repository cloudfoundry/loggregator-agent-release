package simplecache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSimpleCache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SimpleCache Suite")
}
