package node

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/google/uuid"
)

// The physical aspect of the Worker
type Node struct {
	Name string

	Cores             int
	CpuAveragePercent float64
	CpuAverageLoad    float64

	Memory                 int
	MemoryAllocatedPercent float64

	Disk                 int
	DiskAllocatedPercent float64

	Role      string
	TaskCount int

	WorkerUuid uuid.UUID
}

func (n Node) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("worker_uuid", n.WorkerUuid.String()),
		slog.String("name", n.Name),
		slog.Int("cores", n.Cores),
		slog.String("CPU_usage", fmt.Sprintf("%.2f%%", n.CpuAveragePercent)),
		slog.Float64("CPU_load_index", n.CpuAverageLoad/float64(n.Cores)),

		slog.String("memory", fmt.Sprintf("%.2fGB", float64(n.Memory)/float64(types.GigabyteInBytes))),
		slog.String("memory_usage", fmt.Sprintf("%.2f%%", n.MemoryAllocatedPercent)),
		slog.String("disk", fmt.Sprintf("%.2fGB", float64(n.Disk)/float64(types.GigabyteInBytes))),
		slog.String("disk_usage", fmt.Sprintf("%.2f%%", n.DiskAllocatedPercent)),

		slog.String("role", n.Role),
		slog.Int("task_count", n.TaskCount),
	)
}

func (node *Node) PrintNode() {
	fmt.Printf("Worker node [%v] status: \n", node.Name)
	log.Printf(
		"RAM: %.2f GB, RAM usage: %.2f%%, Disk: %.2f GB, Disk usage: %.2f%%, CPU: %d cores, Average CPU usage: %.2f%%, CPU load index: %v\n\n",
		float64(node.Memory)/float64(types.GigabyteInBytes),
		node.MemoryAllocatedPercent,
		float64(node.Disk)/float64(types.GigabyteInBytes),
		node.DiskAllocatedPercent,
		node.Cores,
		node.CpuAveragePercent,
		node.CpuAverageLoad/float64(node.Cores),
	)
}
