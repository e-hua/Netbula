package main

import (
	"fmt"
	"time"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

func main() {
	newTask := task.Task {
		ID: uuid.New(),
		Name: "Task-1",
		State: task.Pending,
		Image: "Image-1",
		Memory: 1024,
		Disk: 1,
	}

	taskEvent := task.TaskEvent {
		ID: uuid.New(),
		TargetState: task.Pending,
		Timestamp: time.Now(),
		Task: newTask,
	}

	fmt.Printf("New task: %v\n", newTask)
	fmt.Printf("Task event: %v\n", taskEvent)


	newWorker := worker.Worker {
		Name: "Worker-1",
		Queue: *queue.New(),
		TaskMap: make(map[uuid.UUID]*task.Task),
	}

	fmt.Printf("New worker: %v\n", newWorker)
	newWorker.CollectStats()
	newWorker.RunTask()
	newWorker.StartTask()
	newWorker.StopTask()


	newManager := manager.Manager {
		Pending: *queue.New(),	

		TaskMap: make(map[string][]*task.Task),
		EventMap: make(map[string][]*task.TaskEvent),

		Workers: []string {newWorker.Name},
	}

	fmt.Printf("New manager: %v\n", newManager)
	newManager.SelectWorker()
	newManager.UpdateTasks()
	newManager.SendWork()


	newNode := node.Node {
		Name: "Node-1",
		Cores: 4,
		Memory: 1024,
		Disk: 25,
		Role: "worker",
	}

	fmt.Printf("New node: %v\n", newNode)
}