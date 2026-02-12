package task

import (
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
)

type State int 

const (
	Pending State = iota // State = 0
	Scheduled  // Scheduled = 1
	Running  // ...
	Completed
	Failed
)

// Acts as an abstraction for a container
type Task struct {
	ID uuid.UUID

	Name string 
	State State
	Image string 
	Memory int 
	Disk int 

	ExposedPorts nat.PortSet
	PortBindings map[string]string
	RestartPolicy string

	StartTime time.Time
	FinishTime time.Time
}

type TaskEvent struct {
	ID uuid.UUID
	TargetState State
	Timestamp time.Time
	Task Task
}