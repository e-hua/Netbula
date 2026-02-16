package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

type Manager struct {
	// Queue of pending taskevents 
	Pending queue.Queue
	
	TaskMap map[uuid.UUID]*task.Task
	EventMap map[uuid.UUID]*task.TaskEvent

	Workers []uuid.UUID
	lastWorkerIdx int
	
	// Maybe we should introduce relational DB here 
	WorkerTaskMap map[uuid.UUID][]uuid.UUID
	TaskWorkerMap map[uuid.UUID]uuid.UUID

	WorkerClientMap map[uuid.UUID]*http.Client
	WorkerNameMap map[uuid.UUID]string
}

func New(workers []uuid.UUID) *Manager {
	taskMap := make(map[uuid.UUID]*task.Task)
	eventMap := make(map[uuid.UUID]*task.TaskEvent)
	workerTaskMap := make(map[uuid.UUID][]uuid.UUID)
	taskWorkerMap := make(map[uuid.UUID]uuid.UUID)

	for workerIdx := range(workers) {
		workerTaskMap[workers[workerIdx]] = []uuid.UUID{}
	}	

	return &Manager{
		Pending: *queue.New(),

		TaskMap: taskMap,
		EventMap: eventMap,

		Workers: workers,
		lastWorkerIdx: -1,

		WorkerTaskMap: workerTaskMap,
		TaskWorkerMap: taskWorkerMap,	

		WorkerClientMap: make(map[uuid.UUID]*http.Client),
		WorkerNameMap: make(map[uuid.UUID]string),
	}
}

func (m *Manager) AddWorkerAndClient(w worker.Worker, client *http.Client) {
	m.Workers = append(m.Workers, w.Uuid)
	m.WorkerClientMap[w.Uuid] = client
	m.WorkerNameMap[w.Uuid] = w.Name
}

func (m *Manager) SelectWorker() uuid.UUID {
	nextWorkerIdx := (m.lastWorkerIdx + 1) % len(m.Workers)
	m.lastWorkerIdx = nextWorkerIdx
	return m.Workers[nextWorkerIdx]
}

// For worker
// Sync the states of the tasks in the manager with the workers 
func (m *Manager) UpdateTasks() {
	for _, workerUuid := range(m.Workers) {
		workerHttpClient := m.WorkerClientMap[workerUuid]
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
			_, ok := m.TaskMap[currTask.ID]

			if (!ok) {
				log.Printf("Task with ID %s not found\n", currTask.ID)
				continue
			}

			m.TaskMap[currTask.ID].State = currTask.State
			m.TaskMap[currTask.ID].StartTime = currTask.StartTime
			m.TaskMap[currTask.ID].FinishTime = currTask.FinishTime
			m.TaskMap[currTask.ID].ContainerID = currTask.ContainerID
		}
	}
}

// For worker
// Sends the `taskEvent` when the "Pending" queue is not empty 
func (m *Manager) SendWork() {
	if (m.Pending.Len() > 0) {
		selectedWorker := m.SelectWorker()
		taskEvent := m.Pending.Dequeue().(task.TaskEvent)
		pendingTask := taskEvent.Task	

		m.EventMap[taskEvent.ID] = &taskEvent
		m.WorkerTaskMap[selectedWorker] = append(m.WorkerTaskMap[selectedWorker], pendingTask.ID)
		m.TaskWorkerMap[pendingTask.ID] = selectedWorker

		pendingTask.State = task.Scheduled
		m.TaskMap[pendingTask.ID] = &pendingTask

		data, err := json.Marshal(taskEvent)
		if err != nil {
			log.Printf("Unable to marshal task object: %v.\n", taskEvent)
		}

		client := m.WorkerClientMap[selectedWorker]
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