package manager

import (
	"fmt"

	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

type Manager struct {
	Pending queue.Queue
	
	TaskMap map[string][]*task.Task
	EventMap map[string][]*task.TaskEvent

	Workers []string
	
	// Maybe we should introduce relational DB here 
	WorkerTaskMap map[string][]uuid.UUID
	TaskWorkerMap map[uuid.UUID]string
}

func (m *Manager) SelectWorker() {
	fmt.Println("Worker selected")
}
func (m *Manager) UpdateTasks() {
	fmt.Println("Task updated")
}
func (m *Manager) SendWork() {
	fmt.Println("Work sent to the worker")
}