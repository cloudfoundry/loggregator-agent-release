package stats

import (
	"code.cloudfoundry.org/loggregator-agent/pkg/collector"
)

type Gauge interface {
	Set(float64)
}

type GaugeRegistry interface {
	Get(gaugeName, origin, unit string, tags map[string]string) Gauge
}

type PromSender struct {
	registry GaugeRegistry
	origin   string
	labels   map[string]string
}

func NewPromSender(registry GaugeRegistry, origin string, labels map[string]string) *PromSender {
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["origin"] = origin
	return &PromSender{
		registry: registry,
		origin:   origin,
		labels:   labels,
	}
}

func (p PromSender) Send(stats collector.SystemStat) {
	p.setSystemStats(stats)
	p.setSystemDiskGauges(stats)
	p.setEphemeralDiskGauges(stats)
	p.setPersistentDiskGauges(stats)

	for _, network := range stats.Networks {
		p.setNetworkGauges(network)
	}
}

func clone(source map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range source {
		copy[k] = v
	}
	return copy
}

func (p PromSender) setSystemStats(stats collector.SystemStat) {
	labels := p.labels

	gauge := p.registry.Get("system_cpu_sys", p.origin, "Percent", labels)
	gauge.Set(float64(stats.CPUStat.System))

	gauge = p.registry.Get("system_cpu_wait", p.origin, "Percent", labels)
	gauge.Set(float64(stats.CPUStat.Wait))

	gauge = p.registry.Get("system_cpu_idle", p.origin, "Percent", labels)
	gauge.Set(float64(stats.CPUStat.Idle))

	gauge = p.registry.Get("system_cpu_user", p.origin, "Percent", labels)
	gauge.Set(float64(stats.CPUStat.User))

	for _, perCoreStat := range stats.CPUCoreStats {
		perCoreLabels := clone(p.labels)
		perCoreLabels["cpu_name"] = perCoreStat.CPU

		gauge = p.registry.Get("system_cpu_core_sys", p.origin, "Percent", perCoreLabels)
		gauge.Set(float64(perCoreStat.CPUStat.System))

		gauge = p.registry.Get("system_cpu_core_wait", p.origin, "Percent", perCoreLabels)
		gauge.Set(float64(perCoreStat.CPUStat.Wait))

		gauge = p.registry.Get("system_cpu_core_idle", p.origin, "Percent", perCoreLabels)
		gauge.Set(float64(perCoreStat.CPUStat.Idle))

		gauge = p.registry.Get("system_cpu_core_user", p.origin, "Percent", perCoreLabels)
		gauge.Set(float64(perCoreStat.CPUStat.User))
	}

	gauge = p.registry.Get("system_mem_kb", p.origin, "KiB", labels)
	gauge.Set(float64(stats.MemKB))

	gauge = p.registry.Get("system_mem_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.MemPercent))

	gauge = p.registry.Get("system_swap_kb", p.origin, "KiB", labels)
	gauge.Set(float64(stats.SwapKB))

	gauge = p.registry.Get("system_swap_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.SwapPercent))

	gauge = p.registry.Get("system_load_1m", p.origin, "Load", labels)
	gauge.Set(float64(stats.Load1M))

	gauge = p.registry.Get("system_load_5m", p.origin, "Load", labels)
	gauge.Set(float64(stats.Load5M))

	gauge = p.registry.Get("system_load_15m", p.origin, "Load", labels)
	gauge.Set(float64(stats.Load15M))

	gauge = p.registry.Get("system_network_ip_forwarding", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.IPForwarding))

	gauge = p.registry.Get("system_network_udp_no_ports", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.UDPNoPorts))

	gauge = p.registry.Get("system_network_udp_in_errors", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.UDPInErrors))

	gauge = p.registry.Get("system_network_udp_lite_in_errors", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.UDPLiteInErrors))

	gauge = p.registry.Get("system_network_tcp_active_opens", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.TCPActiveOpens))

	gauge = p.registry.Get("system_network_tcp_curr_estab", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.TCPCurrEstab))

	gauge = p.registry.Get("system_network_tcp_retrans_segs", p.origin, "", labels)
	gauge.Set(float64(stats.ProtoCounters.TCPRetransSegs))

	if stats.Health.Present {
		var healthValue float64
		if stats.Health.Healthy {
			healthValue = 1.0
		}

		gauge = p.registry.Get("system_healthy", p.origin, "", labels)
		gauge.Set(healthValue)
	}
}

func (p PromSender) setNetworkGauges(network collector.NetworkStat) {
	labels := clone(p.labels)
	labels["network_interface"] = network.Name

	gauge := p.registry.Get("system_network_bytes_sent", p.origin, "Bytes", labels)
	gauge.Set(float64(network.BytesSent))

	gauge = p.registry.Get("system_network_bytes_received", p.origin, "Bytes", labels)
	gauge.Set(float64(network.BytesReceived))

	gauge = p.registry.Get("system_network_packets_sent", p.origin, "Packets", labels)
	gauge.Set(float64(network.PacketsSent))

	gauge = p.registry.Get("system_network_packets_received", p.origin, "Packets", labels)
	gauge.Set(float64(network.PacketsReceived))

	gauge = p.registry.Get("system_network_error_in", p.origin, "Frames", labels)
	gauge.Set(float64(network.ErrIn))

	gauge = p.registry.Get("system_network_error_out", p.origin, "Frames", labels)
	gauge.Set(float64(network.ErrOut))

	gauge = p.registry.Get("system_network_drop_in", p.origin, "Packets", labels)
	gauge.Set(float64(network.DropIn))

	gauge = p.registry.Get("system_network_drop_out", p.origin, "Packets", labels)
	gauge.Set(float64(network.DropOut))
}

func (p PromSender) setSystemDiskGauges(stats collector.SystemStat) {
	if !stats.SystemDisk.Present {
		return
	}
	labels := p.labels

	gauge := p.registry.Get("system_disk_system_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.SystemDisk.Percent))

	gauge = p.registry.Get("system_disk_system_inode_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.SystemDisk.InodePercent))

	gauge = p.registry.Get("system_disk_system_read_bytes", p.origin, "Bytes", labels)
	gauge.Set(float64(stats.SystemDisk.ReadBytes))

	gauge = p.registry.Get("system_disk_system_write_bytes", p.origin, "Bytes", labels)
	gauge.Set(float64(stats.SystemDisk.WriteBytes))

	gauge = p.registry.Get("system_disk_system_read_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.SystemDisk.ReadTime))

	gauge = p.registry.Get("system_disk_system_write_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.SystemDisk.WriteTime))

	gauge = p.registry.Get("system_disk_system_io_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.SystemDisk.IOTime))
}

func (p PromSender) setEphemeralDiskGauges(stats collector.SystemStat) {
	if !stats.EphemeralDisk.Present {
		return
	}
	labels := p.labels

	gauge := p.registry.Get("system_disk_ephemeral_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.EphemeralDisk.Percent))

	gauge = p.registry.Get("system_disk_ephemeral_inode_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.EphemeralDisk.InodePercent))

	gauge = p.registry.Get("system_disk_ephemeral_read_bytes", p.origin, "Bytes", labels)
	gauge.Set(float64(stats.EphemeralDisk.ReadBytes))

	gauge = p.registry.Get("system_disk_ephemeral_write_bytes", p.origin, "Bytes", labels)
	gauge.Set(float64(stats.EphemeralDisk.WriteBytes))

	gauge = p.registry.Get("system_disk_ephemeral_read_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.EphemeralDisk.ReadTime))

	gauge = p.registry.Get("system_disk_ephemeral_write_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.EphemeralDisk.WriteTime))

	gauge = p.registry.Get("system_disk_ephemeral_io_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.EphemeralDisk.IOTime))
}

func (p PromSender) setPersistentDiskGauges(stats collector.SystemStat) {
	if !stats.PersistentDisk.Present {
		return
	}
	labels := p.labels

	gauge := p.registry.Get("system_disk_persistent_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.PersistentDisk.Percent))

	gauge = p.registry.Get("system_disk_persistent_inode_percent", p.origin, "Percent", labels)
	gauge.Set(float64(stats.PersistentDisk.InodePercent))

	gauge = p.registry.Get("system_disk_persistent_read_bytes", p.origin, "Bytes", labels)
	gauge.Set(float64(stats.PersistentDisk.ReadBytes))

	gauge = p.registry.Get("system_disk_persistent_write_bytes", p.origin, "Bytes", labels)
	gauge.Set(float64(stats.PersistentDisk.WriteBytes))

	gauge = p.registry.Get("system_disk_persistent_read_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.PersistentDisk.ReadTime))

	gauge = p.registry.Get("system_disk_persistent_write_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.PersistentDisk.WriteTime))

	gauge = p.registry.Get("system_disk_persistent_io_time", p.origin, "ms", labels)
	gauge.Set(float64(stats.PersistentDisk.IOTime))
}
