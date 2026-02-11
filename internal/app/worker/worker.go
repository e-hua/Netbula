package worker

import (
	"fmt"

	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

type Worker struct {
	Name string 
	Queue queue.Queue
	TaskMap map[uuid.UUID]task.Task
	TaskCount int 
}

func (w *Worker) CollectStats() {
	fmt.Println("Stats collected!")
}

func (w *Worker) RunTask() {
	fmt.Println("Task runned!")
}

func (w *Worker) StartTask() {
	fmt.Println("Task started!")
}

func (w *Worker) StopTask() {
	fmt.Println("Task stopped!")
}