package collector

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

const (
	systemDiskPath     = "/"
	ephemeralDiskPath  = "/var/vcap/data"
	persistentDiskPath = "/var/vcap/store"
	instanceHealthPath = "/var/vcap/instance/health.json"
)

type SystemStat struct {
	CPUStat
	CPUCoreStats []CPUCoreStat

	MemKB      uint64
	MemPercent float64

	SwapKB      uint64
	SwapPercent float64

	Load1M  float64
	Load5M  float64
	Load15M float64

	SystemDisk     DiskStat
	EphemeralDisk  DiskStat
	PersistentDisk DiskStat

	ProtoCounters ProtoCountersStat

	Networks []NetworkStat

	Health HealthStat
}

type CPUCoreStat struct {
	CPU string
	CPUStat
}

type CPUStat struct {
	User   float64
	System float64
	Wait   float64
	Idle   float64
}

type DiskStat struct {
	Present bool

	Percent      float64
	InodePercent float64
	ReadBytes    uint64
	WriteBytes   uint64
	ReadTime     uint64
	WriteTime    uint64
	IOTime       uint64
}

type ProtoCountersStat struct {
	Present bool

	IPForwarding    int64
	UDPNoPorts      int64
	UDPInErrors     int64
	UDPLiteInErrors int64

	TCPActiveOpens int64
	TCPCurrEstab   int64
	TCPRetransSegs int64
}

type Collector struct {
	rawCollector  RawCollector
	prevTimesStat cpu.TimesStat
	prevCoreStats []cpu.TimesStat
}

type NetworkStat struct {
	Name            string
	BytesSent       uint64
	BytesReceived   uint64
	PacketsSent     uint64
	PacketsReceived uint64
	ErrIn           uint64
	ErrOut          uint64
	DropIn          uint64
	DropOut         uint64
}

type HealthStat struct {
	Present bool
	Healthy bool
}

func New(log *log.Logger, opts ...CollectorOption) Collector {
	c := Collector{
		rawCollector: defaultRawCollector{},
	}

	for _, o := range opts {
		o(&c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	firstTS, err := c.rawCollector.TimesWithContext(ctx, false)
	if err != nil {
		log.Panicf("failed to collect initial CPU times: %s", err)
	}
	c.prevTimesStat = firstTS[0]

	c.prevCoreStats, err = c.rawCollector.TimesWithContext(ctx, true)
	if err != nil {
		log.Panicf("failed to collect initial CPU Core times: %s", err)
	}

	return c
}

func (c Collector) Collect() (SystemStat, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m, err := c.rawCollector.VirtualMemoryWithContext(ctx)
	if err != nil {
		return SystemStat{}, err
	}

	s, err := c.rawCollector.SwapMemoryWithContext(ctx)
	if err != nil {
		return SystemStat{}, err
	}

	l, err := c.rawCollector.AvgWithContext(ctx)
	if err != nil {
		return SystemStat{}, err
	}

	ts, err := c.rawCollector.TimesWithContext(ctx, false)
	if err != nil {
		return SystemStat{}, err
	}

	cpu := calculateCPUStat(c.prevTimesStat, ts[0])
	c.prevTimesStat = ts[0]

	coreTs, err := c.rawCollector.TimesWithContext(ctx, true)
	if err != nil {
		return SystemStat{}, err
	}

	coreStats := make([]CPUCoreStat, len(coreTs))
	for i, core := range coreTs {
		coreStats[i] = CPUCoreStat{
			CPU:     core.CPU,
			CPUStat: calculateCPUStat(c.prevCoreStats[i], core),
		}
	}

	c.prevCoreStats = coreTs

	sdisk, err := c.diskStat(ctx, systemDiskPath)
	if err != nil {
		return SystemStat{}, err
	}

	edisk, err := c.diskStat(ctx, ephemeralDiskPath)
	if err != nil {
		return SystemStat{}, err
	}

	pdisk, err := c.diskStat(ctx, persistentDiskPath)
	if err != nil {
		return SystemStat{}, err
	}

	networks, err := c.networkStat(ctx)
	if err != nil {
		return SystemStat{}, err
	}

	protoCounters, err := c.protoCountersStat(ctx)
	if err != nil {
		return SystemStat{}, err
	}

	return SystemStat{
		CPUStat:      cpu,
		CPUCoreStats: coreStats,

		MemKB:      m.Used / 1024,
		MemPercent: m.UsedPercent,

		SwapKB:      s.Used / 1024,
		SwapPercent: s.UsedPercent,

		Load1M:  l.Load1,
		Load5M:  l.Load5,
		Load15M: l.Load15,

		SystemDisk:     sdisk,
		EphemeralDisk:  edisk,
		PersistentDisk: pdisk,

		Networks: networks,

		Health: c.healthy(),

		ProtoCounters: protoCounters,
	}, nil
}

func (c Collector) healthy() HealthStat {
	h, err := c.rawCollector.InstanceHealth()
	if err != nil {
		return HealthStat{}
	}

	var ih map[string]string
	err = json.Unmarshal(h, &ih)
	if err != nil {
		return HealthStat{}
	}

	if ih["state"] == "running" {
		return HealthStat{
			Present: true,
			Healthy: true,
		}
	}

	return HealthStat{
		Present: true,
	}
}

func (c Collector) diskStat(ctx context.Context, path string) (DiskStat, error) {
	disk, err := c.rawCollector.UsageWithContext(ctx, path)
	if err != nil && os.IsNotExist(err) {
		return DiskStat{}, nil
	}

	if err != nil {
		return DiskStat{}, err
	}

	partitions, err := c.rawCollector.PartitionsWithContext(ctx, true)
	if err != nil {
		return DiskStat{}, err
	}

	var devicePath string
	for _, p := range partitions {
		if p.Mountpoint == path {
			devicePath = p.Device
			break
		}
	}

	if devicePath == "" {
		return DiskStat{}, nil
	}

	pStat, err := c.rawCollector.DiskIOCountersWithContext(ctx, devicePath)
	if err != nil {
		return DiskStat{}, err
	}

	deviceName := filepath.Base(devicePath)

	return DiskStat{
		Percent:      disk.UsedPercent,
		InodePercent: disk.InodesUsedPercent,
		ReadBytes:    pStat[deviceName].ReadBytes,
		WriteBytes:   pStat[deviceName].WriteBytes,
		ReadTime:     pStat[deviceName].ReadTime,
		WriteTime:    pStat[deviceName].WriteTime,
		IOTime:       pStat[deviceName].IoTime,
		Present:      true,
	}, nil
}

func (c Collector) networkStat(ctx context.Context) ([]NetworkStat, error) {
	counters, err := c.rawCollector.NetIOCountersWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	var ns []NetworkStat
	for _, c := range counters {
		if strings.HasPrefix(c.Name, "eth") {
			ns = append(ns, NetworkStat{
				Name:            c.Name,
				BytesSent:       c.BytesSent,
				BytesReceived:   c.BytesRecv,
				PacketsSent:     c.PacketsSent,
				PacketsReceived: c.PacketsRecv,
				ErrIn:           c.Errin,
				ErrOut:          c.Errout,
				DropIn:          c.Dropin,
				DropOut:         c.Dropout,
			})
		}
	}

	return ns, nil
}

func (c Collector) protoCountersStat(ctx context.Context) (ProtoCountersStat, error) {
	protoCounters, err := c.rawCollector.ProtoCountersWithContext(ctx, []string{"tcp", "udp", "ip", "udplite"})
	if err != nil {
		return ProtoCountersStat{}, err
	}

	var protoCountersStat ProtoCountersStat
	if len(protoCounters) > 0 {
		protoCountersStat.Present = true
	}
	for _, pc := range protoCounters {
		s := pc.Stats
		switch pc.Protocol {
		case "ip":
			protoCountersStat.IPForwarding = s["Forwarding"]
		case "udp":
			protoCountersStat.UDPNoPorts = s["NoPorts"]
			protoCountersStat.UDPInErrors = s["InErrors"]
		case "udplite":
			protoCountersStat.UDPLiteInErrors = s["InErrors"]
		case "tcp":
			protoCountersStat.TCPActiveOpens = s["ActiveOpens"]
			protoCountersStat.TCPCurrEstab = s["CurrEstab"]
			protoCountersStat.TCPRetransSegs = s["RetransSegs"]
		}
	}
	return protoCountersStat, nil
}

func calculateCPUStat(previous, current cpu.TimesStat) CPUStat {
	totalDiff := current.Total() - previous.Total()

	return CPUStat{
		User:   (current.User - previous.User) / totalDiff * 100.0,
		System: (current.System - previous.System) / totalDiff * 100.0,
		Idle:   (current.Idle - previous.Idle) / totalDiff * 100.0,
		Wait:   (current.Iowait - previous.Iowait) / totalDiff * 100.0,
	}
}

type RawCollector interface {
	ProtoCountersWithContext(context.Context, []string) ([]net.ProtoCountersStat, error)
	VirtualMemoryWithContext(context.Context) (*mem.VirtualMemoryStat, error)
	SwapMemoryWithContext(context.Context) (*mem.SwapMemoryStat, error)
	AvgWithContext(context.Context) (*load.AvgStat, error)
	TimesWithContext(context.Context, bool) ([]cpu.TimesStat, error)
	UsageWithContext(context.Context, string) (*disk.UsageStat, error)
	NetIOCountersWithContext(context.Context, bool) ([]net.IOCountersStat, error)
	DiskIOCountersWithContext(context.Context, ...string) (map[string]disk.IOCountersStat, error)
	PartitionsWithContext(context.Context, bool) ([]disk.PartitionStat, error)
	InstanceHealth() ([]byte, error)
}

type CollectorOption func(*Collector)

func WithRawCollector(c RawCollector) CollectorOption {
	return func(cs *Collector) {
		cs.rawCollector = c
	}
}

type defaultRawCollector struct{}

func (s defaultRawCollector) VirtualMemoryWithContext(ctx context.Context) (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemoryWithContext(ctx)
}

func (s defaultRawCollector) SwapMemoryWithContext(ctx context.Context) (*mem.SwapMemoryStat, error) {
	return mem.SwapMemoryWithContext(ctx)
}

func (s defaultRawCollector) TimesWithContext(ctx context.Context, perCPU bool) ([]cpu.TimesStat, error) {
	return cpu.TimesWithContext(ctx, perCPU)
}

func (s defaultRawCollector) UsageWithContext(ctx context.Context, path string) (*disk.UsageStat, error) {
	return disk.UsageWithContext(ctx, path)
}

func (s defaultRawCollector) NetIOCountersWithContext(ctx context.Context, pernic bool) ([]net.IOCountersStat, error) {
	return net.IOCountersWithContext(ctx, pernic)
}

func (s defaultRawCollector) DiskIOCountersWithContext(ctx context.Context, names ...string) (map[string]disk.IOCountersStat, error) {
	return disk.IOCountersWithContext(ctx, names...)
}

func (s defaultRawCollector) PartitionsWithContext(ctx context.Context, all bool) ([]disk.PartitionStat, error) {
	return disk.PartitionsWithContext(ctx, all)
}

func (s defaultRawCollector) InstanceHealth() ([]byte, error) {
	return ioutil.ReadFile(instanceHealthPath)
}
