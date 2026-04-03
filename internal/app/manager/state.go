package manager

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sync"

	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/store"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const (
	ManagerDbPath                 = "manager.db"
	ManagerDbFileMode os.FileMode = 0600
)

// Stored and inferred states of manager
type State struct {
	// Need to include mutex to prevent multiple writes at the same time
	mutex sync.RWMutex

	TaskDb       store.Store[task.Task]      // uuid -> task
	EventDb      store.Store[task.TaskEvent] // uuid -> taskEvent
	TaskWorkerDb store.Store[uuid.UUID]      // TaskUuid -> WorkerUuid
	WorkerNameDb store.Store[string]         // uuid -> WorkerName

	// These are the info inferred from the TaskWorkerDb on start of the application
	Workers       []uuid.UUID
	WorkerTaskMap map[uuid.UUID][]uuid.UUID

	stateLogger logger.ManagerLogger
}

// Load the storage to the manager object
// Panic if any error appears
func NewState(dbType string, stateLogger logger.ManagerLogger) (*State, error) {
	var taskStorage store.Store[task.Task]
	var taskEventStorage store.Store[task.TaskEvent]
	var taskToWorkerStorage store.Store[uuid.UUID]
	var workerNameStorage store.Store[string]

	switch dbType {
	case "memory":
		taskStorage = store.NewInMemoryStore[task.Task]()
		taskEventStorage = store.NewInMemoryStore[task.TaskEvent]()
		taskToWorkerStorage = store.NewInMemoryStore[uuid.UUID]()
		workerNameStorage = store.NewInMemoryStore[string]()
	case "persistent":
		db, err := bolt.Open(ManagerDbPath, ManagerDbFileMode, nil)
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

		taskStorage = persistentTaskDb
		taskEventStorage = persistentTaskEventDb
		taskToWorkerStorage = persistentTaskToWorkerDb
		workerNameStorage = persistentWorkerNameDb
	}

	newState := &State{
		TaskDb:       taskStorage,
		EventDb:      taskEventStorage,
		TaskWorkerDb: taskToWorkerStorage,
		WorkerNameDb: workerNameStorage,
		stateLogger:  stateLogger,
	}

	err := newState.rehydrate()
	if err != nil {
		return nil, fmt.Errorf("failed to rehydrate the State component with other derived attributes: %w", err)
	}

	return newState, nil
}

// Infer the fields like Workers and WorkerTaskMap from loaded storage
func (state *State) rehydrate() error {
	// Locked entirely, other cannot read or write
	state.mutex.Lock()
	defer state.mutex.Unlock()

	taskWorkerEntries, err := state.TaskWorkerDb.Entries()
	if err != nil {
		return fmt.Errorf("failed to get entries from the DB mapping tasks to workers: %w", err)
	}

	workerTaskMap := make(map[uuid.UUID][]uuid.UUID)
	workers := make([]uuid.UUID, 0)

	// Infer the content of workerTaskMap from taskWorkerDb
	for _, taskWorkerEntry := range taskWorkerEntries {
		taskUuid, err := uuid.Parse(taskWorkerEntry.Key)
		if err != nil {
			return fmt.Errorf("failed to parse the task UUID (%s) from the TaskWorker DB: %w", taskWorkerEntry.Key, err)
		}

		workerUuid := *taskWorkerEntry.Value

		assignedTaskSlice, ok := workerTaskMap[workerUuid]
		if !ok {
			assignedTaskSlice = make([]uuid.UUID, 0)
		}

		assignedTaskSlice = append(assignedTaskSlice, taskUuid)
		workerTaskMap[workerUuid] = assignedTaskSlice
	}

	workerNameEntries, err := state.WorkerNameDb.Entries()
	if err != nil {
		return fmt.Errorf("failed to get entries from the DB mapping worker UUID to worker names: %w", err)
	}

	for _, workerNameEntry := range workerNameEntries {
		workerUuid, err := uuid.Parse(workerNameEntry.Key)
		if err != nil {
			return fmt.Errorf("failed to parse the worker UUID (%s) from the WorkerName DB: %w", workerNameEntry.Key, err)
		}

		assignedTaskSlice, ok := workerTaskMap[workerUuid]
		if !ok {
			assignedTaskSlice = make([]uuid.UUID, 0)
		}
		workerTaskMap[workerUuid] = assignedTaskSlice
	}

	// Infer the content of the workers from the workerTaskMap
	workers = slices.Collect(maps.Keys(workerTaskMap))

	state.Workers = workers
	state.WorkerTaskMap = workerTaskMap

	return nil
}

// Get the name and the number of tasks of a worker
func (state *State) GetWorkerMetadata(id uuid.UUID) (workerName string, taskCount int) {
	// Locks for reading, other cannot write
	state.mutex.RLock()
	defer state.mutex.RUnlock()

	name, _ := state.WorkerNameDb.Get(id.String())
	taskCount = len(state.WorkerTaskMap[id])

	if name == nil {
		return "unknown", taskCount
	}

	return *name, taskCount
}

// Get the UUID of the workers
func (state *State) GetWorkerIds() []uuid.UUID {
	// Locks for reading, other cannot write
	state.mutex.RLock()
	defer state.mutex.RUnlock()

	// Returns a copy of Ids
	return append([]uuid.UUID(nil), state.Workers...)
}

// Add the name and the UUID of the worker to the state of the manager
func (state *State) RegisterWorker(workerUuid uuid.UUID, workerName string) error {
	// Locks entirely
	state.mutex.Lock()
	defer state.mutex.Unlock()

	name, err := state.WorkerNameDb.Get(workerUuid.String())

	// If DB is tripping
	if err != nil {
		return fmt.Errorf("failed to read from the WorkerNameDb: %w", err)
	}

	err = state.WorkerNameDb.Put(workerUuid.String(), &workerName)
	if err != nil {
		return fmt.Errorf("failed to write to the WorkerNameDb: %w", err)
	}

	if name == nil {
		state.Workers = append(state.Workers, workerUuid)
		state.stateLogger.WorkerConnected(workerUuid)
	} else {
		state.stateLogger.WorkerReconnected(workerUuid)
	}

	return nil
}

func (state *State) UpdateTask(taskToUpdate *task.Task) error {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	existingTask, err := state.TaskDb.Get(taskToUpdate.ID.String())
	if err != nil {
		return fmt.Errorf(
			"failed to get task with UUID [%s] from TaskDb: %w",
			taskToUpdate.ID.String(),
			err,
		)
	}

	// If task from worker is not in the manager db
	if existingTask == nil {
		return fmt.Errorf("task with UUID [%s] not found in TaskDb", taskToUpdate.ID.String())
	}

	// Log the comparison between previous task and current task, if the state changed 
	// TODO: Change this implementation when more attributes are added 
	if existingTask.State != taskToUpdate.State {
		state.stateLogger.TaskStatusChanged(*existingTask, *taskToUpdate)
	}

	existingTask.State = taskToUpdate.State

	// TODO: implement these features 
	existingTask.StartTime = taskToUpdate.StartTime
	existingTask.FinishTime = taskToUpdate.FinishTime
	existingTask.ContainerID = taskToUpdate.ContainerID
	existingTask.PortBindings = taskToUpdate.PortBindings

	err = state.TaskDb.Put(taskToUpdate.ID.String(), existingTask)
	if err != nil {
		return fmt.Errorf("failed to put task with UUID [%s] into TaskDb: %w", taskToUpdate.ID.String(), err)
	}

	return nil
}

// Add task to TaskDb
// Link task to worker in TaskWorkerDb
// Link worker to task in WorkerTaskMap
func (state *State) AssignTaskToWorker(taskToAssign *task.Task, workerId uuid.UUID) error {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	taskToAssign.State = task.Scheduled

	if err := state.TaskDb.Put(taskToAssign.ID.String(), taskToAssign); err != nil {
		return fmt.Errorf("failed to add task [%s] to TaskDb: %w", taskToAssign.ID.String(), err)
	}

	if err := state.TaskWorkerDb.Put(taskToAssign.ID.String(), &workerId); err != nil {
		return fmt.Errorf("failed to add worker [%s] to TaskWorkerDb: %w", taskToAssign.ID.String(), err)
	}

	if !slices.Contains(state.WorkerTaskMap[workerId], taskToAssign.ID) {
		state.WorkerTaskMap[workerId] = append(state.WorkerTaskMap[workerId], taskToAssign.ID)
	}

	return nil
}
