package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
)

const UpdateTasksPeriod = 15 * time.Second
const SendTasksPeriod = 15 * time.Second

type Manager struct {
	// Queue of pending taskevents 
	Pending queue.Queue
	
	TaskMap map[uuid.UUID]*task.Task
	EventMap map[uuid.UUID]*task.TaskEvent

	Workers []uuid.UUID
	
	// Maybe we should introduce relational DB here 
	WorkerTaskMap map[uuid.UUID][]uuid.UUID
	TaskWorkerMap map[uuid.UUID]uuid.UUID

	WorkerClientMap map[uuid.UUID]*http.Client
	WorkerNameMap map[uuid.UUID]string

	WorkerNodes []*node.Node
	Scheduler scheduler.Scheduler
}

func New(workers []uuid.UUID, scheduler scheduler.Scheduler) *Manager {
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

		WorkerTaskMap: workerTaskMap,
		TaskWorkerMap: taskWorkerMap,	

		WorkerClientMap: make(map[uuid.UUID]*http.Client),
		WorkerNameMap: make(map[uuid.UUID]string),

		Scheduler: scheduler,
	}
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

		currNode := &node.Node{
			Name: m.WorkerNameMap[workerUuid],

			Cores: stats.CpuCount,
			CpuAveragePercent: stats.CpuPercents[0],
			CpuAverageLoad: stats.LoadAvg,

			Memory: int(stats.MemTotalInBytes),
			MemoryAllocatedPercent: stats.MemUsedPercent,				

			Disk: int(stats.DiskTotalInBytes),	
			DiskAllocatedPercent: stats.DiskUsedPercent,

			WorkerUuid: workerUuid,
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

func (m *Manager) GetTasks() map[uuid.UUID]*task.Task {
	return m.TaskMap
}

func (m *Manager) AddWorkerAndClient(w worker.Worker, client *http.Client) {
	m.Workers = append(m.Workers, w.Uuid)
	m.WorkerClientMap[w.Uuid] = client
	m.WorkerNameMap[w.Uuid] = w.Name
}

func (m *Manager) SelectWorker(t task.Task) (uuid.UUID, error) {
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
		taskEvent := m.Pending.Dequeue().(task.TaskEvent)
		m.EventMap[taskEvent.ID] = &taskEvent

		// Set the `selectedWorker` to be the worker we retreived 
		// If the task is already running
		selectedWorker, ok := m.TaskWorkerMap[taskEvent.Task.ID] 
		if (ok)  {
			if (taskEvent.Task.State == task.Completed) {
				m.stopTask(taskEvent.Task.ID)
				return;
			} 		
		} else {
			workerUuid, err := m.SelectWorker(taskEvent.Task)
			if (err != nil) {
				return 
			}
			selectedWorker = workerUuid
		}

		pendingTask := taskEvent.Task	

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

func (m *Manager) stopTask(taskId uuid.UUID) {
	workerId := m.TaskWorkerMap[taskId]

	client, err := m.getWorkerClient(workerId) 
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