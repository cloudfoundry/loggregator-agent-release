package stats_test

import (
	"testing"

	"code.cloudfoundry.org/loggregator-agent/pkg/collector"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCollector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stats Suite")
}

var (
	defaultInput = collector.SystemStat{
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

		CPUCoreStats: []collector.CPUCoreStat{
			{
				CPU:    "cpu1",
				CPUStat: collector.CPUStat{
					User:   25.25,
					System: 52.52,
					Idle:   10.10,
					Wait:   22.22,
				},

			},
			{
				CPU:    "cpu2",
				CPUStat: collector.CPUStat{
					User:   25.25,
					System: 52.52,
					Idle:   10.10,
					Wait:   22.22,
				},

			},
			{
				CPU:    "cpu3",
				CPUStat: collector.CPUStat{
					User:   25.25,
					System: 52.52,
					Idle:   10.10,
					Wait:   22.22,
				},

			},
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

	networkInput = collector.SystemStat{
		Networks: []collector.NetworkStat{
			{
				Name:            "eth0",
				BytesSent:       1,
				BytesReceived:   2,
				PacketsSent:     3,
				PacketsReceived: 4,
				ErrIn:           5,
				ErrOut:          6,
				DropIn:          7,
				DropOut:         8,
			},
			{
				Name:            "eth1",
				BytesSent:       10,
				BytesReceived:   20,
				PacketsSent:     30,
				PacketsReceived: 40,
				ErrIn:           50,
				ErrOut:          60,
				DropIn:          70,
				DropOut:         80,
			},
		},
	}

	unhealthyInstanceInput = collector.SystemStat{
		Health: collector.HealthStat{
			Present: true,
			Healthy: false,
		},
	}
)
