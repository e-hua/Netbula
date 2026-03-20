package task

import (
	"time"

	"github.com/e-hua/netbula/internal/docker"
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

type TaskEvent struct {
	ID          uuid.UUID
	TargetState State
	Timestamp   time.Time
	Task        Task
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
