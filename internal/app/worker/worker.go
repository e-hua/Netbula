package worker

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/e-hua/netbula/internal/docker"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

type Worker struct {
	Name string 
	Uuid uuid.UUID

	// TODO: Implement a generic queue 
	// Represents the "desired states" of tasks
	Queue queue.Queue
	// TODO: Use a persistent DB to store the tasks 
	// Represents the "current states" of tasks
	
	// Should not be accessible outside the worker package
	taskMap map[uuid.UUID]*task.Task
	TaskCount int 
}

func NewWorker(name string, Queue queue.Queue, taskMap map[uuid.UUID]*task.Task) *Worker {
	return &Worker {
		Name: name,
		Uuid: uuid.New(),
		Queue: Queue,
		taskMap: taskMap,
	}
}

func (w *Worker) CollectStats() {
	fmt.Println("Stats collected!")
}

func (w *Worker) runTask() docker.DockerResult {
	targetTask := w.Queue.Dequeue()	
	if (targetTask == nil) {
		log.Println("No tasks in the queue")
		return docker.DockerResult{Error: nil}
	}


	// Type assertion: Panic if not of type Task
	taskQueued := targetTask.(task.Task)	
	taskPersisted := w.taskMap[taskQueued.ID]

	// New task added to the worker
	if (taskPersisted == nil) {
		taskPersisted = &taskQueued
		w.taskMap[taskQueued.ID] = taskPersisted
	}

	var result docker.DockerResult
	if (task.ValidStateTransition(taskPersisted.State, taskQueued.State)) {
		switch taskQueued.State {

		case task.Scheduled:
			result = w.StartTask(taskQueued)
		case task.Completed: 
			result = w.StopTask(taskQueued)
		default: 
			result.Error = errors.New("Not implemented yet")
		}
	} else {
		err := fmt.Errorf(
			"Invalid transition from %v to %v", 
			taskPersisted.State,
			taskQueued.State,
		)
		result.Error = err
	}

	return result
}

func (w *Worker) StartTask(taskToStart task.Task) docker.DockerResult {
	taskToStart.StartTime = time.Now().UTC()
	config := task.NewConfig(&taskToStart)
	newDocker := docker.NewDocker(config)
	result := newDocker.Run()

	if (result.Error != nil) {
		log.Printf(
			"Error starting container %v: %v\n", 
			taskToStart.ContainerID, 
			result.Error,
		)
	}

	taskToStart.ContainerID = result.ContainerId
	taskToStart.State = task.Running
	
	w.taskMap[taskToStart.ID] = &taskToStart

	return result
}

// Requires the parameter `taskToStop` to have a containerID
func (w *Worker) StopTask(taskToStop task.Task) docker.DockerResult {
	config := task.NewConfig(&taskToStop)
	newDocker := docker.NewDocker(config)

	result := newDocker.Stop(taskToStop.ContainerID)
	if (result.Error != nil) {
		log.Printf(
			"Error stopping container %v: %v\n", 
			taskToStop.ContainerID, 
			result.Error,
		)
	}

	taskToStop.FinishTime = time.Now().UTC()
	taskToStop.State = task.Completed

	w.taskMap[taskToStop.ID] = &taskToStop
	log.Printf(
		"Stopped and removed container %v for task %v \n", 
		taskToStop.ContainerID, 
		taskToStop.ID,
	)
	
	return result
}

func (w *Worker) AddTask(taskToAdd task.Task) {
	w.Queue.Enqueue(taskToAdd)
}

func (w *Worker) GetTasks() map[uuid.UUID]task.Task {
	tasks := make(map[uuid.UUID]task.Task)
	
	for uuid, task := range(w.taskMap) {
		tasks[uuid]	= *task
	}
	return tasks
}

func (w *Worker) RunTasksForever() {
	for {
		if (w.Queue.Len() != 0) {
			result := w.runTask()
			if (result.Error != nil) {
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

	for currTaskId, currTask := range(w.taskMap) {
		newConfig := task.NewConfig(currTask)
		newDocker := docker.NewDocker(newConfig)

		if (currTask.State == task.Running) {
			resp := w.InspectTask(*currTask, newDocker)
			
			// Container removed
			if (resp.Container == nil) {
				log.Printf("No container for running task %s\n", currTaskId)
				w.taskMap[currTaskId].State = task.Failed
				continue
			}

			// Container is not running 
			if (
				resp.Container.State.Status != "running" && 
				resp.Container.State.Status != "created" && 
				resp.Container.State.Status != "restarting") {
					w.taskMap[currTaskId].State = task.Failed
			}

			w.taskMap[currTaskId].PortBindings = resp.Container.NetworkSettings.Ports
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