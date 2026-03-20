package manager

import (
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"sync"

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
}

// Load the storage to the manager object
// Panic if any error appears
func NewState(dbType string) *State {
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
			log.Fatalf("Error creating the persistent DB for manager: %v\n", err)
		}

		persistentTaskDb, err := store.NewPersistentStore[task.Task](db, "tasks")
		persistentTaskEventDb, err := store.NewPersistentStore[task.TaskEvent](db, "events")
		persistentTaskToWorkerDb, err := store.NewPersistentStore[uuid.UUID](db, "task_to_worker")
		persistentWorkerNameDb, err := store.NewPersistentStore[string](db, "worker_id_to_name")
		// TODO: Add error handling here

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
	}

	newState.rehydrate()
	return newState
}

// Infer the fields like Workers and WorkerTaskMap from loaded storage
func (state *State) rehydrate() {
	// Locked entirely, other cannot read or write
	state.mutex.Lock()
	defer state.mutex.Unlock()

	taskWorkerEntries, err := state.TaskWorkerDb.Entries()
	if err != nil {
		log.Printf("Error getting entries in the db mapping tasks to workers")
		taskWorkerEntries = make([]store.Entry[uuid.UUID], 0)
	}

	workerTaskMap := make(map[uuid.UUID][]uuid.UUID)
	workers := make([]uuid.UUID, 0)

	// Infer the content of workerTaskMap from taskWorkerDb
	for _, taskWorkerEntry := range taskWorkerEntries {
		taskUuid, err := uuid.Parse(taskWorkerEntry.Key)
		if err != nil {
			log.Printf("Error parsing the task UUID from the TaskWorker DB")
			continue
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
		log.Printf("Error getting entries in the db mapping worker UUID to worker names")
		workerNameEntries = make([]store.Entry[string], 0)
	}

	for _, workerNameEntry := range workerNameEntries {
		workerUuid, err := uuid.Parse(workerNameEntry.Key)
		if err != nil {
			log.Printf("Error parsing the task UUID from the WorkerNameDb")
			continue
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
func (state *State) RegisterWorker(workerUuid uuid.UUID, workerName string) {
	// Locks entirely
	state.mutex.Lock()
	defer state.mutex.Unlock()

	name, err := state.WorkerNameDb.Get(workerUuid.String())

	// Panic if DB is tripping
	if err != nil {
		log.Fatalf("Error reading from the WorkerNameDb: %v", err)
	}

	// Worker already stored in the map
	if name == nil {
		state.Workers = append(state.Workers, workerUuid)
	}

	err = state.WorkerNameDb.Put(workerUuid.String(), &workerName)
	if err != nil {
		log.Fatalf("Error writing to the WorkerNameDb: %v", err)
	}
}

func (state *State) UpdateTask(taskToUpdate *task.Task) error {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	existingTask, err := state.TaskDb.Get(taskToUpdate.ID.String())
	if err != nil {
		return fmt.Errorf(
			"Error getting task with ID %s from TaskDb: %v\n",
			taskToUpdate.ID.String(),
			err,
		)
	}

	// If task from worker is not in the manager db
	if existingTask == nil {
		return fmt.Errorf("Task %s not found in manager db", taskToUpdate.ID.String())
	}

	existingTask.State = taskToUpdate.State

	existingTask.StartTime = taskToUpdate.StartTime
	existingTask.FinishTime = taskToUpdate.FinishTime
	existingTask.ContainerID = taskToUpdate.ContainerID
	existingTask.PortBindings = taskToUpdate.PortBindings

	return state.TaskDb.Put(taskToUpdate.ID.String(), existingTask)
}

// Add task to TaskDb
// Link task to worker in TaskWorkerDb
// Link worker to task in WorkerTaskMap
func (state *State) AssignTaskToWorker(taskToAssign *task.Task, workerId uuid.UUID) error {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	taskToAssign.State = task.Scheduled

	if err := state.TaskDb.Put(taskToAssign.ID.String(), taskToAssign); err != nil {
		return err
	}
	if err := state.TaskWorkerDb.Put(taskToAssign.ID.String(), &workerId); err != nil {
		return err
	}
	if !slices.Contains(state.WorkerTaskMap[workerId], taskToAssign.ID) {
		state.WorkerTaskMap[workerId] = append(state.WorkerTaskMap[workerId], taskToAssign.ID)
	}

	return nil
}
