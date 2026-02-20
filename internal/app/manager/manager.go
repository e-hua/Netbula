package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/e-hua/netbula/internal/store"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

const (
	UpdateTasksPeriod = 15 * time.Second
	SendTasksPeriod = 15 * time.Second
	ManagerDbPath = "manager.db"
	ManagerDbFileMode os.FileMode = 0600
)

type Manager struct {
	// Queue of pending taskevents 
	Pending queue.Queue
	
	TaskDb store.Store[task.Task] // uuid -> task
	EventDb store.Store[task.TaskEvent] // uuid -> taskEvent
	TaskWorkerDb store.Store[uuid.UUID] // TaskUuid -> WorkerUuid
	WorkerNameDb store.Store[string] // uuid -> WorkerName

	// These are the info inferred from the TaskWorkerDb on start of the application
	Workers []uuid.UUID
	WorkerTaskMap map[uuid.UUID][]uuid.UUID

	WorkerClientMap map[uuid.UUID]*http.Client

	Scheduler scheduler.Scheduler

	// Will be updated periodically
	WorkerNodes []*node.Node
}

// Loading the DBs and process them in memory  
// Infer the rest of the fields before
// Creating a new manager struct 
func New(scheduler scheduler.Scheduler, dbType string) *Manager {
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
		if (err != nil) {
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

	taskWorkerEntries, err := taskToWorkerStorage.Entries()
	if (err != nil) {
		log.Printf("Error getting entries in the db mapping tasks to workers")
		taskWorkerEntries = make([]store.Entry[uuid.UUID], 0)
	}

	workerTaskMap := make(map[uuid.UUID][]uuid.UUID)
	workers := make([]uuid.UUID, 0)

	// Infer the content of workerTaskMap from taskWorkerDb
	for _, taskWorkerEntry := range(taskWorkerEntries) {
		taskUuid, err := uuid.Parse(taskWorkerEntry.Key)
		if (err != nil) {
			log.Printf("Error parsing the task UUID from the DB")
			continue
		}

		workerUuid := *taskWorkerEntry.Value	

		assignedTaskSlice, ok := workerTaskMap[workerUuid]
		if (!ok) {
			assignedTaskSlice = make([]uuid.UUID, 0)
		}

		assignedTaskSlice = append(assignedTaskSlice, taskUuid)
		workerTaskMap[workerUuid] = assignedTaskSlice
	}

	// Infer the content of the workers from the workerTaskMap
	workers = slices.Collect(maps.Keys(workerTaskMap))

	m := &Manager{
		Pending: *queue.New(),

		TaskDb: taskStorage,
		EventDb: taskEventStorage,
		TaskWorkerDb: taskToWorkerStorage,	
		WorkerNameDb: workerNameStorage,

		Workers: workers,
		WorkerTaskMap: workerTaskMap,

		WorkerClientMap: make(map[uuid.UUID]*http.Client),

		Scheduler: scheduler,
	}

	return m
}

func (m *Manager) UpdateWorkerNodes() {
	currNodes := make([]*node.Node, 0)

	for _, workerUuid := range(m.Workers) {
		workerHttpClient := m.WorkerClientMap[workerUuid]

		resp, err := workerHttpClient.Get("http://worker/stats")
		if (err != nil) {
			log.Printf("Error sending the request to get node machine stats: %v\n", err)
			continue
		}

		if (resp.StatusCode != http.StatusOK) {
			log.Printf("Error sending requests, status code: %v\n", resp.StatusCode)
			continue
		}

		newDecoder := json.NewDecoder(resp.Body)		
		var stats types.Stats
		err = newDecoder.Decode(&stats)
		if (err != nil) {
			log.Printf("Error unmarshalling node stats: %s\n", err.Error())
			continue
		}

		workerName, _ := m.WorkerNameDb.Get(workerUuid.String())

		currNode := &node.Node{
			Name: *workerName,

			Cores: stats.CpuCount,
			CpuAveragePercent: stats.CpuPercents[0],
			CpuAverageLoad: stats.LoadAvg,

			Memory: int(stats.MemTotalInBytes),
			MemoryAllocatedPercent: stats.MemUsedPercent,				

			Disk: int(stats.DiskTotalInBytes),	
			DiskAllocatedPercent: stats.DiskUsedPercent,

			WorkerUuid: workerUuid,

			TaskCount: len(m.WorkerTaskMap[workerUuid]),
		}

		currNodes = append(currNodes, currNode)

		currNode.PrintNode()	
	}

	m.WorkerNodes = currNodes
}

func (m *Manager) getWorkerClient(workerId uuid.UUID) (*http.Client, error) {
	client, ok := m.WorkerClientMap[workerId]
	if (!ok) {
		return nil, fmt.Errorf("Worker is not registered in the manager")
	}
	return client, nil
}

func (m *Manager) AddWorkerAndClient(w worker.Worker, client *http.Client) {
	_, ok := m.WorkerTaskMap[w.Uuid]
	// Worker already stored in the map 
	if (!ok) {
		m.Workers = append(m.Workers, w.Uuid)
	}

	m.WorkerClientMap[w.Uuid] = client
	m.WorkerNameDb.Put(w.Uuid.String(), &w.Name)
}

/* 
Update the stats of all the workers
Then select the worker using the scheduler in the manager, with these new stats

Throw an error if no candidate node is found 
*/
func (m *Manager) SelectWorker(t task.Task) (uuid.UUID, error) {
	// Update the stats of all the nodes
	m.UpdateWorkerNodes()

	candidates := m.Scheduler.SelectCandidateNodes(t, m.WorkerNodes)

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
	for _, workerUuid := range(m.Workers) {
		workerHttpClient, ok := m.WorkerClientMap[workerUuid]
		if (!ok) {
			log.Printf("HTTP client is no long available for the worker: %v", workerUuid.String())
			continue
		}

		resp, err := workerHttpClient.Get("http://worker/tasks")
		if (err != nil) {
			log.Printf("Error sending the request to sync states: %v\n", err)
			continue
		}

		if (resp.StatusCode != http.StatusOK) {
			log.Printf("Error sending requests, status code: %v\n", resp.StatusCode)
		}

		newDecoder := json.NewDecoder(resp.Body)		
		var tasks []*task.Task
		err = newDecoder.Decode(&tasks)
		if (err != nil) {
			log.Printf("Error unmarshalling tasks: %s\n", err.Error())
		}

		for _, currTask := range(tasks) {
			log.Printf("Attempting to update task %v\n", currTask.ID)

			taskPersisted, error := m.TaskDb.Get(currTask.ID.String())
			if (error != nil) {
				log.Printf("Task with ID %s not found\n", currTask.ID)
				continue
			}

			taskPersisted.State = currTask.State

			taskPersisted.StartTime = currTask.StartTime
			taskPersisted.FinishTime = currTask.FinishTime
			taskPersisted.ContainerID = currTask.ContainerID
			taskPersisted.PortBindings = currTask.PortBindings

			m.TaskDb.Put(taskPersisted.ID.String(), taskPersisted)
		}
	}
}

// For worker
// Sends the `taskEvent` when the "Pending" queue is not empty 
func (m *Manager) SendWork() {
	if (m.Pending.Len() > 0) {
		taskEvent := m.Pending.Dequeue().(task.TaskEvent)
		err := m.EventDb.Put(taskEvent.ID.String(), &taskEvent)
		if (err != nil) {
			log.Printf("Error attempting to store task event %s: %s\n", taskEvent.ID.String(), err)
			return
		}

		// If the task is already running
		// Set the `selectedWorker` to be the worker we retreived 
		selectedWorker, err := m.TaskWorkerDb.Get(taskEvent.Task.ID.String())

		if (err != nil) {
			log.Printf("Error getting worker from the TaskWorkerDb: %v", err)
			return 
		// If the worker is in DB 
		} else if (selectedWorker != nil) {
			// If we're stopping the task
			if (taskEvent.Task.State == task.Completed) {
				m.stopTask(taskEvent.Task.ID)
			} else {
				log.Printf(
					"Cannot send task with target state %v to the worker, task is already being runned", 
					taskEvent.Task.State,
				)
			}
			return;
		// If the worker is not in DB 
		} else {
			workerUuid, err := m.SelectWorker(taskEvent.Task)
			if (err != nil) {
				log.Printf("Cannot assign task to any of the worker node: %v", err)
				return 
			}
			selectedWorker = &workerUuid
		}

		pendingTask := taskEvent.Task	

		// Assign this task to the worker on manager side 
		m.WorkerTaskMap[*selectedWorker] = append(m.WorkerTaskMap[*selectedWorker], pendingTask.ID)
		m.TaskWorkerDb.Put(pendingTask.ID.String(), selectedWorker)

		// Change its status to Scheduled 
		pendingTask.State = task.Scheduled
		m.TaskDb.Put(pendingTask.ID.String(), &pendingTask)

		data, err := json.Marshal(taskEvent)
		if err != nil {
			log.Printf("Unable to marshal task object: %v.\n", taskEvent)
		}


		client := m.WorkerClientMap[*selectedWorker]
		resp, err := client.Post("http://worker/tasks", "application/json", bytes.NewBuffer(data))
		if (err != nil) {
			log.Printf("Error connecting to %v: %v\n", selectedWorker, err)
			// revert the transaction 
			m.Pending.Enqueue(taskEvent)
			return	
		}

		decoder := json.NewDecoder(resp.Body)

		if (resp.StatusCode != http.StatusCreated) {
			e := routers.ErrResponse{}
			err := decoder.Decode(&e)
				if err != nil {
					fmt.Printf("Error decoding response: %s\n", err.Error())
					return
				}
			log.Printf("Response error (%d): %s", resp.StatusCode, e.Message)
			return
		}

		decodedTask := task.Task{}
		err = decoder.Decode(&decodedTask)
		if (err != nil) {
			fmt.Printf("Error decoding response: %s\n", err.Error())
			return
		}

		log.Printf("New task created by worker: %+v\n", decodedTask)
	} else {
		log.Println("No work in the queue")
	}
}

// For user 
func (m *Manager) AddTaskEvent(te task.TaskEvent) {
	m.Pending.Enqueue(te)
}

func (m *Manager) stopTask(taskId uuid.UUID) {
	workerId, err := m.TaskWorkerDb.Get(taskId.String()) 
	if (err != nil) {
		log.Printf("Error getting workerId from db: %s", err)
		return 
	}

	client, err := m.getWorkerClient(*workerId) 
	if (err != nil) {
		return 
	}

	url := fmt.Sprintf("http://worker/tasks/%s", taskId.String())

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		log.Printf("error creating request to delete task %s: %v\n", taskId, err)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("error connecting to worker <%s> at %s: %v\n", workerId ,url, err)
		return
	}

	if resp.StatusCode != http.StatusNoContent {
		log.Printf("Error sending request: %v\n", err)
		return
	}

	log.Printf("task %s has been sent to the worker to be stopped", taskId)
}

// Runs an infinite loop to sync the state of the manager with the workers 
func (m *Manager) UpdateTasksForever() {
	for {
		fmt.Printf("[Manager] Updating tasks from %d workers\n", len(m.Workers))
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