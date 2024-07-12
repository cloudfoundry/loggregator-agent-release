package simplecache_test

import (
	"code.cloudfoundry.org/loggregator-agent-release/src/pkg/simplecache"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SimpleCache", func() {
	var cache *simplecache.SimpleCache[string, string]

	BeforeEach(func() {
		cache = simplecache.New[string, string](3 * time.Millisecond)
	})

	It("handles set and get", func() {
		cache.Set("key1", "val1")
		value, exists := cache.Get("key1")

		Expect(exists).To(BeTrue())
		Expect(value).To(Equal("val1"))
	})

	It("updates existing entries", func() {
		cache.Set("key1", "val1")
		value, _ := cache.Get("key1")
		Expect(value).To(Equal("val1"))

		cache.Set("key1", "val2")
		value, _ = cache.Get("key1")
		Expect(value).To(Equal("val2"))
	})

	It("handles non-existent entries", func() {
		_, exists := cache.Get("nonexist")
		Expect(exists).To(BeFalse())
	})

	It("expires entries", func() {
		cache.Set("key1", "val1")
		Eventually(func() bool {
			_, exists := cache.Get("key1")
			return exists
		}).WithTimeout(6 * time.Millisecond).WithPolling(time.Millisecond).Should(BeFalse())
	})

	It("handles concurrent access", func() {
		done := make(chan bool)

		go func() {
			for i := 0; i < 1000; i++ {
				cache.Set("key1", "val1")
				cache.Get("key1")
			}
			done <- true
		}()

		go func() {
			for i := 0; i < 1000; i++ {
				cache.Set("key2", "val2")
				cache.Get("key2")
			}
			done <- true
		}()
		<-done
		<-done
	})
})
