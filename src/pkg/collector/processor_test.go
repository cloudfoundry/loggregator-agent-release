package collector_test

import (
	"log"
	"time"

	"code.cloudfoundry.org/loggregator-agent/pkg/collector"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Processor", func() {
	It("broadcasts stats to all senders", func() {
		in := func() (collector.SystemStat, error) {
			return defaultStat, nil
		}

		s1 := newSpySender()
		s2 := newSpySender()

		p := collector.NewProcessor(
			in,
			[]collector.StatsSender{s1, s2},
			10*time.Millisecond,
			log.New(GinkgoWriter, "", log.LstdFlags),
		)

		go p.Run()

		var stat collector.SystemStat
		Eventually(s1.stats).Should(Receive(&stat))
		Expect(stat).To(Equal(defaultStat))

		Eventually(s2.stats).Should(Receive(&stat))
		Expect(stat).To(Equal(defaultStat))
	})
})

type spySender struct {
	stats chan collector.SystemStat
}

func newSpySender() *spySender {
	return &spySender{
		stats: make(chan collector.SystemStat, 100),
	}
}

func (s *spySender) Send(stats collector.SystemStat) {
	s.stats <- stats
}

var (
	defaultStat = collector.SystemStat{
		MemKB:      1025,
		MemPercent: 10.01,

		SwapKB:      2049,
		SwapPercent: 20.01,

		Load1M:  1.1,
		Load5M:  5.5,
		Load15M: 15.15,

		CPUStat: collector.CPUStat{
			User:   25.25,
			System: 52.52,
			Idle:   10.10,
			Wait:   22.22,
		},

		SystemDisk: collector.DiskStat{
			Present: true,

			Percent:      35.0,
			InodePercent: 45.0,

			ReadBytes:  10,
			WriteBytes: 20,
			ReadTime:   30,
			WriteTime:  40,
			IOTime:     50,
		},

		EphemeralDisk: collector.DiskStat{
			Present: true,

			Percent:      55.0,
			InodePercent: 65.0,

			ReadBytes:  100,
			WriteBytes: 200,
			ReadTime:   300,
			WriteTime:  400,
			IOTime:     500,
		},

		PersistentDisk: collector.DiskStat{
			Present: true,

			Percent:      75.0,
			InodePercent: 85.0,

			ReadBytes:  1000,
			WriteBytes: 2000,
			ReadTime:   3000,
			WriteTime:  4000,
			IOTime:     5000,
		},

		ProtoCounters: collector.ProtoCountersStat{
			Present:         true,
			IPForwarding:    1,
			UDPNoPorts:      2,
			UDPInErrors:     3,
			UDPLiteInErrors: 4,
			TCPActiveOpens:  5,
			TCPCurrEstab:    6,
			TCPRetransSegs:  7,
		},

		Health: collector.HealthStat{
			Present: true,
			Healthy: true,
		},
	}
)
