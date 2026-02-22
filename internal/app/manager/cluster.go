package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/google/uuid"
)

// A cluster of connections to worker nodes
type Cluster struct {
	// Need to include mutex to prevent multiple writes at the same time
	mutex sync.RWMutex

	WorkerClientMap map[uuid.UUID]*http.Client
	// Will be updated periodically
	WorkerNodes []*node.Node
}

// Create an empty map and empty slice nodes
func NewCluster() *Cluster {
	return &Cluster{
		WorkerClientMap: make(map[uuid.UUID]*http.Client, 0),
		WorkerNodes: make([]*node.Node, 0),
	}
}

// Iterate through all the worker UUIDs in the current state of the manager 
// Fire http requests to the clients in parallel 
// Getting node stats 
// And Update the stats of the nodes 
func (cluster *Cluster) UpdateWorkerNodes(state *State) {
	// Get a copy of the slices of the current worker UUIDs
	workers := state.GetWorkerIds()
	// Create the node slice to replace current worker nodes 
	currNodes := make([]*node.Node, 0)

	// Copy the HTTP clients at this point to another slice 
	// Lock the mutex while reading 
	clientsCopy := cluster.createWorkerClientMapCopy()
	
	var wg sync.WaitGroup
	nodeChan := make(chan *node.Node, len(workers))

	for _, workerUuid := range(workers) {
		currClient, ok := clientsCopy[workerUuid]
		if (!ok) {
			continue
		}

		// This amazing feature came out in Go 1.25
		wg.Go(func () {
			stats, err := fetchNodeStats(currClient)
			if (err != nil) {
				log.Printf("Error fetching node stats for worker id %s: %v", workerUuid.String() ,err)
				return;
			}

			workerName, workerTaskCount := state.GetWorkerMetadata(workerUuid)

			currNode := &node.Node {
				Name: workerName,

				Cores: stats.CpuCount,
				CpuAveragePercent: stats.CpuPercents[0],
				CpuAverageLoad: stats.LoadAvg,

				Memory: int(stats.MemTotalInBytes),
				MemoryAllocatedPercent: stats.MemUsedPercent,				

				Disk: int(stats.DiskTotalInBytes),	
				DiskAllocatedPercent: stats.DiskUsedPercent,

				WorkerUuid: workerUuid,

				TaskCount: workerTaskCount,
			}

			nodeChan <- currNode
		}) 
	}

	go func() {
		wg.Wait()
		// Close the channel once all the routines are finished
		close(nodeChan)
	}()

	// Iterates over all elements in the buffer, until the channel is closed
	for nodeInChan := range(nodeChan) {
		currNodes = append(currNodes, nodeInChan)
		nodeInChan.PrintNode()
	}

	// Replace the slice of node in the struct entirely 
	cluster.mutex.Lock()
	cluster.WorkerNodes = currNodes
	cluster.mutex.Unlock()
}

// Private helper method 
func fetchNodeStats(workerHttpClient *http.Client) (*types.Stats, error) { 
	resp, err := workerHttpClient.Get("http://worker/stats")
	if (err != nil) {
		log.Printf("Error sending the request to get node machine stats: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	if (resp.StatusCode != http.StatusOK) {
		log.Printf("Error sending requests, status code: %v\n", resp.StatusCode)
		return nil, fmt.Errorf("Bad status code: %d", resp.StatusCode)
	}

	var stats types.Stats
	return &stats, json.NewDecoder(resp.Body).Decode(&stats)
}

// Iterate through all the worker UUIDs in the current state of the manager 
// Fire http requests to the clients in parallel 
// Getting task statuses straight from the worker in charge of them 
// And Update the statuses of the tasks
func (cluster *Cluster) SyncTasks(state *State) {
	// Get a copy of the slices of the current worker UUIDs
	workers := state.GetWorkerIds()

	// Copy the HTTP clients at this point to another slice 
	// Lock the mutex while reading 
	clientsCopy := cluster.createWorkerClientMapCopy()
	
	var wg sync.WaitGroup
	tasksChan := make(chan []*task.Task, len(workers))

	for _, workerUuid := range(workers) {
		currClient, ok := clientsCopy[workerUuid]
		if (!ok) {
			continue
		}

		wg.Go(func () {
			tasks, err := fetchTasks(currClient)
			if (err != nil) {
				log.Printf("Error fetching tasks for worker id %s: %v", workerUuid.String() ,err)
				return;
			}

			tasksChan <- tasks
		}) 
	}

	go func() {
		wg.Wait()
		// Close the channel once all the routines are finished
		close(tasksChan)
	}()

	// Iterate over the slices of tasks in the channel 
	// And sync manager tasks with them 
	for tasksPerWorkerInChan := range(tasksChan) {
		for _, taskToUpdate := range(tasksPerWorkerInChan) {
			err := state.UpdateTask(taskToUpdate)
			if (err != nil) {
				log.Printf("Error updating task %s storage in manager: %s", taskToUpdate.ID, err.Error())
			}
		}
	}

}

// Private helper method 
func fetchTasks(workerHttpClient *http.Client) ([]*task.Task, error) {
	resp, err := workerHttpClient.Get("http://worker/tasks")
	if (err != nil) {
		log.Printf("Error sending the request to sync states: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	if (resp.StatusCode != http.StatusOK) {
		log.Printf("Error sending requests, status code: %v\n", resp.StatusCode)
		return nil, fmt.Errorf("Bad status code: %d", resp.StatusCode)
	}

	var tasks []*task.Task
	return tasks, json.NewDecoder(resp.Body).Decode(&tasks)
}

// Sends the task to the target worker
// Returns the created new task we sent 
func (cluster *Cluster) SendTask(targetWorkerId uuid.UUID, taskEvent task.TaskEvent) (*task.Task, error) {
	cluster.mutex.RLock()	
	client, ok := cluster.WorkerClientMap[targetWorkerId]
	cluster.mutex.RUnlock()	

	if (!ok) {
		return nil, fmt.Errorf("Failed to get HTTP client for taget worker: %s", targetWorkerId.String())
	}

	decodedTask, err := postTask(client, taskEvent, targetWorkerId); 
	if (err == nil) {
		log.Printf("New task created by worker: %+v\n", decodedTask)
	}

	return decodedTask, err
}

// Send HTTP POST request to worker 
func postTask(workerHttpClient *http.Client, taskEvent task.TaskEvent, targetWorkerId uuid.UUID) (*task.Task, error) {
	data, err := json.Marshal(taskEvent)
	if err != nil {
		log.Printf("Unable to marshal task object: %v.\n", taskEvent)
	}

	resp, err := workerHttpClient.Post("http://worker/tasks", "application/json", bytes.NewBuffer(data))
	if (err != nil) {
		log.Printf("Error connecting to %s: %v\n", targetWorkerId.String(), err)
		return nil, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)

	if (resp.StatusCode != http.StatusCreated) {
		e := routers.ErrResponse{}
		err := decoder.Decode(&e)
		if err != nil {
			return nil, fmt.Errorf("Error decoding response: %s\n", err.Error())
		}

		return nil, fmt.Errorf("Response error (%d): %s", resp.StatusCode, e.Message)
	}

	decodedTask := task.Task{}
	err = decoder.Decode(&decodedTask)
	if (err != nil) {
		return nil, fmt.Errorf("Error decoding response: %s\n", err.Error())
	}

	return &decodedTask, nil
}

func (cluster *Cluster) StopTask(state *State, taskIdToStop uuid.UUID) error {
	workerId, err := state.TaskWorkerDb.Get(taskIdToStop.String()) 
	if (err != nil) {
		return fmt.Errorf("Error getting workerId from db: %v", err)
	}
	if (workerId == nil) {
		return fmt.Errorf(
			"Error getting workerId related to taskId %s: %v", 
			taskIdToStop.String(),
			err,
		)
	}

	cluster.mutex.RLock()	
	client, ok := cluster.WorkerClientMap[*workerId]
	cluster.mutex.RUnlock()	

	if (!ok) {
		return fmt.Errorf("Failed to get HTTP client for taget worker: %s", workerId.String())
	}

	return deleteTask(client, taskIdToStop, *workerId)
}

func deleteTask(workerHttpClient *http.Client, taskId uuid.UUID, workerId uuid.UUID) error {
	url := fmt.Sprintf("http://worker/tasks/%s", taskId.String())

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("Error creating request to delete task %s: %v\n", taskId, err)
	}

	resp, err := workerHttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Error connecting to worker <%s> at %s: %v\n", workerId ,url, err)
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Response error: status code (%d)\n", resp.StatusCode)
	}

	log.Printf("Task %s has been sent to the worker to be stopped\n", taskId)
	return nil
}

// Retrieve the HTTP client(of the worker with the UUID in the params) from the cluster 
func (cluster *Cluster) GetClient(workerId uuid.UUID) (*http.Client, error) {
	cluster.mutex.RLock()
	defer cluster.mutex.RUnlock()

	client, ok := cluster.WorkerClientMap[workerId]
	if (!ok) {
		return nil, fmt.Errorf("Client of worker %s is not in the WorkerClientMap", workerId.String()) 
	}

	return client, nil 
}

func (cluster *Cluster) AddClient(workerId uuid.UUID, client *http.Client) {
	cluster.mutex.Lock()
	defer cluster.mutex.Unlock()

	cluster.WorkerClientMap[workerId] = client
}

// Copy the HTTP clients at this point to another slice 
// Lock the mutex while reading 
func (cluster *Cluster) createWorkerClientMapCopy() map[uuid.UUID]*http.Client {
	cluster.mutex.RLock()
	defer cluster.mutex.RUnlock()

	clientsCopy := make(map[uuid.UUID]*http.Client)
	for k, v := range(cluster.WorkerClientMap) {
		clientsCopy[k] = v
	}

	return clientsCopy
}

func (cluster *Cluster) GetNodes() []node.Node {
	cluster.mutex.RLock()
	defer cluster.mutex.RUnlock()

	copiedNodes := make([]node.Node, 0, len(cluster.WorkerNodes))

	for _, node := range(cluster.WorkerNodes) {
		copiedNode := *node
		copiedNodes = append(copiedNodes, copiedNode)
	}

	return copiedNodes
}