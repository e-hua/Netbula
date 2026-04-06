package manager

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

const testConcurrentAccessCount int = 10_000

// Use this helper functions
// To insert some mock values into the testStore we passed in
func insertMockData(testStore *stateStores) {
	testWorker1Id := uuid.New()
	testWorker2Id := uuid.New()
	testWorker3Id := uuid.New()

	testWorker1Name := "worker-1"
	testWorker2Name := "worker-2"
	testWorker3Name := "worker-3"

	// Assigned to worker-1
	testTask1Uuid := uuid.New()
	testTask2Uuid := uuid.New()

	// Assigned to worker-2
	testTask3Uuid := uuid.New()

	testStore.taskWorkerDb.Put(testTask1Uuid.String(), &testWorker1Id)
	testStore.taskWorkerDb.Put(testTask2Uuid.String(), &testWorker1Id)
	testStore.taskWorkerDb.Put(testTask3Uuid.String(), &testWorker2Id)

	testStore.workerNameDb.Put(testWorker1Id.String(), &testWorker1Name)
	testStore.workerNameDb.Put(testWorker2Id.String(), &testWorker2Name)
	testStore.workerNameDb.Put(testWorker3Id.String(), &testWorker3Name)
}

func createTestState(t *testing.T, testStore *stateStores) (*State, *bytes.Buffer) {
	t.Helper()

	var logBuf bytes.Buffer
	testLogger := logger.NewManagerLogger(true, &logBuf)

	newState, err := NewState(*testStore, *testLogger)
	if err != nil {
		t.Fatalf("Failed to create the test state component: %v", err)
	}

	return newState, &logBuf
}

func TestWorkerManagement(t *testing.T) {
	testInMemoryStore := createInMemoryStores()
	testState, testLogBuffer := createTestState(t, testInMemoryStore)

	workerUuid := uuid.New()
	workerName := "worker-1"

	// Test: Registering and re-registering the workers
	testState.RegisterWorker(workerUuid, workerName)

	// Test: Asserting the log messages
	expectedLogMessage := "Worker Connected"
	expectedLogWorkerUuid := workerUuid

	type logData struct {
		Message  string    `json:"msg"`
		WorkerId uuid.UUID `json:"worker_id"`
	}
	testLogData := logData{}

	json.NewDecoder(testLogBuffer).Decode(&testLogData)
	if testLogData.Message != expectedLogMessage {
		t.Errorf("Expected log message to be [%s], got [%s]", expectedLogMessage, testLogData.Message)
	}
	if testLogData.WorkerId.String() != expectedLogWorkerUuid.String() {
		t.Errorf("Expected worker uuid to be [%s], got [%s]", expectedLogWorkerUuid.String(), testLogData.WorkerId.String())
	}

	workerNewName := "worker-2"
	expectedNewLogMessage := "Worker Reconnected"

	testState.RegisterWorker(workerUuid, workerNewName)
	json.NewDecoder(testLogBuffer).Decode(&testLogData)
	if testLogData.Message != expectedNewLogMessage {
		t.Errorf("Expected log message to be [%s], got [%s]", expectedNewLogMessage, testLogData.Message)
	}
	if testLogData.WorkerId.String() != expectedLogWorkerUuid.String() {
		t.Errorf("Expected worker uuid to be [%s], got [%s]", expectedLogWorkerUuid.String(), testLogData.WorkerId.String())
	}

	// Test retrieving from the testState
	workerIds := testState.GetWorkerIds()
	if len(workerIds) != 1 {
		t.Errorf("Expected to find only 1 worker in slice got from `getWorkerIds`, get %d", len(workerIds))
	}

	if workerIds[0].String() != workerUuid.String() {
		t.Errorf("Expected worker uuid to be [%s], got [%s]", workerUuid.String(), workerIds[0].String())
	}

	retrievedWorkerName, retrievedWorkerTaskCount := testState.GetWorkerMetadata(workerUuid)
	if retrievedWorkerName != workerNewName {
		t.Errorf("Expected retrieved worker name to be [%s], got [%s]", workerNewName, retrievedWorkerName)
	}
	if retrievedWorkerTaskCount != 0 {
		t.Errorf("Expected retrieved task count to be 0, got [%d]", retrievedWorkerTaskCount)
	}
}

func Test_rehydrate(t *testing.T) {
	testStore := createInMemoryStores()
	insertMockData(testStore)
	testState, _ := createTestState(t, testStore)

	if len(testState.workers) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(testState.workers))
	}

	// Check if all the inferred do exist in the original DB
	mapWorkerToTaskCount := 0

	for workerUuid, taskUuids := range testState.workerTaskMap {
		for _, taskUuid := range taskUuids {
			mapWorkerToTaskCount++

			workerStoredUuid, err := testState.taskWorkerDb.Get(taskUuid.String())
			if err != nil {
				t.Errorf("Error getting stored uuid of the worker from TaskWorkerDb")
			}

			if workerStoredUuid == nil {
				t.Errorf("Expected worker with uuid %s to be in the TaskWorkerDb, got `nil` instead", workerUuid.String())
			}
		}
	}

	// Check if everything relation is captured by the inferred map
	dbTaskToWorkerCount, err := testState.taskWorkerDb.Count()
	if err != nil {
		t.Errorf("Error getting counts of entries from TaskWorkerDb: %v", err)
	}

	if dbTaskToWorkerCount != mapWorkerToTaskCount {
		t.Errorf(
			"Number of workers from TaskWorkerDb: %d, Number of workers from WorkerTaskMap: %d",
			dbTaskToWorkerCount,
			mapWorkerToTaskCount,
		)
	}
}

func TestTaskManagement(t *testing.T) {
	testInMemoryStore := createInMemoryStores()
	testState, _ := createTestState(t, testInMemoryStore)

	testWorkerId := uuid.New()
	testWorkerName := "worker"

	err := testState.RegisterWorker(testWorkerId, testWorkerName)
	if err != nil {
		t.Fatalf("Failed to register the worker in the State component for testing: %v", err)
	}

	testTask := task.Task{
		ID:    uuid.New(),
		State: task.Pending,
	}

	err = testState.AssignTaskToWorker(&testTask, testWorkerId)
	if err != nil {
		t.Fatalf("Failed to assign task %#v to the worker with UUID %s : %v", testTask, testWorkerId.String(), err)
	}

	// Assigning the task to worker will turn the state of task from "pending" to "scheduled"
	if testTask.State != task.Scheduled {
		t.Errorf("Expected the state of testTask to be [%s], got [%s] instead", task.Scheduled.String(), testTask.State.String())
	}

	_, taskAssignedCount := testState.GetWorkerMetadata(testWorkerId)
	if taskAssignedCount != 1 {
		t.Errorf("Expected the number of task assigned to be 1, got %d instead", taskAssignedCount)
	}

	testTaskCopy := testTask
	testTaskCopy.State = task.Completed

	err = testState.UpdateTask(&testTaskCopy)
	if err != nil {
		t.Errorf("Error updating the testState with new task: %v", err)
	}

	updatedTask, err := testState.taskDb.Get(testTask.ID.String())
	if err != nil {
		t.Errorf("Error getting the updated task from testState: %v", err)
	}

	if diff := cmp.Diff(testTaskCopy, *updatedTask); diff != "" {
		t.Errorf("Task passed in the `UpdateTask` function is not the same as task returned, difference: %s", diff)
	}
}

// Detect the data race bug
// By spinning up many Go routines to modify the same storage at the same time

// Should use `go test -race ./...` when testing this
func TestState_Concurrent(t *testing.T) {
	testInMemoryStore := createInMemoryStores()
	insertMockData(testInMemoryStore)
	testState, _ := createTestState(t, testInMemoryStore)

	var wg sync.WaitGroup

	for range testConcurrentAccessCount {
		wg.Go(func() {
			// Get the works
			workerUuids := testState.GetWorkerIds()

			var subWg sync.WaitGroup

			for _, workerUUid := range workerUuids {
				subWg.Go(func() {
					// Register a new task
					testState.GetWorkerMetadata(workerUUid)
					newTask := task.Task{
						ID:    uuid.New(),
						State: task.Pending,
					}

					err := testState.AssignTaskToWorker(&newTask, workerUUid)
					if err != nil {
						t.Errorf("Failed to assign task [%s] to worker [%s]: %v", newTask.ID.String(), workerUUid.String(), err)
					}

					// Update the state of the task we just registered
					newTaskCopy := newTask
					newTaskCopy.State = task.Scheduled

					err = testState.UpdateTask(&newTaskCopy)
					if err != nil {
						t.Errorf("Failed to update task [%#v] to [%#v]: %v", newTask, newTaskCopy, err)
					}
				})
			}

			subWg.Wait()
		})
	}

	wg.Wait()
}
