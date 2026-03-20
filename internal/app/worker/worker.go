package worker

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/e-hua/netbula/internal/docker"
	"github.com/e-hua/netbula/internal/store"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

type Worker struct {
	Name string
	Uuid uuid.UUID

	// TODO: Implement a generic queue
	// Represents the "desired states" of tasks
	Queue queue.Queue

	// Should not be accessible outside the worker package
	taskDb store.Store[task.Task]

	mutex sync.RWMutex
}

const (
	WorkerDbPath                 = "worker.db"
	WorkerDbFileMode os.FileMode = 0600
)

func NewWorker(uuid uuid.UUID, name string, Queue queue.Queue, dbType string) *Worker {
	var taskDb store.Store[task.Task]

	switch dbType {
	case "memory":
		taskDb = store.NewInMemoryStore[task.Task]()
	case "persistent":
		db, err := bolt.Open(WorkerDbPath, WorkerDbFileMode, nil)
		if err != nil {
			log.Fatalf("Error creating the persistent DB for manager: %v\n", err)
		}

		persistentTaskDb, err := store.NewPersistentStore[task.Task](db, name+"_tasks")
		if err != nil {
			log.Fatalf("Error creating the persistent task DB: %v\n", err)
		}
		taskDb = persistentTaskDb
	}

	return &Worker{
		Name:   name,
		Uuid:   uuid,
		Queue:  Queue,
		taskDb: taskDb,
	}
}

// Execute task transition if possible
// Throw error in DockerResult if transition is invalid
func (w *Worker) transitionTask(taskQueued task.Task) docker.DockerResult {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	taskPersisted, err := w.taskDb.Get(taskQueued.ID.String())
	// Error reading from DB
	if err != nil {
		log.Printf("Error getting the task from Task DB: %s", err.Error())
		return docker.DockerResult{Error: err}
	}
	// New task added to the worker
	// No entry in DB
	if taskPersisted == nil {
		taskPersisted = &taskQueued
		w.taskDb.Put(taskQueued.ID.String(), taskPersisted)
	}

	if !task.ValidStateTransition(taskPersisted.State, taskQueued.State) {
		return docker.DockerResult{Error: fmt.Errorf("invalid transition")}
	}

	switch taskQueued.State {
	case task.Scheduled:
		return w.StartTask(taskQueued)
	case task.Completed:
		return w.StopTask(taskQueued)
	default:
		return docker.DockerResult{Error: errors.New("Unsupported state, not implemented yet")}
	}
}

// Dequeue from the queue of pending tasks
// And execute task (starting or stopping the task)
func (w *Worker) runTask() docker.DockerResult {
	targetTask := w.Queue.Dequeue()
	if targetTask == nil {
		log.Println("No tasks in the queue")
		return docker.DockerResult{Error: nil}
	}

	// Type assertion: Panic if it's not of type Task
	taskQueued := targetTask.(task.Task)

	return w.transitionTask(taskQueued)
}

func (w *Worker) StartTask(taskToStart task.Task) docker.DockerResult {
	taskToStart.StartTime = time.Now().UTC()
	config := task.NewConfig(&taskToStart)
	newDocker := docker.NewDocker(config)
	result := newDocker.Run()

	if result.Error != nil {
		log.Printf(
			"Error starting container %v: %v\n",
			taskToStart.ContainerID,
			result.Error,
		)
	}

	taskToStart.ContainerID = result.ContainerId
	taskToStart.State = task.Running

	w.taskDb.Put(taskToStart.ID.String(), &taskToStart)

	return result
}

// Requires the parameter `taskToStop` to have a containerID
func (w *Worker) StopTask(taskToStop task.Task) docker.DockerResult {
	config := task.NewConfig(&taskToStop)
	newDocker := docker.NewDocker(config)

	result := newDocker.Stop(taskToStop.ContainerID)
	if result.Error != nil {
		log.Printf(
			"Error stopping container %v: %v\n",
			taskToStop.ContainerID,
			result.Error,
		)
	}

	taskToStop.FinishTime = time.Now().UTC()
	taskToStop.State = task.Completed

	w.taskDb.Put(taskToStop.ID.String(), &taskToStop)
	log.Printf(
		"Stopped and removed container %v for task %v \n",
		taskToStop.ContainerID,
		taskToStop.ID,
	)

	return result
}

func (w *Worker) AddTask(taskToAdd task.Task) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.Queue.Enqueue(taskToAdd)
}

func (w *Worker) GetTasks() ([]*task.Task, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return w.taskDb.List()
}

func (w *Worker) RunTasksForever() {
	for {
		if w.Queue.Len() != 0 {
			result := w.runTask()
			if result.Error != nil {
				log.Printf("Error running task: %v\n", result.Error)
			}
		} else {
			log.Println("No tasks to run currently, sleeping for 10 seconds")
			time.Sleep(10 * time.Second)
		}
	}
}

func (w *Worker) InspectTask(t task.Task, docker docker.Docker) docker.DockerInspectResponse {
	return docker.Inspect(t.ContainerID)
}

// Check if all running tasks are actually being runned by Docker
func (w *Worker) updateTasks() {

	tasks, err := w.taskDb.List()
	if err != nil {
		log.Printf("Error getting list of tasks from db: %v\n", err)
		return
	}

	for currTaskId, currTask := range tasks {
		newConfig := task.NewConfig(currTask)
		newDocker := docker.NewDocker(newConfig)

		if currTask.State == task.Running {
			resp := w.InspectTask(*currTask, newDocker)

			taskInspected, _ := w.taskDb.Get(currTask.ID.String())
			// Container removed
			if resp.Container == nil {
				log.Printf("No container for running task %d\n", currTaskId)

				taskInspected.State = task.Failed
				w.taskDb.Put(taskInspected.ID.String(), taskInspected)
				continue
			}

			// Container is not running
			if resp.Container.State.Status != "running" &&
				resp.Container.State.Status != "created" &&
				resp.Container.State.Status != "restarting" {
				taskInspected.State = task.Failed
			}

			taskInspected.PortBindings = resp.Container.NetworkSettings.Ports
		}

	}
}

func (w *Worker) UpdateTaskStatsForever() {
	for {
		log.Println("Checking status of tasks")
		w.updateTasks()
		log.Println("Task updates completed")

		log.Println("Sleeping for 15 seconds")
		time.Sleep(15 * time.Second)
	}
}
