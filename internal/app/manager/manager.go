package manager

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

const (
	UpdateTasksPeriod = 15 * time.Second
	SendTasksPeriod = 15 * time.Second
)

type Manager struct {
	State *State
	WorkerCluster *Cluster

	// Queue of pending taskevents 
	Pending queue.Queue
	Scheduler scheduler.Scheduler
}

// Loading the DBs and process them in memory  
// Infer the rest of the fields before
// Creating a new manager struct with no HTTP connections between workers
func New(scheduler scheduler.Scheduler, dbType string) *Manager {
	loadedStates := NewState(dbType)
	createdCluster := NewCluster()

	m := &Manager{
		State: loadedStates,
		WorkerCluster: createdCluster,
		
		Pending: *queue.New(),
		Scheduler: scheduler,
	}

	return m
}

// Iterate through all the UUIDs in the current state of the manager 
// Fire http requests to the clients in parallel 
// And Update the stats of the nodes 
func (m *Manager) UpdateWorkerNodes() {
	m.WorkerCluster.UpdateWorkerNodes(m.State)
}

// Get the HTTP connection for a worker
// Throw an error if client is not in the map
func (m *Manager) getWorkerClient(workerId uuid.UUID) (*http.Client, error) {
	return m.WorkerCluster.GetClient(workerId)
}

// Register a worker to the manager state
// And put the client to the map 
func (m *Manager) AddWorkerAndClient(w worker.Worker, client *http.Client) {
	m.State.RegisterWorker(w.Uuid, w.Name)
	m.WorkerCluster.AddClient(w.Uuid, client)
}

/* 
Update the stats of all the workers
Then select the worker using the scheduler in the manager, with these new stats

Throw an error if no candidate node is found 
*/
func (m *Manager) SelectWorker(t task.Task) (uuid.UUID, error) {
	// Update the stats of all the nodes
	m.UpdateWorkerNodes()

	candidates := m.Scheduler.SelectCandidateNodes(t, m.WorkerCluster.WorkerNodes)

	if candidates == nil {
		msg := fmt.Sprintf("No available candidates match resource request for task %v", t.ID)
		err := errors.New(msg)
		return uuid.Nil, err
	}

	scores := m.Scheduler.Score(t, candidates)
	selectedNode := m.Scheduler.Pick(scores, candidates)
	return selectedNode.WorkerUuid, nil
}

// For worker
// Sync the states of the tasks in the manager with the workers 
func (m *Manager) updateTasks() {
	m.WorkerCluster.SyncTasks(m.State)
}

// For worker
// Dequeue and send the `taskEvent` 
// when the "Pending" queue is not empty 
func (m *Manager) SendWork() {
	if (m.Pending.Len() == 0) {
		log.Println("No work in the queue")
		return
	}

	// Dequeue from the pending queue 
	taskEvent := m.Pending.Dequeue().(task.TaskEvent)
	err := m.State.EventDb.Put(taskEvent.ID.String(), &taskEvent)
	if (err != nil) {
		log.Printf("Error putting pending task event to the EventDb: %v\n", err)	 
		return
	}

	// Find the worker already running the task 
	// or use Scheduler to determine the worker 
	assignedWorkerId, err := m.determineWorker(taskEvent)
	if (err != nil) {
		log.Printf("Failed to schedule the task: %v\n", err)
		return 
	}

	// Work is to stop the task 
	if (taskEvent.Task.State == task.Completed) {
		m.stopTask(taskEvent.Task.ID)
		return 
	}

	// Assign task to the worker 
	// And store the assignment relations 
	if err := m.State.AssignTaskToWorker(&taskEvent.Task, assignedWorkerId); err != nil {
		log.Printf("Error in DB assigning task to worker: %v\n", err)
		return 
	}

	updatedTask, err := m.WorkerCluster.SendTask(assignedWorkerId, taskEvent)		
	if (err != nil) {
		log.Printf("Error sending task to target worker %s: %v\n", assignedWorkerId , err)

		// Mark the task as failed 
		taskEvent.Task.State = task.Failed
		m.State.UpdateTask(&taskEvent.Task)

		return 
	}

	log.Printf("Task %s successfully deployed to worker %s\n", updatedTask.ID.String(), assignedWorkerId)
}

// Retrieve worker from the taskWorkerDb 
// if the task is already assigned to a worker 

// Or use the scheduler to determine the best worker for the task
func (m *Manager) determineWorker(taskEvent task.TaskEvent)  (uuid.UUID, error) {
	assignedWorker, _ := m.State.TaskWorkerDb.Get(taskEvent.Task.ID.String())
	// If the task is already running
	// Select worker we retreived 
	if (assignedWorker != nil) {
		return *assignedWorker, nil
	}

	return m.SelectWorker(taskEvent.Task)
}

// For user 
func (m *Manager) AddTaskEvent(te task.TaskEvent) {
	m.Pending.Enqueue(te)
}

func (m *Manager) stopTask(taskId uuid.UUID) {
	err := m.WorkerCluster.StopTask(m.State, taskId) 
	if (err != nil) {
		fmt.Printf("Error stopping task: %s", err.Error())
	}
}

// Runs an infinite loop to sync the state of the manager with the workers 
func (m *Manager) UpdateTasksForever() {
	for {
		fmt.Printf("[Manager] Updating tasks from %d workers\n", len(m.State.Workers))
		m.updateTasks()
		time.Sleep(UpdateTasksPeriod)
	}
}

// Runs an infinite loop to send existing tasks to the workers 
func (m *Manager) SendTasksForever() {
	for {
		if (m.Pending.Len() == 0) {
			log.Println("No tasks to send, sleeping for 10 seconds")
			time.Sleep(SendTasksPeriod)
			continue
		}

		numOfTasksToSend := m.Pending.Len()

		for range(numOfTasksToSend) {
			m.SendWork()
		}
	}
}

func (m *Manager) GetNodes() []node.Node {
	return m.WorkerCluster.GetNodes() 
}