package manager_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"testing"
	"time"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/mocks"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreatePersistentStateStores(t *testing.T) {
	tempDir := t.TempDir()
	tempDbFilePath := path.Join(tempDir, "test.db")

	testStores, err := manager.CreatePersistentStateStores(tempDbFilePath, 0600)

	assert.NoError(t, err)
	assert.NotNil(t, testStores)
}

func createTestManager(t *testing.T) (*manager.Manager, *bytes.Buffer, *mocks.MockCluster, *manager.MappingStorage) {
	t.Helper()

	var testManagerLogsBuffer bytes.Buffer
	testManagerLogger := logger.NewManagerLogger(true, &testManagerLogsBuffer)

	var testStateLogsBuffer bytes.Buffer
	testStateLogger := logger.NewManagerLogger(true, &testStateLogsBuffer)

	testRoundRobinScheduler := scheduler.RoundRobin{LastWorkerIdx: -1}

	testInMemoryStores := manager.CreateInMemoryStores()
	testInMemoryState, err := manager.NewState(*testInMemoryStores, *testStateLogger)
	assert.NoError(t, err)

	mockCluster := mocks.NewMockCluster(t)

	testManager := manager.NewManager(testInMemoryState, mockCluster, &testRoundRobinScheduler, *testManagerLogger)

	return testManager, &testManagerLogsBuffer, mockCluster, testInMemoryState
}

func TestNew(t *testing.T) {
	testRoundRobinScheduler := scheduler.RoundRobin{LastWorkerIdx: -1}

	var testManagerLogsBuffer bytes.Buffer
	testManagerLogger := logger.NewManagerLogger(true, &testManagerLogsBuffer)

	testInMemoryStores := manager.CreateInMemoryStores()

	newManager, err := manager.New(&testRoundRobinScheduler, *testManagerLogger, testInMemoryStores)
	assert.NoError(t, err)
	assert.NotNil(t, newManager)
}

func TestUpdateWorkerNodes(t *testing.T) {
	testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

	// Expect the call to `testInMemoryState` with `testInMemoryState` as parameter
	// Fail if this mock is not called that way

	// Return nothing when called that way
	// For only one time
	mockCluster.EXPECT().UpdateWorkerNodes(testInMemoryState).Return().Once()

	testManager.UpdateWorkerNodes()

	assert.Equal(t, "", testManagerLogBuf.String(), "Buffer containing logs for manager should be empty")
}

func TestAddWorkerAndClient(t *testing.T) {
	testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

	testWorkerUuid := uuid.New()
	testWorkerName := "test-worker-1"
	testWorker := worker.NewWorker(testWorkerUuid, testWorkerName, *queue.New(), "memory")

	testClient := http.DefaultClient

	mockCluster.EXPECT().AddClient(testWorkerUuid, testClient).Return().Once()

	testManager.AddWorkerAndClient(testWorker, testClient)

	testWorkerNameFromState, testWorkerTaskCountFromState := testInMemoryState.GetWorkerMetadata(testWorkerUuid)

	assert.Equal(t, testWorkerName, testWorkerNameFromState, "Worker name should be stored in the state component")
	assert.Equal(t, 0, testWorkerTaskCountFromState, "New worker added to state should have 0 tasks")
	assert.Equal(t, "", testManagerLogBuf.String(), "Buffer containing logs for manager should be empty")
}

func TestSelectWorker(t *testing.T) {

	t.Run("Runs on valid node list for multiple iterations", func(t *testing.T) {
		testManager, _, mockCluster, testInMemoryState := createTestManager(t)

		// `m.TestSelectWorker` calls `m.UpdateWorkerNode`, who calls `m.cluster.UpdateWorkerNodes`
		// To update the stats of all nodes
		mockCluster.EXPECT().UpdateWorkerNodes(testInMemoryState).Return()

		mockNodesFromCluster := []node.Node{
			{Name: "node-1", WorkerUuid: uuid.New()},
			{Name: "node-2", WorkerUuid: uuid.New()},
		}

		// Always return this slice to test the scheduler logic
		mockCluster.EXPECT().GetNodes().Return(mockNodesFromCluster)

		testTask := task.Task{
			ID: uuid.New(),
		}

		assertSelectedNode := func(t *testing.T, expectedNode node.Node, notMatchErrMsg string) {
			t.Helper()

			selectedWorkerUuid, err := testManager.SelectWorker(testTask)

			assert.Equal(
				t,
				expectedNode.WorkerUuid.String(),
				selectedWorkerUuid.String(),
				notMatchErrMsg,
			)
			assert.NoError(t, err)
		}

		assertSelectedNode(
			t,
			mockNodesFromCluster[0],
			"First worker in the node list should be selected when manager first runs round robin",
		)

		assertSelectedNode(
			t,
			mockNodesFromCluster[1],
			"Second worker in the node list should be selected when manager second time runs round robin",
		)

		assertSelectedNode(
			t,
			mockNodesFromCluster[0],
			"First worker in the node list should be selected after the first iteration",
		)
	})

	t.Run("Runs on an empty list of nodes, error expected", func(t *testing.T) {
		testManager, _, mockCluster, testInMemoryState := createTestManager(t)

		mockCluster.EXPECT().UpdateWorkerNodes(testInMemoryState).Return()

		mockEmptyNodeListFromCluster := []node.Node{}

		mockCluster.EXPECT().GetNodes().Return(mockEmptyNodeListFromCluster)

		testTask := task.Task{
			ID: uuid.New(),
		}

		selectedWorkerUuid, err := testManager.SelectWorker(testTask)

		assert.ErrorContains(
			t,
			err,
			fmt.Sprintf("no available candidates match resource request for task `%s`", testTask.ID.String()),
		)
		assert.Equal(t, uuid.Nil, selectedWorkerUuid, "The Nil uuid should be returned")
	})

}

func TestSendWork(t *testing.T) {
	addTestWorkerAndClient := func(testManager *manager.Manager) (*worker.Worker, *http.Client) {
		testWorkerUuid := uuid.New()
		testWorkerName := "test-worker-1"
		testWorker := worker.NewWorker(testWorkerUuid, testWorkerName, *queue.New(), "memory")
		testClient := http.DefaultClient
		testManager.AddWorkerAndClient(testWorker, testClient)

		return testWorker, testClient
	}

	addTestTaskEvent := func(testManager *manager.Manager, taskState task.State, targetState task.State) task.TaskEvent {
		testTask := task.Task{
			ID:    uuid.New(),
			Name:  "test-task-1",
			State: taskState,
		}

		testTaskEvent := task.TaskEvent{
			ID:          uuid.New(),
			TargetState: targetState,
			Timestamp:   time.Now(),
			Task:        testTask,
		}

		testManager.AddTaskEvent(testTaskEvent)
		return testTaskEvent
	}

	type TaskSentLog struct {
		Message   string         `json:"msg"`
		Level     string         `json:"level"`
		TaskEvent task.TaskEvent `json:"task_event"`
	}

	// See if the task is updated as "failed" in the state
	assertFailedTaskInState := func(t *testing.T, originalTask task.Task, testState manager.State) {
		t.Helper()

		expectedUpdatedTask := originalTask
		expectedUpdatedTask.State = task.Failed

		updatedTask, err := testState.GetTask(originalTask.ID)

		assert.NoError(t, err)
		assert.Equal(
			t,
			expectedUpdatedTask,
			*updatedTask,
			"Task stored in state should be updated to have `Failed` as its state",
		)
	}

	t.Run("No taskEvents in manager's queue", func(t *testing.T) {
		testManager, _, _, _ := createTestManager(t)

		targetEvent, err := testManager.SendWork()
		assert.Nil(
			t,
			targetEvent,
			"`targetEvent` returned should be nil when there're no pending taskEvents in manager queue",
		)
		assert.Error(t, err, "no TaskEvent left in the pending queue")
	})

	// Sending to start the task
	t.Run("One taskEvent (with `Running` as `targetState`) in manager's queue", func(*testing.T) {
		testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

		mockCluster.EXPECT().AddClient(mock.Anything, mock.Anything).Return().Once()
		mockCluster.EXPECT().UpdateWorkerNodes(testInMemoryState).Return().Once()

		// Preparation: insert the `testTaskEvent` to the queue
		testTaskEvent := addTestTaskEvent(testManager, task.Pending, task.Running)
		// The taskEvent expect to be returned by the `SendWork` function
		expectedSentTaskEvent := testTaskEvent
		expectedSentTaskEvent.Task.State = task.Scheduled

		// Preparation: Add a worker and its client to the manager
		testWorker, _ := addTestWorkerAndClient(testManager)

		mockCluster.EXPECT().GetNodes().Return([]node.Node{{Name: testWorker.Name, WorkerUuid: testWorker.Uuid}}).Once()
		mockCluster.EXPECT().SendTask(testWorker.Uuid, mock.Anything).Return(&testTaskEvent.Task, nil).Once()

		targetTaskEvent, err := testManager.SendWork()
		assert.Equal(
			t,
			expectedSentTaskEvent,
			*targetTaskEvent,
			"A copy of the only TaskEvent stored in the manager (with state set to scheduled) should be returned",
		)
		assert.NoError(t, err)

		assignedWorkerId, err := testInMemoryState.GetAssignedWorker(testTaskEvent.Task.ID)
		assert.Equal(
			t,
			testWorker.Uuid.String(),
			assignedWorkerId.String(),
			"Task should be assigned to the only worker",
		)
		assert.NoError(t, err)

		var testTaskSentLog TaskSentLog
		json.NewDecoder(testManagerLogBuf).Decode(&testTaskSentLog)

		assert.Equal(t, "INFO", testTaskSentLog.Level)
		assert.Equal(t, "Task sent to worker", testTaskSentLog.Message)
		assert.Equal(
			t,
			"",
			cmp.Diff(testTaskSentLog.TaskEvent, *targetTaskEvent),
			"TaskEvent sent to worker should be no different from the target TaskEvent returned from `SendWork`",
		)
	})

	// Sending to stop the task
	t.Run("One taskEvent (with `Completed` as `targetState`) in manager's queue", func(*testing.T) {
		testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

		mockCluster.EXPECT().AddClient(mock.Anything, mock.Anything).Return().Once()

		// Preparation: insert the `testTaskEvent` to the queue
		testTaskEvent := addTestTaskEvent(testManager, task.Running, task.Completed)
		// The taskEvent expect to be returned by the `SendWork` function

		// Preparation: Add a worker and its client to the manager
		testWorker, _ := addTestWorkerAndClient(testManager)

		// Preparation: assign the task to the worker
		// So the manager sees the worker as "currently working on" this task
		_, err := testInMemoryState.AssignTaskToWorker(testTaskEvent.Task, testWorker.Uuid)
		assert.NoError(t, err)

		// mockCluster.EXPECT().GetNodes().Return([]node.Node{{Name: testWorker.Name, WorkerUuid: testWorker.Uuid}}).Once()
		mockCluster.EXPECT().StopTask(testWorker.Uuid, mock.Anything).Return(nil).Once()

		targetTaskEvent, err := testManager.SendWork()

		// Check the return value
		assert.Equal(
			t,
			testTaskEvent,
			*targetTaskEvent,
			"A copy of the only TaskEvent stored in the manager should be returned",
		)
		assert.NoError(t, err)

		// Check the logs
		var testTaskSentLog TaskSentLog
		json.NewDecoder(testManagerLogBuf).Decode(&testTaskSentLog)

		assert.Equal(t, "INFO", testTaskSentLog.Level)
		assert.Equal(t, "Task sent to worker", testTaskSentLog.Message)
		assert.Equal(
			t,
			testTaskEvent,
			*targetTaskEvent,
			"TaskEvent sent to worker should be no different from the target TaskEvent returned from `SendWork`",
		)
	})

	t.Run("Mock cluster failed to do SendTask", func(t *testing.T) {
		testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

		mockCluster.EXPECT().AddClient(mock.Anything, mock.Anything).Return().Once()
		mockCluster.EXPECT().UpdateWorkerNodes(testInMemoryState).Return().Once()

		// Preparation: insert the `testTaskEvent` to the queue
		testTaskEvent := addTestTaskEvent(testManager, task.Pending, task.Running)
		// The taskEvent expect to be returned by the `SendWork` function
		expectedSentTaskEvent := testTaskEvent
		expectedSentTaskEvent.Task.State = task.Scheduled

		// Preparation: Add a worker and its client to the manager
		testWorker, _ := addTestWorkerAndClient(testManager)

		mockCluster.EXPECT().GetNodes().Return([]node.Node{{Name: testWorker.Name, WorkerUuid: testWorker.Uuid}}).Once()
		mockCluster.EXPECT().SendTask(testWorker.Uuid, mock.Anything).Return(nil, errors.New("dummy error")).Once()

		targetTaskEvent, err := testManager.SendWork()
		assert.ErrorContains(
			t,
			err,
			fmt.Sprintf(
				"failed to send the task to target worker %s: %v",
				testWorker.Uuid.String(),
				fmt.Errorf("dummy error"),
			),
		)
		assert.Equal(
			t,
			expectedSentTaskEvent,
			*targetTaskEvent,
			"A copy of the only TaskEvent stored in the manager (with state set to scheduled) should be returned",
		)

		// Check the logs
		assert.Equal(t, "", testManagerLogBuf.String(), "Should not print logs when the SendTask fails")

		// Check the cleanup in defer block
		assertFailedTaskInState(t, testTaskEvent.Task, testInMemoryState)
	})

	t.Run("Mock cluster failed to do StopTask", func(t *testing.T) {
		testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

		mockCluster.EXPECT().AddClient(mock.Anything, mock.Anything).Return().Once()

		// Preparation: insert the `testTaskEvent` to the queue
		testTaskEvent := addTestTaskEvent(testManager, task.Running, task.Completed)
		// The taskEvent expect to be returned by the `SendWork` function

		// Preparation: Add a worker and its client to the manager
		testWorker, _ := addTestWorkerAndClient(testManager)

		// Preparation: assign the task to the worker
		// So the manager sees the worker as "currently working on" this task
		_, err := testInMemoryState.AssignTaskToWorker(testTaskEvent.Task, testWorker.Uuid)
		assert.NoError(t, err)

		// mockCluster.EXPECT().GetNodes().Return([]node.Node{{Name: testWorker.Name, WorkerUuid: testWorker.Uuid}}).Once()
		mockCluster.EXPECT().StopTask(testWorker.Uuid, mock.Anything).Return(errors.New("dummy error")).Once()

		targetTaskEvent, err := testManager.SendWork()

		// Check the return value
		assert.Equal(
			t,
			testTaskEvent,
			*targetTaskEvent,
			"A copy of the only TaskEvent stored in the manager should be returned",
		)
		assert.ErrorContains(
			t,
			err,
			fmt.Sprintf("error stopping task: %v", fmt.Errorf("dummy error")),
		)

		// Check the logs
		assert.Equal(t, "", testManagerLogBuf.String(), "Should not print logs when the SendTask fails")

		// Check the cleanup in defer block
		assertFailedTaskInState(t, testTaskEvent.Task, testInMemoryState)
	})

	t.Run("No worker available, failed in `determineWorker`", func(t *testing.T) {
		testManager, testManagerLogBuf, mockCluster, testInMemoryState := createTestManager(t)

		// mockCluster.EXPECT().AddClient(mock.Anything, mock.Anything).Return().Once()
		mockCluster.EXPECT().UpdateWorkerNodes(testInMemoryState).Return().Once()

		// Preparation: insert the `testTaskEvent` to the queue
		testTaskEvent := addTestTaskEvent(testManager, task.Pending, task.Running)
		// The taskEvent expect to be returned by the `SendWork` function
		// expectedSentTaskEvent := testTaskEvent
		// expectedSentTaskEvent.Task.State = task.Scheduled

		// Preparation: Add a worker and its client to the manager
		// testWorker, _ := addTestWorkerAndClient(testManager)

		mockCluster.EXPECT().GetNodes().Return([]node.Node{}).Once()
		// mockCluster.EXPECT().SendTask(testWorker.Uuid, mock.Anything).Return(nil, errors.New("dummy error")).Once()

		targetTaskEvent, err := testManager.SendWork()
		assert.ErrorContains(
			t,
			err,
			fmt.Sprintf(
				"failed to schedule the task: %v",
				fmt.Errorf(
					"no available candidates match resource request for task `%s`",
					testTaskEvent.Task.ID.String(),
				),
			),
		)
		assert.Equal(
			t,
			testTaskEvent,
			*targetTaskEvent,
			"A copy of the only TaskEvent stored in the manager (with state set to scheduled) should be returned",
		)

		// Check the logs
		assert.Equal(t, "", testManagerLogBuf.String(), "Should not print logs when the SendTask fails")

		// Did not get to assign the task to a worker
		// No task in the manager state
		// Hence the task won't be marked as failed in the state
		taskFromManagerState, err := testInMemoryState.GetTask(targetTaskEvent.Task.ID)
		assert.Nil(t, taskFromManagerState)
		assert.NoError(t, err)
	})
}
