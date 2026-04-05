package scheduler

import (
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
)

// mockNode is a test helper for generating node.Node objects quickly
func mockNode(name string, cores int, cpuLoad float64, memory int, memAllocPct float64, disk int, diskAllocPct float64) *node.Node {
	return &node.Node{
		Name:                   name,
		Cores:                  cores,
		CpuAverageLoad:         cpuLoad,
		Memory:                 memory,
		MemoryAllocatedPercent: memAllocPct,
		Disk:                   disk,
		DiskAllocatedPercent:   diskAllocPct,
		WorkerUuid:             uuid.New(),
	}
}

// mockTask is a test helper for generating task.Task objects quickly
func mockTask(cpu float64, memory int, disk int) task.Task {
	return task.Task{
		ID:     uuid.New(),
		Cpu:    cpu,
		Memory: memory,
		Disk:   disk,
	}
}
