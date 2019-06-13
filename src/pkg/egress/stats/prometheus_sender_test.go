package stats_test

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/collector"
	"code.cloudfoundry.org/loggregator-agent/pkg/egress/stats"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("Prometheus Sender", func() {
	var (
		sender   *stats.PromSender
		registry *stubRegistry
		labels   map[string]string
	)

	BeforeEach(func() {
		registry = newStubRegistry()
		labels = map[string]string{
			"source_id":  "test-origin",
			"deployment": "test-deployment",
			"job":        "test-job",
			"index":      "test-index",
			"ip":         "test-ip",
		}

		sender = stats.NewPromSender(registry, "test-origin", labels)
	})

	It("gets the correct number of metrics from the registry", func() {
		sender.Send(defaultInput)

		Expect(registry.gaugeCount).To(Equal(52))
	})

	It("does not panic with no default labels", func() {
		stats.NewPromSender(registry, "test-origin", nil)
	})

	DescribeTable("default tags", func(tag, value string) {
		sender.Send(defaultInput)

		gauge := registry.gauges["system_mem_kbtest-originKiB"]

		Expect(gauge.tags[tag]).To(Equal(value))
	},
		Entry("origin", "origin", "test-origin"),
		Entry("source_id", "source_id", "test-origin"),
		Entry("deployment", "deployment", "test-deployment"),
		Entry("job", "job", "test-job"),
		Entry("index", "index", "test-index"),
		Entry("ip", "ip", "test-ip"),
	)

	DescribeTable("default metrics", func(name string, tags map[string]string, unit string, value float64) {
		sender.Send(defaultInput)

		cpuName := ""
		if cpuTag, ok := tags["cpu_name"]; ok {
			cpuName = cpuTag
		}

		keyName := name + tags["origin"] + unit + cpuName
		gauge := registry.gauges[keyName]

		Expect(gauge).NotTo(BeNil())
		Expect(gauge.value).To(BeNumerically("==", value))

		for k, v := range tags {
			Expect(gauge.tags[k]).To(Equal(v))
		}
	},
		Entry("system_mem_kb", "system_mem_kb", map[string]string{"origin": "test-origin"}, "KiB", 1025.0),
		Entry("system_mem_percent", "system_mem_percent", map[string]string{"origin": "test-origin"}, "Percent", 10.01),
		Entry("system_swap_kb", "system_swap_kb", map[string]string{"origin": "test-origin"}, "KiB", 2049.0),
		Entry("system_swap_percent", "system_swap_percent", map[string]string{"origin": "test-origin"}, "Percent", 20.01),
		Entry("system_load_1m", "system_load_1m", map[string]string{"origin": "test-origin"}, "Load", 1.1),
		Entry("system_load_5m", "system_load_5m", map[string]string{"origin": "test-origin"}, "Load", 5.5),
		Entry("system_load_15m", "system_load_15m", map[string]string{"origin": "test-origin"}, "Load", 15.15),
		Entry("system_cpu_user", "system_cpu_user", map[string]string{"origin": "test-origin"}, "Percent", 25.25),
		Entry("system_cpu_sys", "system_cpu_sys", map[string]string{"origin": "test-origin"}, "Percent", 52.52),
		Entry("system_cpu_idle", "system_cpu_idle", map[string]string{"origin": "test-origin"}, "Percent", 10.10),
		Entry("system_cpu_wait", "system_cpu_wait", map[string]string{"origin": "test-origin"}, "Percent", 22.22),
		Entry("system cpu core 1 user", "system_cpu_core_user", map[string]string{"origin": "test-origin", "cpu_name": "cpu1"}, "Percent", 25.25),
		Entry("system cpu core 1 sys", "system_cpu_core_sys", map[string]string{"origin": "test-origin", "cpu_name": "cpu1"}, "Percent", 52.52),
		Entry("system cpu core 1 idle", "system_cpu_core_idle", map[string]string{"origin": "test-origin", "cpu_name": "cpu1"}, "Percent", 10.10),
		Entry("system cpu core 1 wait", "system_cpu_core_wait", map[string]string{"origin": "test-origin", "cpu_name": "cpu1"}, "Percent", 22.22),
		Entry("system cpu core 2 user", "system_cpu_core_user", map[string]string{"origin": "test-origin", "cpu_name": "cpu2"}, "Percent", 25.25),
		Entry("system cpu core 2 sys", "system_cpu_core_sys", map[string]string{"origin": "test-origin", "cpu_name": "cpu2"}, "Percent", 52.52),
		Entry("system cpu core 2 idle", "system_cpu_core_idle", map[string]string{"origin": "test-origin", "cpu_name": "cpu2"}, "Percent", 10.10),
		Entry("system cpu core 2 wait", "system_cpu_core_wait", map[string]string{"origin": "test-origin", "cpu_name": "cpu2"}, "Percent", 22.22),
		Entry("system_disk_system_percent", "system_disk_system_percent", map[string]string{"origin": "test-origin"}, "Percent", 35.0),
		Entry("system_disk_system_inode_percent", "system_disk_system_inode_percent", map[string]string{"origin": "test-origin"}, "Percent", 45.0),
		Entry("system_disk_system_read_bytes", "system_disk_system_read_bytes", map[string]string{"origin": "test-origin"}, "Bytes", 10.0),
		Entry("system_disk_system_write_bytes", "system_disk_system_write_bytes", map[string]string{"origin": "test-origin"}, "Bytes", 20.0),
		Entry("system_disk_system_read_time", "system_disk_system_read_time", map[string]string{"origin": "test-origin"}, "ms", 30.0),
		Entry("system_disk_system_write_time", "system_disk_system_write_time", map[string]string{"origin": "test-origin"}, "ms", 40.0),
		Entry("system_disk_system_io_time", "system_disk_system_io_time", map[string]string{"origin": "test-origin"}, "ms", 50.0),
		Entry("system_disk_ephemeral_percent", "system_disk_ephemeral_percent", map[string]string{"origin": "test-origin"}, "Percent", 55.0),
		Entry("system_disk_ephemeral_inode_percent", "system_disk_ephemeral_inode_percent", map[string]string{"origin": "test-origin"}, "Percent", 65.0),
		Entry("system_disk_ephemeral_read_bytes", "system_disk_ephemeral_read_bytes", map[string]string{"origin": "test-origin"}, "Bytes", 100.0),
		Entry("system_disk_ephemeral_write_bytes", "system_disk_ephemeral_write_bytes", map[string]string{"origin": "test-origin"}, "Bytes", 200.0),
		Entry("system_disk_ephemeral_read_time", "system_disk_ephemeral_read_time", map[string]string{"origin": "test-origin"}, "ms", 300.0),
		Entry("system_disk_ephemeral_write_time", "system_disk_ephemeral_write_time", map[string]string{"origin": "test-origin"}, "ms", 400.0),
		Entry("system_disk_ephemeral_io_time", "system_disk_ephemeral_io_time", map[string]string{"origin": "test-origin"}, "ms", 500.0),
		Entry("system_disk_persistent_percent", "system_disk_persistent_percent", map[string]string{"origin": "test-origin"}, "Percent", 75.0),
		Entry("system_disk_persistent_inode_percent", "system_disk_persistent_inode_percent", map[string]string{"origin": "test-origin"}, "Percent", 85.0),
		Entry("system_disk_persistent_read_bytes", "system_disk_persistent_read_bytes", map[string]string{"origin": "test-origin"}, "Bytes", 1000.0),
		Entry("system_disk_persistent_write_bytes", "system_disk_persistent_write_bytes", map[string]string{"origin": "test-origin"}, "Bytes", 2000.0),
		Entry("system_disk_persistent_read_time", "system_disk_persistent_read_time", map[string]string{"origin": "test-origin"}, "ms", 3000.0),
		Entry("system_disk_persistent_write_time", "system_disk_persistent_write_time", map[string]string{"origin": "test-origin"}, "ms", 4000.0),
		Entry("system_disk_persistent_io_time", "system_disk_persistent_io_time", map[string]string{"origin": "test-origin"}, "ms", 5000.0),
		Entry("system_healthy", "system_healthy", map[string]string{"origin": "test-origin"}, "", 1.0),
	)

	DescribeTable("network metrics", func(name, origin, unit, networkName string, value float64) {
		sender.Send(networkInput)

		gauge, exists := registry.gauges[name+origin+unit+networkName]
		Expect(exists).To(BeTrue())

		Expect(gauge.value).To(BeNumerically("==", value))
		Expect(gauge.tags["network_interface"]).To(Or(Equal("eth0"), Equal("eth1")))
		Expect(gauge.tags["origin"]).To(Equal("test-origin"))
	},
		Entry("system_network_bytes_sent", "system_network_bytes_sent", "test-origin", "Bytes", "eth0", 1.0),
		Entry("system_network_bytes_received", "system_network_bytes_received", "test-origin", "Bytes", "eth0", 2.0),
		Entry("system_network_packets_sent", "system_network_packets_sent", "test-origin", "Packets", "eth0", 3.0),
		Entry("system_network_packets_received", "system_network_packets_received", "test-origin", "Packets", "eth0", 4.0),
		Entry("system_network_error_in", "system_network_error_in", "test-origin", "Frames", "eth0", 5.0),
		Entry("system_network_error_out", "system_network_error_out", "test-origin", "Frames", "eth0", 6.0),
		Entry("system_network_drop_in", "system_network_drop_in", "test-origin", "Packets", "eth0", 7.0),
		Entry("system_network_drop_out", "system_network_drop_out", "test-origin", "Packets", "eth0", 8.0),

		Entry("system_network_bytes_sent", "system_network_bytes_sent", "test-origin", "Bytes", "eth1", 10.0),
		Entry("system_network_bytes_received", "system_network_bytes_received", "test-origin", "Bytes", "eth1", 20.0),
		Entry("system_network_packets_sent", "system_network_packets_sent", "test-origin", "Packets", "eth1", 30.0),
		Entry("system_network_packets_received", "system_network_packets_received", "test-origin", "Packets", "eth1", 40.0),
		Entry("system_network_error_in", "system_network_error_in", "test-origin", "Frames", "eth1", 50.0),
		Entry("system_network_error_out", "system_network_error_out", "test-origin", "Frames", "eth1", 60.0),
		Entry("system_network_drop_in", "system_network_drop_in", "test-origin", "Packets", "eth1", 70.0),
		Entry("system_network_drop_out", "system_network_drop_out", "test-origin", "Packets", "eth1", 80.0),
	)

	DescribeTable("does not have disk metrics if disk is not present", func(name, origin, unit string) {
		sender.Send(collector.SystemStat{})

		_, exists := registry.gauges[name+origin+unit]
		Expect(exists).To(BeFalse())
	},
		Entry("system_disk_system_percent", "system_disk_system_percent", "test-origin", "Percent"),
		Entry("system_disk_system_inode_percent", "system_disk_system_inode_percent", "test-origin", "Percent"),
		Entry("system_disk_system_read_bytes", "system_disk_system_read_bytes", "test-origin", "Bytes"),
		Entry("system_disk_system_write_bytes", "system_disk_system_write_bytes", "test-origin", "Bytes"),
		Entry("system_disk_system_read_time", "system_disk_system_read_time", "test-origin", "ms"),
		Entry("system_disk_system_write_time", "system_disk_system_write_time", "test-origin", "ms"),
		Entry("system_disk_system_io_time", "system_disk_system_io_time", "test-origin", "ms"),
		Entry("system_disk_ephemeral_percent", "system_disk_ephemeral_percent", "test-origin", "Percent"),
		Entry("system_disk_ephemeral_inode_percent", "system_disk_ephemeral_inode_percent", "test-origin", "Percent"),
		Entry("system_disk_ephemeral_read_bytes", "system_disk_ephemeral_read_bytes", "test-origin", "Bytes"),
		Entry("system_disk_ephemeral_write_bytes", "system_disk_ephemeral_write_bytes", "test-origin", "Bytes"),
		Entry("system_disk_ephemeral_read_time", "system_disk_ephemeral_read_time", "test-origin", "ms"),
		Entry("system_disk_ephemeral_write_time", "system_disk_ephemeral_write_time", "test-origin", "ms"),
		Entry("system_disk_ephemeral_io_time", "system_disk_ephemeral_io_time", "test-origin", "ms"),
		Entry("system_disk_persistent_percent", "system_disk_persistent_percent", "test-origin", "Percent"),
		Entry("system_disk_persistent_inode_percent", "system_disk_persistent_inode_percent", "test-origin", "Percent"),
		Entry("system_disk_persistent_read_bytes", "system_disk_persistent_read_bytes", "test-origin", "Bytes"),
		Entry("system_disk_persistent_write_bytes", "system_disk_persistent_write_bytes", "test-origin", "Bytes"),
		Entry("system_disk_persistent_read_time", "system_disk_persistent_read_time", "test-origin", "ms"),
		Entry("system_disk_persistent_write_time", "system_disk_persistent_write_time", "test-origin", "ms"),
		Entry("system_disk_persistent_io_time", "system_disk_persistent_io_time", "test-origin", "ms"),
	)

	It("returns 0 for an unhealthy instance", func() {
		sender.Send(unhealthyInstanceInput)

		gauge, exists := registry.gauges["system_healthy"+"test-origin"]
		Expect(exists).To(BeTrue())
		Expect(gauge.value).To(Equal(0.0))
	})

	It("excludes system_healthy if health precence is false", func() {
		sender.Send(collector.SystemStat{})

		_, exists := registry.gauges["system_healthy"+"test-origin"]
		Expect(exists).To(BeFalse())
	})
})

type spyGauge struct {
	value float64
	tags  map[string]string
}

func (g *spyGauge) Set(value float64) {
	g.value = value
}

type stubRegistry struct {
	gaugeCount int
	gauges     map[string]*spyGauge
}

func newStubRegistry() *stubRegistry {
	return &stubRegistry{
		gauges: make(map[string]*spyGauge),
	}
}

func (r *stubRegistry) Get(gaugeName, origin, unit string, tags map[string]string) stats.Gauge {
	r.gaugeCount++

	networkName := ""
	if tags != nil {
		networkName = tags["network_interface"]
	}

	cpuName := ""
	if cpuTag, ok := tags["cpu_name"]; ok {
		cpuName = cpuTag
	}

	key := gaugeName + origin + unit + networkName + cpuName

	r.gauges[key] = &spyGauge{
		tags: tags,
	}

	return r.gauges[key]
}
