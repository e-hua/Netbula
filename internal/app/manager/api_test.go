package manager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/mocks"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type LogData struct {
	Message string `json:"msg"`
	Level   string `json:"level"`
}

type ErrorLog struct {
	LogData
	Err string `json:"error"`
}

type TaskReceivedLog struct {
	LogData
}

var (
	mockTasks = []*task.Task{
		{
			ID:    uuid.New(),
			Name:  "task-1",
			State: task.Completed,
		},
		{
			ID:    uuid.New(),
			Name:  "task-2",
			State: task.Pending,
			Image: "node",
		},
	}
)

func assertResponseRecorderBodyContent[T any](t *testing.T, responseRecorder *httptest.ResponseRecorder, expectedValue T) {
	t.Helper()

	var receivedValue T
	err := json.NewDecoder(responseRecorder.Body).Decode(&receivedValue)

	assert.NoError(t, err)
	assert.Equal(t, expectedValue, receivedValue)
}

func assertErrorLog(t *testing.T, logsBuffer *bytes.Buffer, expectedLogMsg string, expectedErrMsg string) {
	t.Helper()

	var errorLog ErrorLog
	err := json.NewDecoder(logsBuffer).Decode(&errorLog)

	assert.NoError(t, err)
	assert.Equal(t, "ERROR", errorLog.Level)
	assert.Contains(t, errorLog.Message, expectedLogMsg)
	assert.Contains(t, errorLog.Err, expectedErrMsg)
}

func assertTaskReceivedLog(t *testing.T, logsBuffer *bytes.Buffer) {
	var taskReceivedLog TaskReceivedLog
	err := json.NewDecoder(logsBuffer).Decode(&taskReceivedLog)

	assert.NoError(t, err)
	assert.Equal(t, "INFO", taskReceivedLog.Level)
	assert.Equal(t, "Task received from control", taskReceivedLog.Message)
}

func createTestApi(t *testing.T) (manager.Api, *bytes.Buffer, *mocks.MockManagerService) {
	t.Helper()

	var testManagerLogsBuffer bytes.Buffer
	testManagerLogger := logger.NewManagerLogger(true, &testManagerLogsBuffer)

	mockManager := mocks.NewMockManagerService(t)

	testApi := manager.Api{
		Manager:  mockManager,
		TlsToken: "dummy-token",

		Logger: *testManagerLogger,
	}

	testApi.InitRouter(false)

	return testApi, &testManagerLogsBuffer, mockManager
}

func TestApi_Authenticate(t *testing.T) {
	testCases := []struct {
		caseName      string
		testAuthToken string

		mockTasksFromManager []*task.Task

		expectedResponseCode    int
		expectedResponseBodyErr string

		canAuthenticate bool
	}{
		{
			caseName:      "No token in header",
			testAuthToken: "",

			mockTasksFromManager: mockTasks,

			expectedResponseCode:    http.StatusUnauthorized,
			expectedResponseBodyErr: "Unauthorized\n",

			canAuthenticate: false,
		},
		{
			caseName:      "Wrong token in header",
			testAuthToken: "wrong-token",

			mockTasksFromManager: mockTasks,

			expectedResponseCode:    http.StatusUnauthorized,
			expectedResponseBodyErr: "Unauthorized\n",

			canAuthenticate: false,
		},
		{
			caseName:      "Correct token in header",
			testAuthToken: "dummy-token",

			mockTasksFromManager: mockTasks,

			expectedResponseCode:    http.StatusOK,
			expectedResponseBodyErr: "",

			canAuthenticate: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			testApi, _, mockManager := createTestApi(t)

			if testCase.canAuthenticate {
				mockManager.EXPECT().GetTasks().Return(testCase.mockTasksFromManager, nil).Once()
			}

			req, err := http.NewRequest(http.MethodGet, "/tasks", nil)
			assert.NoError(t, err)
			if testCase.testAuthToken != "" {
				req.Header.Set(manager.AuthenticationHeaderKey, testCase.testAuthToken)
			}

			responseRecorder := httptest.NewRecorder()

			testApi.Router.ServeHTTP(responseRecorder, req)

			assert.Equal(t, testCase.expectedResponseCode, responseRecorder.Code)
			if testCase.canAuthenticate {
				assertResponseRecorderBodyContent(t, responseRecorder, testCase.mockTasksFromManager)
			} else {
				assert.Equal(t, testCase.expectedResponseBodyErr, responseRecorder.Body.String())
			}
		})
	}
}

func assertErrorInResponseRecorder(t *testing.T, responseRecorder *httptest.ResponseRecorder, expectedErrMsg string) {
	t.Helper()

	var errResponse routers.ErrResponse
	decoder := json.NewDecoder(responseRecorder.Body)

	decoder.DisallowUnknownFields()
	err := decoder.Decode(&errResponse)

	assert.NoError(t, err)
	assert.Contains(t, errResponse.Message, expectedErrMsg)
}

func TestApi_StartTaskHandler(t *testing.T) {
	testSentTask := task.Task{
		ID:    uuid.New(),
		Name:  "postgres",
		State: task.Pending,
		Image: "postgres:alpine",
	}
	testSentTaskEvent := task.TaskEvent{
		ID:          uuid.New(),
		TargetState: task.Running,
		Timestamp:   time.Unix(0, 0),
		Task:        testSentTask,
	}

	testCases := []struct {
		caseName string

		testRequestContent any

		expectedResponseCode int
		isTaskReceived       bool
	}{
		{
			caseName: "Valid task event in the request",

			testRequestContent: testSentTaskEvent,

			expectedResponseCode: http.StatusCreated,
			isTaskReceived:       true,
		},
		{
			caseName: "Unknown field in the request",

			testRequestContent: struct {
				task.TaskEvent
				InvalidField string
			}{
				TaskEvent:    testSentTaskEvent,
				InvalidField: "some-new-value",
			},

			expectedResponseCode: http.StatusBadRequest,
			isTaskReceived:       false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			testApi, testManagerLogsBuffer, mockManager := createTestApi(t)

			var testBody bytes.Buffer
			err := json.NewEncoder(&testBody).Encode(testCase.testRequestContent)
			assert.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "/tasks", &testBody)
			assert.NoError(t, err)
			responseRecorder := httptest.NewRecorder()

			if testCase.isTaskReceived {
				mockManager.EXPECT().AddTaskEvent(testSentTaskEvent).Return().Once()
			}

			testApi.StartTaskHandler(responseRecorder, req)

			assert.Equal(t, testCase.expectedResponseCode, responseRecorder.Code)
			if testCase.isTaskReceived {
				assertTaskReceivedLog(t, testManagerLogsBuffer)
			} else {
				expectedDecodeErr := fmt.Errorf("json: unknown field %q", "InvalidField")
				expectedResponseErr := fmt.Errorf("failed to unmarshal body: %w", expectedDecodeErr)

				assertErrorInResponseRecorder(t, responseRecorder, expectedResponseErr.Error())

				// Check the logs
				assertErrorLog(
					t,
					testManagerLogsBuffer,
					"Failed when handling request to start a task",
					expectedResponseErr.Error(),
				)
			}
		})
	}
}

func TestApi_GetTasksHandler(t *testing.T) {
	mockErrFromManager := fmt.Errorf("dummy error")

	testCases := []struct {
		caseName string

		mockTasksFromManager []*task.Task
		mockErrFromManager   error

		expectedTasksInResponse  []*task.Task
		expectedErrMsgInResponse error
		expectedLogMsg           string
	}{
		{
			caseName: "Manager returns valid list of tasks",

			mockTasksFromManager: mockTasks,
			mockErrFromManager:   nil,

			expectedTasksInResponse:  mockTasks,
			expectedErrMsgInResponse: nil,
		},
		{
			caseName: "Manager returns error",

			mockTasksFromManager: nil,
			mockErrFromManager:   mockErrFromManager,

			expectedTasksInResponse:  nil,
			expectedErrMsgInResponse: fmt.Errorf("failed to get tasks from TaskDb: %w", mockErrFromManager),
			expectedLogMsg:           "Failed when handling request to list tasks",
		},
		{
			caseName: "Manager returns nil for  list of tasks",

			mockTasksFromManager: nil,
			mockErrFromManager:   nil,

			expectedTasksInResponse:  []*task.Task{},
			expectedErrMsgInResponse: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			testApi, testManagerLogsBuffer, mockManager := createTestApi(t)
			mockManager.EXPECT().GetTasks().Return(testCase.mockTasksFromManager, testCase.mockErrFromManager).Once()

			req, err := http.NewRequest(http.MethodGet, "/tasks", nil)
			assert.NoError(t, err)

			responseRecorder := httptest.NewRecorder()

			testApi.GetTasksHandler(responseRecorder, req)

			if testCase.expectedErrMsgInResponse != nil {
				assertErrorInResponseRecorder(t, responseRecorder, testCase.expectedErrMsgInResponse.Error())
				// Check the logs
				assertErrorLog(
					t,
					testManagerLogsBuffer,
					testCase.expectedLogMsg,
					testCase.expectedErrMsgInResponse.Error(),
				)
			} else {
				assertResponseRecorderBodyContent(t, responseRecorder, testCase.expectedTasksInResponse)
			}

		})
	}
}

func TestApi_StopTaskHandler(t *testing.T) {

	mockTask := task.Task{
		ID:    uuid.New(),
		Name:  "test-task-1",
		State: task.Running,
		Image: "node:alpine",
	}

	testInvalidUuid := "Some-invalid-uuid"
	testNotExistingUuid := uuid.New().String()

	testCases := []struct {
		caseName string

		testTaskId        string
		mockGetTaskResult *task.Task
		mockGetTaskErr    error

		isTaskIdValid             bool
		isTaskEventAddedToManager bool
		expectedErrorLogMsg       string
		expectedResErrMsg         string

		expectedHttpResCode int
	}{
		{
			caseName:                  "Valid taskId in request url and task found in manager, taskEvent successfully added",
			testTaskId:                mockTask.ID.String(),
			mockGetTaskResult:         &mockTask,
			mockGetTaskErr:            nil,
			isTaskIdValid:             true,
			isTaskEventAddedToManager: true,
			expectedErrorLogMsg:       "",
			expectedResErrMsg:         "",
			expectedHttpResCode:       http.StatusNoContent,
		},
		{
			caseName:                  "Invalid taskId in request url, no taskEvent added",
			testTaskId:                testInvalidUuid,
			mockGetTaskResult:         nil,
			mockGetTaskErr:            nil, // Won't reach this stop, hence not considering return values
			isTaskIdValid:             false,
			isTaskEventAddedToManager: false,
			expectedErrorLogMsg:       "Failed when handling request to stop task",
			expectedResErrMsg:         fmt.Sprintf("failed to parse taskId %s as UUID", testInvalidUuid),
			expectedHttpResCode:       http.StatusBadRequest,
		},
		{
			caseName:                  "Valid taskId in request url but task not matching task in manager, no taskEvent added",
			testTaskId:                testNotExistingUuid,
			mockGetTaskResult:         nil,
			mockGetTaskErr:            nil,
			isTaskIdValid:             true,
			isTaskEventAddedToManager: false,
			expectedErrorLogMsg:       "Failed when handling request to stop task",
			expectedResErrMsg:         fmt.Sprintf("failed to get task with ID [%s] from TaskDb: ", testNotExistingUuid),
			expectedHttpResCode:       http.StatusNotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			testApi, testManagerLogsBuffer, mockManager := createTestApi(t)

			// 	If the ID is parsed successfully, `manager.GetTask` will be called
			if testCase.isTaskIdValid {
				mockManager.EXPECT().GetTask(mock.Anything).Return(testCase.mockGetTaskResult, testCase.mockGetTaskErr).Once()
			}

			if testCase.isTaskEventAddedToManager {
				mockManager.EXPECT().AddTaskEvent(mock.Anything).Return().Once()
			}

			// Add chi path parameter to the request
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("tasks/%s", testCase.testTaskId), nil)
			assert.NoError(t, err)

			routeCtx := chi.NewRouteContext()
			routeCtx.URLParams.Add("taskId", testCase.testTaskId)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))

			responseRecorder := httptest.NewRecorder()

			testApi.StopTaskHandler(responseRecorder, req)

			assert.Equal(t, testCase.expectedHttpResCode, responseRecorder.Code)
			if testCase.expectedErrorLogMsg != "" {
				assertErrorLog(t, testManagerLogsBuffer, testCase.expectedErrorLogMsg, testCase.expectedResErrMsg)
			}
			if testCase.expectedResErrMsg != "" {
				assertErrorInResponseRecorder(t, responseRecorder, testCase.expectedResErrMsg)
			}
			if testCase.isTaskEventAddedToManager {
				assertTaskReceivedLog(t, testManagerLogsBuffer)
			}
		})
	}
}

func TestApi_GetNodesHandler(t *testing.T) {
	mockNodes := []node.Node{
		{Name: "node-1", WorkerUuid: uuid.New()},
		{Name: "node-2", WorkerUuid: uuid.New()},
	}

	testCases := []struct {
		caseName string

		mockNodesFromManager      []node.Node
		expectedNodesFromResponse []node.Node
	}{
		{
			caseName:                  "Valid array of nodes returned from manager",
			mockNodesFromManager:      mockNodes,
			expectedNodesFromResponse: mockNodes,
		},
		{
			caseName:                  "`nil` returned from manager",
			mockNodesFromManager:      nil,
			expectedNodesFromResponse: []node.Node{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			testApi, testManagerLogsBuffer, mockManager := createTestApi(t)

			mockManager.EXPECT().UpdateWorkerNodes().Return().Once()
			mockManager.EXPECT().GetNodes().Return(testCase.mockNodesFromManager).Once()

			req, err := http.NewRequest(http.MethodGet, "nodes/", nil)
			assert.NoError(t, err)

			responseRecorder := httptest.NewRecorder()

			testApi.GetNodesHandler(responseRecorder, req)

			assert.Equal(t, http.StatusOK, responseRecorder.Code)
			assertResponseRecorderBodyContent(t, responseRecorder, testCase.expectedNodesFromResponse)
			assert.Equal(t, "", testManagerLogsBuffer.String())
		})
	}
}
