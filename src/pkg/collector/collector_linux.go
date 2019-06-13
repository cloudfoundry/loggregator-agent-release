// +build !windows

package collector

import (
	"context"

	"github.com/shirou/gopsutil/load"
	"github.com/shirou/gopsutil/net"
)

func (s defaultRawCollector) AvgWithContext(ctx context.Context) (*load.AvgStat, error) {
	return load.AvgWithContext(ctx)
}

func (s defaultRawCollector) ProtoCountersWithContext(ctx context.Context, protocols []string) ([]net.ProtoCountersStat, error) {
	return net.ProtoCountersWithContext(ctx, protocols)
}
