package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"sync"

	"github.com/e-hua/netbula/internal/logger"
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

	clusterLogger logger.ManagerLogger
}

// Create an empty map and empty slice nodes
func NewCluster(clusterLogger logger.ManagerLogger) *Cluster {
	return &Cluster{
		WorkerClientMap: make(map[uuid.UUID]*http.Client, 0),
		WorkerNodes:     make([]*node.Node, 0),
		clusterLogger: clusterLogger,
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

	for _, workerUuid := range workers {
		currClient, ok := clientsCopy[workerUuid]
		if !ok {
			continue
		}

		// This amazing feature came out in Go 1.25
		// No need to do Wg.Done() and wg.Add() manually 
		wg.Go(func() {
			stats, err := fetchNodeStats(currClient)
			if err != nil {
				wrappedErr := fmt.Errorf("failed to fetch node stats from worker with id [%s]: %w", workerUuid.String(), err)
				cluster.clusterLogger.Error("Failed when updating worker nodes", "error", wrappedErr)
				return
			}

			workerName, workerTaskCount := state.GetWorkerMetadata(workerUuid)

			currNode := &node.Node{
				Name: workerName,

				Cores:             stats.CpuCount,
				CpuAveragePercent: stats.CpuPercents[0],
				CpuAverageLoad:    stats.LoadAvg,

				Memory:                 int(stats.MemTotalInBytes),
				MemoryAllocatedPercent: stats.MemUsedPercent,

				Disk:                 int(stats.DiskTotalInBytes),
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
	for nodeInChan := range nodeChan {
		currNodes = append(currNodes, nodeInChan)
	}

	cluster.clusterLogger.WorkerNodesUpdated(currNodes)
	cluster.detectNodeChanges(currNodes)
	// Replace the slice of node in the struct entirely
	cluster.mutex.Lock()
	cluster.WorkerNodes = currNodes
	cluster.mutex.Unlock()
}

func (cluster *Cluster) detectNodeChanges(newNodes []*node.Node) {
	oldNodes := cluster.GetNodes()
	oldNodesMap := make(map[uuid.UUID]node.Node)
	newNodesMap := make(map[uuid.UUID]node.Node)
	
	for _, oldNode := range oldNodes {
		oldNodesMap[oldNode.WorkerUuid] = oldNode
	}

	for _, newNode := range newNodes {
		newNodesMap[newNode.WorkerUuid] = *newNode
	}

	for oldNodeId, oldNode := range oldNodesMap {
		_, ok := newNodesMap[oldNodeId]
		if !ok {
			cluster.clusterLogger.NodeDisappeared(oldNode)	
		}
	}

	for newNodeId, newNode := range newNodesMap {
		_, ok := oldNodesMap[newNodeId]
		if !ok {
			cluster.clusterLogger.NodeAppeared(newNode)	
		}
	}
}

// Private helper method
func fetchNodeStats(workerHttpClient *http.Client) (*types.Stats, error) {
	resp, err := workerHttpClient.Get("http://worker/stats")
	if err != nil {
		return nil, fmt.Errorf("failed to send the request to get node machine stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
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

	for _, workerUuid := range workers {
		currClient, ok := clientsCopy[workerUuid]
		if !ok {
			continue
		}

		wg.Go(func() {
			tasks, err := fetchTasks(currClient)
			if err != nil {
				cluster.clusterLogger.Error("Failed to send task to worker", "error", err, "worker", workerUuid.String())
				return
			}

			// Log to show the fetching is done 
			cluster.clusterLogger.TasksFetched(workerUuid)

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
	for tasksPerWorkerInChan := range tasksChan {
		for _, taskToUpdate := range tasksPerWorkerInChan {
			err := state.UpdateTask(taskToUpdate)
			if err != nil {
				cluster.clusterLogger.Error(
					"Failed when updating task in the storage of the manager", 
					"error", err, 
					"task_to_update", taskToUpdate.ID.String(),
				)
			}
		}
	}
}

// Private helper method
func fetchTasks(workerHttpClient *http.Client) ([]*task.Task, error) {
	resp, err := workerHttpClient.Get("http://worker/tasks")
	if err != nil {
		return nil, fmt.Errorf("failed to send the request to worker to fetch its tasks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	var tasks []*task.Task
	return tasks, json.NewDecoder(resp.Body).Decode(&tasks)
}

// Sends the task to the target worker
//
// Returns the created new task we sent
func (cluster *Cluster) SendTask(targetWorkerId uuid.UUID, taskEvent task.TaskEvent) (*task.Task, error) {
	cluster.mutex.RLock()
	client, ok := cluster.WorkerClientMap[targetWorkerId]
	cluster.mutex.RUnlock()

	if !ok {
		return nil, fmt.Errorf("failed to get HTTP client for taget worker with UUID [%s]", targetWorkerId.String())
	}

	decodedTask, err := postTask(client, taskEvent, targetWorkerId)
	if err != nil {
		return nil, fmt.Errorf("failed to send the taskEvent to target worker with UUID [%s]: %w", targetWorkerId.String(), err)
	}

	return decodedTask, nil
}

// Send HTTP POST request to worker
func postTask(workerHttpClient *http.Client, taskEvent task.TaskEvent, targetWorkerId uuid.UUID) (*task.Task, error) {
	data, err := json.Marshal(taskEvent)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal the task event %v: %w", taskEvent, err)
	}

	resp, err := workerHttpClient.Post("http://worker/tasks", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to send task event %v to worker with UUID [%s]: %w", taskEvent, targetWorkerId.String(), err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		e := routers.ErrResponse{}
		err := decoder.Decode(&e)
		if err != nil {
			return nil, fmt.Errorf("failed to decoding error response body: %w", err)
		}

		return nil, fmt.Errorf("bad status code (%d): %s", resp.StatusCode, e.Message)
	}

	decodedTask := task.Task{}
	err = decoder.Decode(&decodedTask)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return &decodedTask, nil
}

// TODO: Merge this method with SendTask 
func (cluster *Cluster) StopTask(state *State, taskIdToStop uuid.UUID) error {
	workerId, err := state.TaskWorkerDb.Get(taskIdToStop.String())
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

	cluster.mutex.RLock()
	client, ok := cluster.WorkerClientMap[*workerId]
	cluster.mutex.RUnlock()

	if !ok {
		return fmt.Errorf("failed to get HTTP client for taget worker with UUID [%s]", workerId.String())
	}

	err = deleteTask(client, taskIdToStop, *workerId)
	if err != nil {
		return fmt.Errorf("failed to delete the task with UUID [%s]: %w", taskIdToStop.String(), err)
	}

	return nil
}

func deleteTask(workerHttpClient *http.Client, taskId uuid.UUID, workerId uuid.UUID) error {
	url := fmt.Sprintf("http://worker/tasks/%s", taskId.String())

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create the request to delete task with UUID [%s]: %w", taskId.String(), err)
	}

	resp, err := workerHttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to worker [%s] at %s: %w", workerId.String(), url, err)
	}

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("bad status code (%d)", resp.StatusCode)
	}

	return nil
}

// Retrieve the HTTP client(of the worker with the UUID in the params) from the cluster
func (cluster *Cluster) GetClient(workerId uuid.UUID) (*http.Client, error) {
	cluster.mutex.RLock()
	defer cluster.mutex.RUnlock()

	client, ok := cluster.WorkerClientMap[workerId]
	if !ok {
		return nil, fmt.Errorf("client of worker [%s] is not in the WorkerClientMap", workerId.String())
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
	maps.Copy(clientsCopy, cluster.WorkerClientMap)

	return clientsCopy
}

func (cluster *Cluster) GetNodes() []node.Node {
	cluster.mutex.RLock()
	defer cluster.mutex.RUnlock()

	copiedNodes := make([]node.Node, 0, len(cluster.WorkerNodes))

	for _, node := range cluster.WorkerNodes {
		copiedNode := *node
		copiedNodes = append(copiedNodes, copiedNode)
	}

	return copiedNodes
}
