package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/e-hua/netbula/internal/store"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const (
	UpdateTasksPeriod = 15 * time.Second
	SendTasksPeriod   = 15 * time.Second
)

type Cluster interface {
	UpdateWorkerNodes(state State)
	SyncTasks(state State)

	SendTask(targetWorkerId uuid.UUID, taskEvent task.TaskEvent) (*task.Task, error)
	StopTask(targetWorkerId uuid.UUID, taskIdToStop uuid.UUID) error

	GetClient(workerId uuid.UUID) (*http.Client, error)
	AddClient(workerId uuid.UUID, client *http.Client)

	GetNodes() []node.Node
}

type State interface {
	GetWorkerMetadata(id uuid.UUID) (workerName string, taskCount int)
	GetWorkerIds() []uuid.UUID

	RegisterWorker(workerUuid uuid.UUID, workerName string) error

	GetTasks() ([]*task.Task, error)
	UpdateTask(taskToUpdate *task.Task) error
	GetTask(taskId uuid.UUID) (*task.Task, error)

	UpdateTaskEvent(taskEventToUpdate task.TaskEvent) error

	AssignTaskToWorker(taskToAssign task.Task, workerId uuid.UUID) (*task.Task, error)
	GetAssignedWorker(taskId uuid.UUID) (workerId *uuid.UUID, err error)
}

type Manager struct {
	State         State
	WorkerCluster Cluster

	// Queue of pending TaskEvents
	Pending   queue.Queue
	Scheduler scheduler.Scheduler

	ManagerLogger logger.ManagerLogger
}

func CreatePersistentStateStores(managerDbPath string, managerDbFileMode os.FileMode) (*stateStores, error) {
	db, err := bolt.Open(managerDbPath, managerDbFileMode, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open the persistent DB for manager: %w", err)
	}

	persistentTaskDb, err := store.NewPersistentStore[task.Task](db, "tasks")
	if err != nil {
		return nil, fmt.Errorf("failed to construct the persistent task DB: %w", err)
	}

	persistentTaskEventDb, err := store.NewPersistentStore[task.TaskEvent](db, "events")
	if err != nil {
		return nil, fmt.Errorf("failed to construct the persistent task event DB: %w", err)
	}

	persistentTaskToWorkerDb, err := store.NewPersistentStore[uuid.UUID](db, "task_to_worker")
	if err != nil {
		return nil, fmt.Errorf("failed to construct the persistent task to worker DB: %w", err)
	}

	persistentWorkerNameDb, err := store.NewPersistentStore[string](db, "worker_id_to_name")
	if err != nil {
		return nil, fmt.Errorf("failed to construct the persistent worker id to name DB: %w", err)
	}

	return &stateStores{
		taskDb:       persistentTaskDb,
		eventDb:      persistentTaskEventDb,
		taskWorkerDb: persistentTaskToWorkerDb,
		workerNameDb: persistentWorkerNameDb,
	}, nil
}

func CreateInMemoryStores() *stateStores {
	taskStorage := store.NewInMemoryStore[task.Task]()
	taskEventStorage := store.NewInMemoryStore[task.TaskEvent]()
	taskToWorkerStorage := store.NewInMemoryStore[uuid.UUID]()
	workerNameStorage := store.NewInMemoryStore[string]()

	return &stateStores{
		taskDb:       taskStorage,
		eventDb:      taskEventStorage,
		taskWorkerDb: taskToWorkerStorage,
		workerNameDb: workerNameStorage,
	}
}

// Loading the DBs and process them in memory
// Infer the rest of the fields before
// Creating a new manager struct with no HTTP connections between workers
func New(scheduler scheduler.Scheduler, managerLogger logger.ManagerLogger, stateStores *stateStores) (*Manager, error) {
	loadedStates, err := NewState(*stateStores, *logger.NewManagerLoggerWithSubsystem(managerLogger, "state"))
	if err != nil {
		return nil, fmt.Errorf("failed to create the State component storing mappings: %w", err)
	}

	createdCluster := NewCluster(*logger.NewManagerLoggerWithSubsystem(managerLogger, "worker_cluster"))

	m := NewManager(loadedStates, createdCluster, scheduler, managerLogger)

	return m, nil
}

// Dummy function for testing
func NewManager(state State, cluster Cluster, scheduler scheduler.Scheduler, managerLogger logger.ManagerLogger) *Manager {
	return &Manager{
		State:         state,
		WorkerCluster: cluster,

		Pending:   *queue.New(),
		Scheduler: scheduler,

		ManagerLogger: managerLogger,
	}
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
	err := m.State.RegisterWorker(workerInfo.Uuid, workerInfo.Name)
	if err != nil {
		m.ManagerLogger.Error(
			"Failed to register name of the worker",
			"error", err,
			"worker_id", workerInfo.Uuid.String(),
			"worker_name", workerInfo.Name,
		)
		return
	}

	m.WorkerCluster.AddClient(workerInfo.Uuid, client)
}

/*
Update the stats of all the workers
Then select the worker using the scheduler in the manager, with these new stats

Throw an error if no candidate node is found
*/
func (m *Manager) SelectWorker(t task.Task) (uuid.UUID, error) {
	// Update the stats of all the nodes
	m.UpdateWorkerNodes()

	candidates := m.Scheduler.SelectCandidateNodes(t, m.WorkerCluster.GetNodes())

	if len(candidates) == 0 {
		msg := fmt.Sprintf("no available candidates match resource request for task `%s`", t.ID.String())
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

	// This is like a log for the taskEvent we removed from the queue
	err := m.State.UpdateTaskEvent(taskEvent)
	if err != nil {
		retErr = fmt.Errorf("failed to put pending task event to EventDb: %w", err)
		return targetEvent, retErr
	}

	// TODO: Make `STOP` and other state transitions share the same API
	// Work is to stop the task
	if taskEvent.TargetState == task.Completed {
		err = m.stopTask(taskEvent.Task.ID)
		if err != nil {
			retErr = fmt.Errorf("error stopping task: %w", err)
			return targetEvent, retErr
		}

		m.ManagerLogger.TaskSent(&taskEvent)
		return targetEvent, nil
	}

	// Find the worker already running the task
	// or use Scheduler to determine the worker
	assignedWorkerId, err := m.determineWorker(taskEvent)
	if err != nil {
		retErr = fmt.Errorf("failed to schedule the task: %w", err)
		return targetEvent, retErr
	}

	// Assign task to the worker
	// And store the assignment relations
	if newAssignedTask, err := m.State.AssignTaskToWorker(taskEvent.Task, assignedWorkerId); err != nil {
		retErr = fmt.Errorf("failed to assign task to worker: %w", err)
		return targetEvent, retErr
	} else {
		targetEvent.Task = *newAssignedTask
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
	assignedWorker, _ := m.State.GetAssignedWorker(taskEvent.Task.ID)

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

func (m *Manager) stopTask(taskIdToStop uuid.UUID) error {
	workerId, err := m.State.GetAssignedWorker(taskIdToStop)

	if err != nil {
		return fmt.Errorf("failed to get workerId from db: %w", err)
	}

	if workerId == nil {
		return fmt.Errorf(
			"failed to get workerId related to taskId [%s] (probably because the key is not in the bucket): %w",
			taskIdToStop.String(),
			err,
		)
	}

	return m.WorkerCluster.StopTask(*workerId, taskIdToStop)
}

// Runs an infinite loop to sync the state of the manager with the workers
func (m *Manager) UpdateTasksForever(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		m.updateTasks()
		time.Sleep(UpdateTasksPeriod)
	}
}

// Runs an infinite loop to send existing tasks to the workers
func (m *Manager) SendTasksForever(ctx context.Context) {
	for {
		numOfTasksToSend := m.Pending.Len()

		if numOfTasksToSend == 0 {
			if ctx.Err() != nil {
				return
			}
			m.ManagerLogger.Debug("No tasks to send, sleeping for 10 seconds")
			time.Sleep(SendTasksPeriod)
			continue
		}

		for range numOfTasksToSend {
			if ctx.Err() != nil {
				return
			}
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

func (m *Manager) GetTasks() ([]*task.Task, error) {
	return m.State.GetTasks()
}

func (m *Manager) GetTask(taskId uuid.UUID) (*task.Task, error) {
	return m.State.GetTask(taskId)
}
