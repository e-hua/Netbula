package manager

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

const (
	UpdateTasksPeriod = 15 * time.Second
	SendTasksPeriod   = 15 * time.Second
)

type Manager struct {
	State         *State
	WorkerCluster *Cluster

	// Queue of pending taskevents
	Pending   queue.Queue
	Scheduler scheduler.Scheduler

	ManagerLogger logger.ManagerLogger
}

// Loading the DBs and process them in memory
// Infer the rest of the fields before
// Creating a new manager struct with no HTTP connections between workers
func New(scheduler scheduler.Scheduler, dbType string, managerLogger logger.ManagerLogger) *Manager {
	loadedStates := NewState(dbType)
	createdCluster := NewCluster(*logger.NewManagerLoggerWithSubsystem(managerLogger, "worker_cluster"))

	m := &Manager{
		State:         loadedStates,
		WorkerCluster: createdCluster,

		Pending:   *queue.New(),
		Scheduler: scheduler,

		ManagerLogger: managerLogger,
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

// Register a worker
// And put the http client connected with the worker into the WorkerCluster
func (m *Manager) AddWorkerAndClient(workerInfo *worker.Worker, client *http.Client) {
	m.State.RegisterWorker(workerInfo.Uuid, workerInfo.Name)
	m.WorkerCluster.AddClient(workerInfo.Uuid, client)
	// TODO: Log here to indicate the worker is connected
	// Since is different from "disappearance" and "appearance" of nodes?
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
		msg := fmt.Sprintf("no available candidates match resource request for task %v", t.ID)
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
func (m *Manager) SendWork() (targetEvent *task.TaskEvent, retErr error) {
	if m.Pending.Len() == 0 {
		retErr = errors.New("no TaskEvent left in the pending queue")
		return nil, retErr
	}

	// Dequeue from the pending queue
	taskEvent := m.Pending.Dequeue().(task.TaskEvent)
	targetEvent = &taskEvent

	// If we didn't manage to send the work
	// Mark a copy of task as failed and update it in all the states
	defer func(taskCopy task.Task) {
		if retErr != nil && targetEvent != nil {
			taskCopy.State = task.Failed
			m.State.UpdateTask(&taskCopy)
		}
	}(taskEvent.Task)

	err := m.State.EventDb.Put(taskEvent.ID.String(), &taskEvent)
	if err != nil {
		retErr = fmt.Errorf("failed to put pending task event to EventDb: %w", err)
		return targetEvent, retErr
	}

	// Find the worker already running the task
	// or use Scheduler to determine the worker
	assignedWorkerId, err := m.determineWorker(taskEvent)
	if err != nil {
		retErr = fmt.Errorf("failed to schedule the task: %w", err)
		return targetEvent, retErr
	}

	// TODO: Make `STOP` and other state transitions share the same API
	// Work is to stop the task
	if taskEvent.Task.State == task.Completed {
		err = m.stopTask(taskEvent.Task.ID)
		if err != nil {
			retErr = fmt.Errorf("error stopping task: %w", err)
			return targetEvent, retErr
		}

		m.ManagerLogger.TaskSent(&taskEvent)
		return targetEvent, nil
	}

	// Assign task to the worker
	// And store the assignment relations
	if err := m.State.AssignTaskToWorker(&taskEvent.Task, assignedWorkerId); err != nil {
		retErr = fmt.Errorf("failed to assigning task to worker: %w", err)
		return targetEvent, retErr
	}

	_, err = m.WorkerCluster.SendTask(assignedWorkerId, taskEvent)
	if err != nil {
		retErr = fmt.Errorf(
			"failed to send the task to target worker %s: %w", assignedWorkerId.String(), err)

		return targetEvent, retErr
	}

	// Logging: TaskEvent sent to worker successfully
	m.ManagerLogger.TaskSent(&taskEvent)
	return targetEvent, nil
}

// Retrieve worker from the taskWorkerDb
// if the task is already assigned to a worker

// Or use the scheduler to determine the best worker for the task
func (m *Manager) determineWorker(taskEvent task.TaskEvent) (uuid.UUID, error) {
	assignedWorker, _ := m.State.TaskWorkerDb.Get(taskEvent.Task.ID.String())
	// If the task is already running
	// Select worker we retreived
	if assignedWorker != nil {
		return *assignedWorker, nil
	}

	return m.SelectWorker(taskEvent.Task)
}

// For user
func (m *Manager) AddTaskEvent(te task.TaskEvent) {
	m.Pending.Enqueue(te)
}

func (m *Manager) stopTask(taskId uuid.UUID) error {
	return m.WorkerCluster.StopTask(m.State, taskId)
}

// Runs an infinite loop to sync the state of the manager with the workers
func (m *Manager) UpdateTasksForever() {
	for {
		m.updateTasks()
		time.Sleep(UpdateTasksPeriod)
	}
}

// Runs an infinite loop to send existing tasks to the workers
func (m *Manager) SendTasksForever() {
	for {
		if m.Pending.Len() == 0 {
			m.ManagerLogger.Debug("No tasks to send, sleeping for 10 seconds")
			time.Sleep(SendTasksPeriod)
			continue
		}

		numOfTasksToSend := m.Pending.Len()

		for range numOfTasksToSend {
			targetEvent, err := m.SendWork()
			if err != nil {
				// Logging: Failed to send the taskEvent to worker
				m.ManagerLogger.Error("Failed to send taskEvent to worker", "error", err, "target_event", targetEvent)
			}
		}
	}
}

func (m *Manager) GetNodes() []node.Node {
	return m.WorkerCluster.GetNodes()
}
