package timeoutwaitgroup_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestTimeoutWaitGroup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TimeoutWaitGroup Suite")
}
