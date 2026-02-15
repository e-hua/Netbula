package types

import (
	"fmt"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

const GigabyteInBytes = 1024 * 1024 * 1024;

type Stats struct {
	MemTotalInBytes uint64 `json:"mem_total_in_bytes"`
	MemUsedPercent float64 `json:"mem_used_percent"`

	DiskTotalInBytes uint64 `json:"disk_total_in_bytes"`
	DiskUsedPercent float64 `json:"disk_used_percent"`

	CpuCount int `json:"cpu_count"`
	CpuPercents []float64 `json:"cpu_percents"`

	LoadAvg float64 `json:"load_average"` 
}

func GetStats() (*Stats, error) {
	v, err := mem.VirtualMemory()
	if (err != nil) {
		return nil, fmt.Errorf("Failed to get memory stats: %w", err)
	}

	d, err := disk.Usage("/")
	if (err != nil) {
		return nil, fmt.Errorf("Failed to get disk stats: %w", err)
	}

	// Get the average CPU usage
	cpuPercents, err := cpu.Percent(0, false)
	if (err != nil) {
		return nil, fmt.Errorf("Failed to get cpu percent stats: %w", err)
	}

	loadAvg, err := load.Avg()
	if (err != nil) {
		return nil, fmt.Errorf("Failed to get cpu load average stats: %w", err)
	}

	cpuCount, _ := cpu.Counts(true)

	return & Stats{
			MemTotalInBytes: v.Total, 
			MemUsedPercent: v.UsedPercent,
			DiskTotalInBytes: d.Total, 
			DiskUsedPercent: d.UsedPercent,
			CpuCount: cpuCount,
			CpuPercents: cpuPercents,
			LoadAvg: loadAvg.Load1,
	}, nil
}