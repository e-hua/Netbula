package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/go-chi/chi/v5"
	"github.com/golang-collections/collections/queue"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

type testTaskData struct {
	taskName string
}

type testWorkerData struct {
	workerName string
	testTasks  []testTaskData
}

var (
	testCases = []struct {
		testName   string
		workerData []testWorkerData
	}{
		{
			testName: "Single worker",
			workerData: []testWorkerData{
				{
					workerName: "MacBook",
					testTasks:  []testTaskData{},
				},
			},
		},
		{
			testName: "Multiple worker",
			workerData: []testWorkerData{
				{
					workerName: "Ubuntu-VM-1",
					testTasks:  []testTaskData{},
				},
				{
					workerName: "Ubuntu-VM-2",
					testTasks:  []testTaskData{},
				},
			},
		},
		{
			testName: "Multiple worker with existing tasks",
			workerData: []testWorkerData{
				{
					workerName: "Ubuntu-VM-1",
					testTasks: []testTaskData{
						{taskName: "http-server"},
						{taskName: "postgreSQL"},
					},
				},
				{
					workerName: "Ubuntu-VM-2",
					testTasks:  []testTaskData{},
				},
			},
		},
	}

	mockStats = types.Stats{
		MemTotalInBytes:  0x800000000,
		MemUsedPercent:   69.19236183166504,
		DiskTotalInBytes: 0x731ba15000,
		DiskUsedPercent:  69.08153314714131,
		CpuCount:         10,
		CpuPercents:      []float64{10.380591165889095},
		LoadAvg:          2.02001953125,
	}
)

func createTestWorker(workerData testWorkerData) (*worker.Worker, []task.Task) {
	testWorker := worker.NewWorker(uuid.New(), workerData.workerName, *queue.New(), "memory")
	testTasks := make([]task.Task, 0)

	for _, taskData := range workerData.testTasks {
		newTestTask := task.Task{
			ID: uuid.New(),

			Name:  taskData.taskName,
			State: task.Pending,
		}
		testTasks = append(testTasks, newTestTask)

		testWorker.AddTask(newTestTask)
	}

	return testWorker, testTasks
}

// A helper function that creates a Cluster component and
// returns a buffer containing the logs
func createTestCluster() (*HttpClientCluster, *bytes.Buffer) {
	var testLogsBuffer bytes.Buffer
	testLogger := logger.NewManagerLogger(true, &testLogsBuffer)

	testCluster := NewCluster(*testLogger)

	return testCluster, &testLogsBuffer
}

func initTestRouterFromHandlers(handlers worker.Handlers) *chi.Mux {
	mockGetWorkerStatsHandler := func(responseWriter http.ResponseWriter, request *http.Request) {
		routers.RespondJSON(responseWriter, http.StatusOK, mockStats)
	}

	newRouter := chi.NewRouter()

	newRouter.Route("/tasks", func(router chi.Router) {
		router.Post("/", handlers.StartTaskHandler)
		router.Get("/", handlers.GetTasksHandler)
		router.Route("/{taskId}", func(router chi.Router) {
			router.Delete("/", handlers.StopTaskHandler)
		})
	})

	newRouter.Route("/info", func(router chi.Router) {
		router.Get("/", handlers.GetWorkerInfoHandler)
	})

	// Mock the `/stats` route
	newRouter.Route("/stats", func(router chi.Router) {
		router.Get("/", mockGetWorkerStatsHandler)
	})

	return newRouter
}

func TestNewCluster(t *testing.T) {
	testCluster, _ := createTestCluster()

	if clientMapSize := len(testCluster.workerClientMap); clientMapSize != 0 {
		t.Errorf("Expected size of the clientMap to be 0, got %d", clientMapSize)
	}

	if numOfWorkerNodes := len(testCluster.workerNodes); numOfWorkerNodes != 0 {
		t.Errorf("Expected number of nodes in the cluster to be 0, got %d", numOfWorkerNodes)
	}
}

type mockClientRoundTripper struct {
	mockServerUrl    string
	defaultTransport http.RoundTripper
}

func (r *mockClientRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	targetUrl, err := url.Parse(r.mockServerUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the url of mock server: %w", err)
	}

	// Link to the standard lib docs: https://pkg.go.dev/net/url#URL
	// General from of an URL: [scheme:][//[userinfo@]host][/]path[?query][#fragment]

	// Modify the URL
	request.URL.Scheme = targetUrl.Scheme
	request.URL.Host = targetUrl.Host

	return r.defaultTransport.RoundTrip(request)
}

// Taking in a mock worker.Worker struct
// Creating a mock server serving the information related to the worker
// Returning a client listening to this mock server
func createMockWorkerClient(t *testing.T, mockWorker *worker.Worker) *http.Client {
	handlers := worker.Handlers{
		Worker: mockWorker,
	}

	// Creating a chi multiplexer(mux) / router / handler with this handler
	router := initTestRouterFromHandlers(handlers)

	mockServer := httptest.NewServer(router)

	t.Cleanup(func() {
		mockServer.Close()
	})

	mockClient := mockServer.Client()

	// Here we modify the Transport attribute of the client
	// To redirect the request from `http://worker/<path>` to `<mockServer.URL>(http://ipaddr:port)/<path>`
	mockClient.Transport = &mockClientRoundTripper{
		mockServerUrl: mockServer.URL,
		// Using the default Transport here
		defaultTransport: mockClient.Transport,
	}

	return mockClient
}

// Testing `AddClient`, `GetClient` and `createWorkerClientMapCopy`
func TestCluster_ClientManagement(t *testing.T) {
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			testCluster, _ := createTestCluster()

			for _, testWorkerDataEntry := range testCase.workerData {
				newWorker, _ := createTestWorker(testWorkerDataEntry)
				newWorkerClient := createMockWorkerClient(t, newWorker)

				testCluster.AddClient(newWorker.Uuid, newWorkerClient)

				retrievedClient, err := testCluster.GetClient(newWorker.Uuid)
				if err != nil {
					t.Fatalf("Error getting client from Cluster component: %v", err)
				}

				if newWorkerClient != retrievedClient {
					t.Error("Client stored and client retrieved does not have the same reference")
				}
			}

			// Modifying the copy of map of clients should not affect the original map in Cluster
			testWorkerClientMapCopy := testCluster.createWorkerClientMapCopy()

			workerToInsertIntoMapCopy := worker.Worker{Uuid: uuid.New()}
			workerClientToInsertIntoMapCopy := createMockWorkerClient(t, &workerToInsertIntoMapCopy)
			testWorkerClientMapCopy[workerToInsertIntoMapCopy.Uuid] = workerClientToInsertIntoMapCopy

			_, err := testCluster.GetClient(workerToInsertIntoMapCopy.Uuid)
			expectedErrMessage := fmt.Sprintf(
				"client of worker [%s] is not in the WorkerClientMap",
				workerToInsertIntoMapCopy.Uuid.String(),
			)

			if err == nil || err.Error() != expectedErrMessage {
				t.Errorf("Expected error message: %s, got %v instead", expectedErrMessage, err)
			}
		})
	}
}

type LogData struct {
	Message string `json:"msg"`
	Level   string `json:"level"`
}

// Helper function to decode one log JSON object from the buffer used for testing
// And check if all its attributes are the same as we expected
func DecodeAndInspectLog(t *testing.T, testLogsDecoder *json.Decoder, expectedLevel string, expectedMsg string) {
	t.Helper()

	var testLogData LogData

	err := testLogsDecoder.Decode(&testLogData)
	if err != nil {
		t.Fatalf("Failed to decode the one log object from the buffer: %v", err)
	}

	if testLogData.Level != expectedLevel {
		t.Errorf("Expected the WorkerNodesUpdated log to have the level `%s`, got `%s` instead", expectedLevel, testLogData.Level)
	}

	if testLogData.Message != expectedMsg {
		t.Errorf("Expected the WorkerNodesUpdated log to have the message `%s`, got `%s` instead", expectedMsg, testLogData.Message)
	}
}

// Testing `GetNodes`, `UpdateWorkerNodes` along with other helper methods along the way
func TestCluster_NodesStateManagement(t *testing.T) {
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			testCluster, testClusterLogsBuffer := createTestCluster()

			mockStateStore := CreateInMemoryStores()

			// Add all the clients in this test case to the cluster
			for _, testWorkerDataEntry := range testCase.workerData {
				newWorker, _ := createTestWorker(testWorkerDataEntry)
				mockStateStore.workerNameDb.Put(newWorker.Uuid.String(), &newWorker.Name)

				newWorkerClient := createMockWorkerClient(t, newWorker)

				testCluster.AddClient(newWorker.Uuid, newWorkerClient)
			}

			initialNodes := testCluster.GetNodes()
			if len(initialNodes) != 0 {
				t.Errorf("Expected initially no nodes in the manager, instead got %#v", initialNodes)
			}

			var testStateLogsBuffer bytes.Buffer
			testStateLogger := logger.NewManagerLogger(true, &testStateLogsBuffer)
			mockState, err := NewState(*mockStateStore, *testStateLogger)
			if err != nil {
				t.Errorf("Failed to initialize the mock State component: %v", err)
			}

			testCluster.UpdateWorkerNodes(mockState)

			newTestLogDecoder := json.NewDecoder(testClusterLogsBuffer)

			// This is for the first `cluster.clusterLogger.WorkerNodesUpdated(currNodes)`
			DecodeAndInspectLog(t, newTestLogDecoder, "DEBUG", "Worker nodes stats updated")

			for range len(testCase.workerData) {
				DecodeAndInspectLog(t, newTestLogDecoder, "INFO", "Node Appeared")
			}

			updatedNodes := testCluster.GetNodes()
			// fmt.Printf("Updated nodes: %#v", updatedNodes)
			if len(updatedNodes) != len(testCase.workerData) {
				t.Errorf("Expected after update %d nodes in the manager, instead got %#v", len(testCluster.workerNodes), updatedNodes)
			}

			// Check the slice of nodes returned
			for _, currNode := range updatedNodes {
				workerName, workerTaskCount := mockState.GetWorkerMetadata(currNode.WorkerUuid)

				rebuiltNode := node.StatsToNode(currNode.WorkerUuid, workerName, workerTaskCount, mockStats)
				if diff := cmp.Diff(currNode, rebuiltNode); diff != "" {
					t.Errorf("Expected the rebuilt node to be exactly the same as retrieved node, got a difference between two nodes `%s`", diff)
				}
			}

			// No node to update
			testCluster.UpdateWorkerNodes(mockState)
			DecodeAndInspectLog(t, newTestLogDecoder, "DEBUG", "Worker nodes stats updated")
			if testClusterLogsBuffer.String() != "" {
				t.Errorf("Expected no logs left in the `testClusterLogsBuffer`, got %s", testClusterLogsBuffer.String())
			}

			if testStateLogsBuffer.String() != "" {
				t.Errorf("Expected no logs left in the `testStateLogsBuffer`, got %s", testStateLogsBuffer.String())
			}
		})
	}
}

func containTask(t *testing.T, targetTask task.Task, tasks []*task.Task) {
	t.Helper()

	for _, currTask := range tasks {
		if targetTask.ID.String() == currTask.ID.String() {
			if diff := cmp.Diff(targetTask, *currTask); diff != "" {
				t.Errorf(
					"Target task `%#v` is different from task found in the list `%#v`, difference: `%s`",
					targetTask,
					*currTask,
					diff,
				)
			}
			return
		}
	}

	var newByteBuffer bytes.Buffer
	json.NewEncoder(&newByteBuffer).Encode(tasks)

	t.Errorf("Failed to locate task %v in the slice of tasks: %s", targetTask, newByteBuffer.String())
}

func AssertBufferIsEmpty(t *testing.T, buffer *bytes.Buffer, bufferName string) {
	if buffer.String() != "" {
		t.Errorf("Expect the buffer `%s` to be empty, got %s instead", bufferName, buffer.String())
	}
}

// Testing `SyncTasks`, `SendTask`, `StopTask` and the helper functions they used
func TestCluster_TaskManagement(t *testing.T) {
	for _, testCase := range testCases {
		t.Run(testCase.testName, func(t *testing.T) {
			testCluster, testClusterLogsBuffer := createTestCluster()
			testClusterLogsDecoder := json.NewDecoder(testClusterLogsBuffer)

			mockStateStore := CreateInMemoryStores()

			workerMap := make(map[uuid.UUID]*worker.Worker)

			// Iterate through the workers
			for _, testWorkerDataEntry := range testCase.workerData {
				newWorker, initialTasks := createTestWorker(testWorkerDataEntry)
				workerMap[newWorker.Uuid] = newWorker

				// Note: Need to add the tasks assigned to the worker to the manager as well
				for _, initialTask := range initialTasks {
					// Add tasks on manager side
					mockStateStore.taskDb.Put(initialTask.ID.String(), &initialTask)
					mockStateStore.taskWorkerDb.Put(initialTask.ID.String(), &newWorker.Uuid)
				}

				mockStateStore.workerNameDb.Put(newWorker.Uuid.String(), &newWorker.Name)

				newWorkerClient := createMockWorkerClient(t, newWorker)

				// Add all the clients in this test case to the cluster
				testCluster.AddClient(newWorker.Uuid, newWorkerClient)
			}

			var testStateLogsBuffer bytes.Buffer
			testStateLogger := logger.NewManagerLogger(true, &testStateLogsBuffer)
			testStateLogsDecoder := json.NewDecoder(&testStateLogsBuffer)

			mockState, err := NewState(*mockStateStore, *testStateLogger)
			if err != nil {
				t.Fatalf("Failed to create mockState out of store: %v", err)
			}

			// First, before updating the worker, tasks in workers and manager are in sync

			// Now sync the manager with workers for the first time

			// Can see that no `TaskStatusChanged` was called,
			// hence the buffer containing the logs of the state component is gonna be empty
			testCluster.SyncTasks(mockState)
			AssertBufferIsEmpty(t, &testStateLogsBuffer, "testStateLogsBuffer")

			// Made requests to all the n workers
			// Invoked the `cluster.clusterLogger.TasksFetched(workerUuid)` for n times
			managerStoredWorkerIds := mockState.GetWorkerIds()
			for range len(managerStoredWorkerIds) {
				DecodeAndInspectLog(t, testClusterLogsDecoder, "DEBUG", "Tasks from one worker is fetched")
			}

			AssertBufferIsEmpty(t, testClusterLogsBuffer, "testClusterLogsBuffer")

			for _, workerValue := range workerMap {
				// Secondly, we send one new task to each of the workers
				newTaskToSend := task.Task{
					ID:    uuid.New(),
					Name:  "redis",
					State: task.Scheduled,
				}

				newTaskEventToSend := task.TaskEvent{
					ID:          uuid.New(),
					TargetState: task.Running,
					Timestamp:   time.Now(),
					Task:        newTaskToSend,
				}

				taskSentBackFromWorker, err := testCluster.SendTask(workerValue.Uuid, newTaskEventToSend)
				if err != nil {
					t.Errorf("Failed to send the task %#v to worker %s: %v", newTaskToSend, workerValue.Uuid.String(), err)
				}

				if taskSentBackFromWorker.ID.String() == newTaskToSend.ID.String() {
					if diff := cmp.Diff(*taskSentBackFromWorker, newTaskToSend); diff != "" {
						t.Errorf("Expected the task from manager component and worker component to be the same, got difference string: %s", diff)
					}
				} else {
					t.Error("Failed to find the task sent from manager in the list of tasks retrieved from the worker")
				}

				// Now let's update the state of tasks in the worker
				// before we sync the manager with the workers again
				for workerValue.Queue.Len() != 0 {
					elementFromQueue := workerValue.Queue.Dequeue()

					taskToRun := elementFromQueue.(task.Task)
					taskToRun.State = task.Running

					err := workerValue.StoreTask(taskToRun)
					if err != nil {
						t.Errorf("Failed to store the task into the worker: %v", err)
					}
				}

				// Need to put this task into the taskDb before using the `SyncTasks` method
				// For it to be able to be updated correctly
				mockStateStore.taskDb.Put(newTaskToSend.ID.String(), &newTaskToSend)

				testCluster.SyncTasks(mockState)
				for range len(testCase.workerData) {
					DecodeAndInspectLog(t, testClusterLogsDecoder, "DEBUG", "Tasks from one worker is fetched")
				}

				AssertBufferIsEmpty(t, testClusterLogsBuffer, "testClusterLogsBuffer")

				if err != nil {
					t.Errorf("Failed to get the tasks stored in manager: %v", err)
				}

				// Check if the new task we created for this worker is also synced
				newTaskCopy := newTaskToSend
				newTaskCopy.State = task.Running

				syncedManagerTasks, err := mockState.GetTasks()
				containTask(t, newTaskCopy, syncedManagerTasks)
			}

			// Iterate over all the tasks in the manager (from different users)
			// Inspect the logs
			syncedManagerTasks, err := mockState.GetTasks()
			for range syncedManagerTasks {
				DecodeAndInspectLog(t, testStateLogsDecoder, "INFO", "Task status change detected")
			}

			AssertBufferIsEmpty(t, testClusterLogsBuffer, "testClusterLogsBuffer")
			AssertBufferIsEmpty(t, &testStateLogsBuffer, "testStateLogsBuffer")
		})
	}
}
