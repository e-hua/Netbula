package task

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/e-hua/netbula/internal/docker"
	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/network"
)

type State int

const (
	// Initial state for every task
	Pending State = iota // State = 0
	// Once the manager has scheduled it onto a worker
	Scheduled // Scheduled = 1
	// Once a worker successfully starts the task
	Running // ...
	// Once the task completes its work
	Completed
	// If task fails
	Failed
)

func (s State) String() string {
	switch s {
	case Pending:
		return "PENDING"
	case Scheduled:
		return "SCHEDULED"
	case Running:
		return "RUNNING"
	case Completed:
		return "COMPLETED"
	case Failed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// Think of this as an adjacent list
// In the graph representing the state machine
var stateTransitionMap = map[State][]State{
	Pending:   {Scheduled},
	Scheduled: {Scheduled, Running, Failed},
	Running:   {Running, Completed, Failed},
	Completed: {},
	Failed:    {},
}

func contains(states []State, state State) bool {
	for _, s := range states {
		if s == state {
			return true
		}
	}
	return false
}

func ValidStateTransition(src State, dest State) bool {
	return contains(stateTransitionMap[src], dest)
}

// Acts as an abstraction for a container
type Task struct {
	ID uuid.UUID

	Name   string
	State  State
	Image  string
	Cpu    float64
	Memory int
	Disk   int

	ExposedPorts  network.PortSet
	PortBindings  network.PortMap
	RestartPolicy string

	StartTime  time.Time
	FinishTime time.Time

	ContainerID string
}

func (t Task) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("task_id", t.ID.String()),
		slog.String("name", t.Name),
		slog.String("state", t.State.String()),
		slog.String("image", t.Image),
		slog.Float64("cpu", t.Cpu),
		slog.String("memory", fmt.Sprintf("%.2fGB", float64(t.Memory)/float64(types.GigabyteInBytes))),
		slog.String("disk", fmt.Sprintf("%.2fGB", float64(t.Disk)/float64(types.GigabyteInBytes))),
		slog.String("restart_policy", t.RestartPolicy),
		slog.Time("start_time", t.StartTime),
		slog.Time("finish_time", t.FinishTime),
		slog.String("container_id", t.ContainerID),

		slog.Any("exposed_ports", t.ExposedPorts),
		slog.Any("port_bindings", t.PortBindings),
	)
}

type TaskEvent struct {
	ID          uuid.UUID
	TargetState State
	Timestamp   time.Time
	Task        Task
}

func (te TaskEvent) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("event_id", te.ID.String()),
		slog.String("target_state", te.TargetState.String()),
		slog.Time("timestamp", te.Timestamp),
		slog.Any("task", te.Task),
	)
}

func NewConfig(task *Task) docker.Config {
	return docker.Config{
		Name: task.Name,

		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,

		ExposedPorts: task.ExposedPorts,

		Cmd:   make([]string, 0),
		Image: task.Image,

		Cpu:           task.Cpu,
		Memory:        task.Memory,
		Disk:          task.Disk,
		Env:           make([]string, 0),
		RestartPolicy: task.RestartPolicy,
	}
}
