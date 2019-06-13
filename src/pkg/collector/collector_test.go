package collector_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"syscall"

	"code.cloudfoundry.org/loggregator-agent/pkg/collector"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Collector", func() {
	var (
		c   collector.Collector
		src *stubRawCollector
	)

	BeforeEach(func() {
		src = &stubRawCollector{}
		c = collector.New(
			log.New(GinkgoWriter, "", log.LstdFlags),
			collector.WithRawCollector(src),
		)
	})

	It("returns true if the instance is healthy", func() {
		src.healthy = true

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.Health.Healthy).To(BeTrue())
		Expect(stats.Health.Present).To(BeTrue())
	})

	It("returns true if the instance is healthy", func() {
		src.healthy = false

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.Health.Healthy).To(BeFalse())
		Expect(stats.Health.Present).To(BeTrue())
	})

	It("returns the memory metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.MemKB).To(Equal(uint64(2)))
		Expect(stats.MemPercent).To(Equal(40.23))
	})

	It("returns the swap metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.SwapKB).To(Equal(uint64(4)))
		Expect(stats.SwapPercent).To(Equal(20.23))
	})

	It("returns load metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.Load1M).To(Equal(1.1))
		Expect(stats.Load5M).To(Equal(5.5))
		Expect(stats.Load15M).To(Equal(15.15))
	})

	It("returns cpu metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.User).To(Equal(10.0))
		Expect(stats.System).To(Equal(20.0))
		Expect(stats.Idle).To(Equal(30.0))
		Expect(stats.Wait).To(Equal(40.0))
	})

	It("returns cpu per core metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.CPUCoreStats).To(HaveLen(4))

		Expect(stats.CPUCoreStats[0].CPU).To(Equal("cpu1"))
		Expect(stats.CPUCoreStats[0].User).To(Equal(10.0))
		Expect(stats.CPUCoreStats[0].System).To(Equal(20.0))
		Expect(stats.CPUCoreStats[0].Idle).To(Equal(30.0))
		Expect(stats.CPUCoreStats[0].Wait).To(Equal(40.0))

		Expect(stats.CPUCoreStats[1].CPU).To(Equal("cpu2"))
		Expect(stats.CPUCoreStats[2].CPU).To(Equal("cpu3"))
		Expect(stats.CPUCoreStats[3].CPU).To(Equal("cpu4"))
	})

	It("returns network metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.Networks).To(HaveLen(1))

		stat := stats.Networks[0]
		Expect(stat.Name).To(Equal("eth0"))
		Expect(stat.BytesSent).To(Equal(uint64(10)))
		Expect(stat.BytesReceived).To(Equal(uint64(20)))
		Expect(stat.PacketsSent).To(Equal(uint64(30)))
		Expect(stat.PacketsReceived).To(Equal(uint64(40)))
		Expect(stat.ErrIn).To(Equal(uint64(50)))
		Expect(stat.ErrOut).To(Equal(uint64(60)))
		Expect(stat.DropIn).To(Equal(uint64(70)))
		Expect(stat.DropOut).To(Equal(uint64(80)))
	})

	It("returns protocol based network metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.ProtoCounters.Present).To(BeTrue())
		Expect(stats.ProtoCounters.UDPNoPorts).To(Equal(int64(1337)))
		Expect(stats.ProtoCounters.UDPInErrors).To(Equal(int64(1338)))

		Expect(stats.ProtoCounters.UDPLiteInErrors).To(Equal(int64(1339)))

		Expect(stats.ProtoCounters.TCPActiveOpens).To(Equal(int64(1340)))
		Expect(stats.ProtoCounters.TCPCurrEstab).To(Equal(int64(1341)))
		Expect(stats.ProtoCounters.TCPRetransSegs).To(Equal(int64(1342)))

		Expect(stats.ProtoCounters.IPForwarding).To(Equal(int64(1343)))
	})

	It("returns disk metrics", func() {
		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.SystemDisk.Percent).To(Equal(65.0))
		Expect(stats.SystemDisk.InodePercent).To(Equal(75.0))
		Expect(stats.SystemDisk.ReadBytes).To(Equal(uint64(100)))
		Expect(stats.SystemDisk.WriteBytes).To(Equal(uint64(200)))
		Expect(stats.SystemDisk.ReadTime).To(Equal(uint64(300)))
		Expect(stats.SystemDisk.WriteTime).To(Equal(uint64(400)))
		Expect(stats.SystemDisk.IOTime).To(Equal(uint64(500)))
		Expect(stats.SystemDisk.Present).To(BeTrue())

		Expect(stats.EphemeralDisk.Percent).To(Equal(85.0))
		Expect(stats.EphemeralDisk.InodePercent).To(Equal(95.0))
		Expect(stats.EphemeralDisk.ReadBytes).To(Equal(uint64(1000)))
		Expect(stats.EphemeralDisk.WriteBytes).To(Equal(uint64(2000)))
		Expect(stats.EphemeralDisk.ReadTime).To(Equal(uint64(3000)))
		Expect(stats.EphemeralDisk.WriteTime).To(Equal(uint64(4000)))
		Expect(stats.EphemeralDisk.IOTime).To(Equal(uint64(5000)))
		Expect(stats.EphemeralDisk.Present).To(BeTrue())

		Expect(stats.PersistentDisk.Percent).To(Equal(105.0))
		Expect(stats.PersistentDisk.InodePercent).To(Equal(115.0))
		Expect(stats.PersistentDisk.ReadBytes).To(Equal(uint64(10000)))
		Expect(stats.PersistentDisk.WriteBytes).To(Equal(uint64(20000)))
		Expect(stats.PersistentDisk.ReadTime).To(Equal(uint64(30000)))
		Expect(stats.PersistentDisk.WriteTime).To(Equal(uint64(40000)))
		Expect(stats.PersistentDisk.IOTime).To(Equal(uint64(50000)))
		Expect(stats.PersistentDisk.Present).To(BeTrue())
	})

	It("returns disk metrics when persistent disk is not present", func() {
		src.persistentDiskUsageErr = syscall.ENOENT

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.SystemDisk.Percent).To(Equal(65.0))
		Expect(stats.SystemDisk.InodePercent).To(Equal(75.0))
		Expect(stats.SystemDisk.ReadBytes).To(Equal(uint64(100)))
		Expect(stats.SystemDisk.WriteBytes).To(Equal(uint64(200)))
		Expect(stats.SystemDisk.ReadTime).To(Equal(uint64(300)))
		Expect(stats.SystemDisk.WriteTime).To(Equal(uint64(400)))
		Expect(stats.SystemDisk.IOTime).To(Equal(uint64(500)))
		Expect(stats.SystemDisk.Present).To(BeTrue())

		Expect(stats.EphemeralDisk.Percent).To(Equal(85.0))
		Expect(stats.EphemeralDisk.InodePercent).To(Equal(95.0))
		Expect(stats.EphemeralDisk.ReadBytes).To(Equal(uint64(1000)))
		Expect(stats.EphemeralDisk.WriteBytes).To(Equal(uint64(2000)))
		Expect(stats.EphemeralDisk.ReadTime).To(Equal(uint64(3000)))
		Expect(stats.EphemeralDisk.WriteTime).To(Equal(uint64(4000)))
		Expect(stats.EphemeralDisk.IOTime).To(Equal(uint64(5000)))
		Expect(stats.EphemeralDisk.Present).To(BeTrue())
	})

	It("shows the system disk is not present if directory does not exist", func() {
		src.systemDiskUsageErr = syscall.ENOENT

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.SystemDisk.Percent).To(Equal(0.0))
		Expect(stats.SystemDisk.InodePercent).To(Equal(0.0))
		Expect(stats.SystemDisk.Present).To(BeFalse())
	})

	It("shows the ephemeral disk is not present if directory does not exist", func() {
		src.ephemeralDiskUsageErr = syscall.ENOENT

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.EphemeralDisk.Percent).To(Equal(0.0))
		Expect(stats.EphemeralDisk.InodePercent).To(Equal(0.0))
		Expect(stats.EphemeralDisk.Present).To(BeFalse())
	})

	It("shows the persistent disk is not present if directory does not exist", func() {
		src.persistentDiskUsageErr = syscall.ENOENT

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.PersistentDisk.Percent).To(Equal(0.0))
		Expect(stats.PersistentDisk.InodePercent).To(Equal(0.0))
		Expect(stats.PersistentDisk.Present).To(BeFalse())
	})

	It("panics on initialization when failing to get initial cpu times", func() {
		src.cpuTimesErr = errors.New("an error")

		Expect(func() {
			_ = collector.New(
				log.New(GinkgoWriter, "", log.LstdFlags),
				collector.WithRawCollector(src),
			)
		}).To(Panic())
	})

	It("returns an error when unable to collect protocol counters", func() {
		src.protoCountersError = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an health presence as false if we fail to read", func() {
		src.healthyErr = errors.New("an error")

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.Health.Healthy).To(BeFalse())
		Expect(stats.Health.Present).To(BeFalse())
	})

	It("returns an error if we fail to unmarshal instance health", func() {
		src.healthyInvalidJSON = true

		stats, err := c.Collect()
		Expect(err).ToNot(HaveOccurred())

		Expect(stats.Health.Healthy).To(BeFalse())
		Expect(stats.Health.Present).To(BeFalse())
	})

	It("returns an error when getting memory fails", func() {
		src.virtualMemoryErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting swap fails", func() {
		src.swapMemoryErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting cpu load fails", func() {
		src.cpuLoadErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting cpu times fails", func() {
		src.cpuTimesErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting networks fails", func() {
		src.netIOCountersErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting system disk usage fails", func() {
		src.systemDiskUsageErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting partitions fails", func() {
		src.partitionsErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting disk IO counters fails", func() {
		src.diskIOCountersErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting ephemeral disk usage fails", func() {
		src.ephemeralDiskUsageErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})

	It("returns an error when getting persistent disk usage fails", func() {
		src.persistentDiskUsageErr = errors.New("an error")

		_, err := c.Collect()
		Expect(err).To(HaveOccurred())
	})
})

type stubRawCollector struct {
	timesCallCount      float64
	diskIOCountersNames []string
	cannotFindPartition bool

	virtualMemoryErr       error
	swapMemoryErr          error
	cpuLoadErr             error
	cpuTimesErr            error
	netIOCountersErr       error
	diskIOCountersErr      error
	systemDiskUsageErr     error
	ephemeralDiskUsageErr  error
	persistentDiskUsageErr error

	protoCountersError error

	partitionsErr error

	healthy            bool
	healthyInvalidJSON bool
	healthyErr         error
}

func (s *stubRawCollector) ProtoCountersWithContext(_ context.Context, protocols []string) ([]net.ProtoCountersStat, error) {
	if s.protoCountersError != nil {
		return nil, s.protoCountersError
	}

	return []net.ProtoCountersStat{
		{
			"udp",
			map[string]int64{
				"NoPorts":  1337,
				"InErrors": 1338,
			},
		},
		{
			"udplite",
			map[string]int64{
				"InErrors": 1339,
			},
		},
		{
			"tcp",
			map[string]int64{
				"ActiveOpens": 1340,
				"CurrEstab":   1341,
				"RetransSegs": 1342,
			},
		},
		{
			"ip",
			map[string]int64{
				"Forwarding": 1343,
			},
		},
	}, nil
}

func (s *stubRawCollector) VirtualMemoryWithContext(context.Context) (*mem.VirtualMemoryStat, error) {
	if s.virtualMemoryErr != nil {
		return nil, s.virtualMemoryErr
	}

	return &mem.VirtualMemoryStat{
		Used:        2048,
		UsedPercent: 40.23,
	}, nil
}

func (s *stubRawCollector) SwapMemoryWithContext(context.Context) (*mem.SwapMemoryStat, error) {
	if s.swapMemoryErr != nil {
		return nil, s.swapMemoryErr
	}

	return &mem.SwapMemoryStat{
		Used:        4096,
		UsedPercent: 20.23,
	}, nil
}

func (s *stubRawCollector) AvgWithContext(context.Context) (*load.AvgStat, error) {
	if s.cpuLoadErr != nil {
		return nil, s.cpuLoadErr
	}

	return &load.AvgStat{
		Load1:  1.1,
		Load5:  5.5,
		Load15: 15.15,
	}, nil
}

func (s *stubRawCollector) TimesWithContext(_ context.Context, perCPU bool) ([]cpu.TimesStat, error) {
	if s.cpuTimesErr != nil {
		return nil, s.cpuTimesErr
	}

	s.timesCallCount += 1.0

	if perCPU {
		return []cpu.TimesStat{
			{
				CPU:    "cpu1",
				User:   500.0 * s.timesCallCount,
				System: 1000.0 * s.timesCallCount,
				Idle:   1500.0 * s.timesCallCount,
				Iowait: 2000.0 * s.timesCallCount,

				Nice:      1000.0,
				Irq:       1000.0,
				Softirq:   1000.0,
				Steal:     1000.0,
				Guest:     1000.0,
				GuestNice: 1000.0,
				Stolen:    1000.0,
			},
			{
				CPU:    "cpu2",
				User:   500.0 * s.timesCallCount,
				System: 1000.0 * s.timesCallCount,
				Idle:   1500.0 * s.timesCallCount,
				Iowait: 2000.0 * s.timesCallCount,

				Nice:      1000.0,
				Irq:       1000.0,
				Softirq:   1000.0,
				Steal:     1000.0,
				Guest:     1000.0,
				GuestNice: 1000.0,
				Stolen:    1000.0,
			},
			{
				CPU:    "cpu3",
				User:   500.0 * s.timesCallCount,
				System: 1000.0 * s.timesCallCount,
				Idle:   1500.0 * s.timesCallCount,
				Iowait: 2000.0 * s.timesCallCount,

				Nice:      1000.0,
				Irq:       1000.0,
				Softirq:   1000.0,
				Steal:     1000.0,
				Guest:     1000.0,
				GuestNice: 1000.0,
				Stolen:    1000.0,
			},
			{
				CPU:    "cpu4",
				User:   500.0 * s.timesCallCount,
				System: 1000.0 * s.timesCallCount,
				Idle:   1500.0 * s.timesCallCount,
				Iowait: 2000.0 * s.timesCallCount,

				Nice:      1000.0,
				Irq:       1000.0,
				Softirq:   1000.0,
				Steal:     1000.0,
				Guest:     1000.0,
				GuestNice: 1000.0,
				Stolen:    1000.0,
			},
		}, nil
	}

	return []cpu.TimesStat{
		{
			User:   500.0 * s.timesCallCount,
			System: 1000.0 * s.timesCallCount,
			Idle:   1500.0 * s.timesCallCount,
			Iowait: 2000.0 * s.timesCallCount,

			Nice:      1000.0,
			Irq:       1000.0,
			Softirq:   1000.0,
			Steal:     1000.0,
			Guest:     1000.0,
			GuestNice: 1000.0,
			Stolen:    1000.0,
		},
	}, nil
}

func (s *stubRawCollector) NetIOCountersWithContext(context.Context, bool) ([]net.IOCountersStat, error) {
	if s.netIOCountersErr != nil {
		return nil, s.netIOCountersErr
	}

	return []net.IOCountersStat{
		{
			Name:        "blah",
			BytesSent:   1,
			BytesRecv:   2,
			PacketsSent: 3,
			PacketsRecv: 4,
			Errin:       5,
			Errout:      6,
			Dropin:      7,
			Dropout:     8,
		},
		{
			Name:        "eth0",
			BytesSent:   10,
			BytesRecv:   20,
			PacketsSent: 30,
			PacketsRecv: 40,
			Errin:       50,
			Errout:      60,
			Dropin:      70,
			Dropout:     80,
		},
	}, nil
}

func (s *stubRawCollector) UsageWithContext(_ context.Context, path string) (*disk.UsageStat, error) {
	switch path {
	case "/":
		if s.systemDiskUsageErr != nil {
			return nil, s.systemDiskUsageErr
		}

		return &disk.UsageStat{
			UsedPercent:       65.0,
			InodesUsedPercent: 75.0,
		}, nil
	case "/var/vcap/data":
		if s.ephemeralDiskUsageErr != nil {
			return nil, s.ephemeralDiskUsageErr
		}

		return &disk.UsageStat{
			UsedPercent:       85.0,
			InodesUsedPercent: 95.0,
		}, nil
	case "/var/vcap/store":
		if s.persistentDiskUsageErr != nil {
			return nil, s.persistentDiskUsageErr
		}

		return &disk.UsageStat{
			UsedPercent:       105.0,
			InodesUsedPercent: 115.0,
		}, nil
	}

	panic(fmt.Sprintf("requested usage for forbidden path: %s", path))
}

func (s *stubRawCollector) PartitionsWithContext(context.Context, bool) ([]disk.PartitionStat, error) {
	if s.partitionsErr != nil {
		return nil, s.partitionsErr
	}

	if s.cannotFindPartition {
		return nil, nil
	}

	return []disk.PartitionStat{
		{
			Device:     "/dev/sda1",
			Mountpoint: "/",
		},
		{
			Device:     "/dev/sda2",
			Mountpoint: "/waffle",
		},
		{
			Device:     "/dev/sdb1",
			Mountpoint: "/var/vcap/data",
		},
		{
			Device:     "/dev/sdb2",
			Mountpoint: "/var/vcap/store",
		},
	}, nil
}

func (s *stubRawCollector) DiskIOCountersWithContext(_ context.Context, names ...string) (map[string]disk.IOCountersStat, error) {
	s.diskIOCountersNames = append(s.diskIOCountersNames, names...)

	if len(names) != 1 {
		panic("expecting only 1 name to DiskIOCountersWithContext")
	}

	if s.diskIOCountersErr != nil {
		return nil, s.diskIOCountersErr
	}

	switch names[0] {
	case "/dev/sda1": // system disk
		return map[string]disk.IOCountersStat{
			"sda1": disk.IOCountersStat{
				ReadBytes:  100,
				WriteBytes: 200,
				ReadTime:   300,
				WriteTime:  400,
				IoTime:     500,
			},
		}, nil
	case "/dev/sdb1": // system disk
		return map[string]disk.IOCountersStat{
			"sdb1": disk.IOCountersStat{
				ReadBytes:  1000,
				WriteBytes: 2000,
				ReadTime:   3000,
				WriteTime:  4000,
				IoTime:     5000,
			},
		}, nil
	case "/dev/sdb2": // system disk
		return map[string]disk.IOCountersStat{
			"sdb2": disk.IOCountersStat{
				ReadBytes:  10000,
				WriteBytes: 20000,
				ReadTime:   30000,
				WriteTime:  40000,
				IoTime:     50000,
			},
		}, nil
	default:
		panic("unknown disk name")
	}
}

func (s *stubRawCollector) InstanceHealth() ([]byte, error) {
	if s.healthyInvalidJSON {
		return []byte("NOTJSON"), nil
	}

	if s.healthyErr != nil {
		return nil, s.healthyErr
	}

	if s.healthy {
		return []byte(`{"state":"running"}`), nil
	}

	return []byte(`{"state":"failing"}`), nil
}
