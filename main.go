package main

import (
	"fmt"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/docker"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
)

func main() {
	/*
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


	fmt.Printf("Start creating a test container\n")
	dockerTask, createResult := createContainer()
	if createResult.Error != nil {
		fmt.Printf("%v", createResult.Error)
		os.Exit(1)
	}

	fmt.Printf("stopping container %s\n", createResult.ContainerId)
	_ = stopContainer(dockerTask, createResult.ContainerId)
	*/

	db := make(map[uuid.UUID]*task.Task)
	newWorker := worker.NewWorker("", *queue.New(), db)

	newTask := task.Task {
		ID: uuid.New(),
		Name: "test-container-1",
		State: task.Scheduled,
		Image: "strm/helloworld-http",
	}

	fmt.Println("starting task")
	newWorker.AddTask(newTask)

	result := newWorker.RunTask()
	if (result.Error != nil) {
		panic(result.Error)
	}

	newTask.ContainerID = result.ContainerId
	fmt.Printf("task %s is running in the container %s\n", newTask.ID, newTask.ContainerID)


	// time.Sleep(30 * time.Second)


	fmt.Printf("stopping task %s\n", newTask.ID)
	newTask.State = task.Completed
	newWorker.AddTask(newTask)

	result = newWorker.RunTask()
	if (result.Error != nil) {
		panic(result.Error)
	}
}

func createContainer() (*docker.Docker, *docker.DockerResult) {
	dockerConfig := docker.Config{
		Name: "test-container-1",
		Image: "postgres:18",
		Env: []string{
		"POSTGRES_USER=netbula",
		"POSTGRES_PASSWORD=secret",
		},
	}

	apiClient, _ := client.New(client.FromEnv)
	newDocker := docker.Docker{
		Client: apiClient,
		Config: dockerConfig,
 	}

	result := newDocker.Run()
	if (result.Error != nil) {
		fmt.Printf("%v\n", result.Error)
		return nil, nil
	}

	fmt.Printf(
		"Container %s is running with config %v\n", 
		result.ContainerId, 
		dockerConfig,
	)

	return &newDocker, &result
}

func stopContainer(docker *docker.Docker, id string) *docker.DockerResult {
	result := docker.Stop(id)
	if (result.Error != nil) {
		fmt.Printf("%v\n", result.Error)
		return nil		
	}
	
	fmt.Printf("Container %s has been stopped and removed\n", result.ContainerId)
	return &result
}