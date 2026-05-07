package manager_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/configs"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/mocks"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type TestListener struct {
	conn         net.Conn
	returnedOnce sync.Once
	// A channel without buffer, blocks on the first write
	chanToBlock chan int
}

func InitTestListener(conn net.Conn) *TestListener {
	return &TestListener{
		conn:         conn,
		returnedOnce: sync.Once{},
		chanToBlock:  make(chan int),
	}
}

// `Accept` will return the connection when it's first called
// And blocks for the rest of the time

// Returns an error if the connection is closed
func (l *TestListener) Accept() (net.Conn, error) {
	var isFirstCall bool

	l.returnedOnce.Do(func() {
		isFirstCall = true
	})

	if isFirstCall {
		return l.conn, nil
	}

	// Block if the connection was returned before
	// Unblocks when the channel is closed
	<-l.chanToBlock
	return nil, errors.New("test listener is closed")
}

// Any blocked `Accept` operations will be unblocked and return errors.
func (l *TestListener) Close() error {
	// If the `Close` is called before any `Accept`
	// Then even the first `Accept` call will return error
	l.returnedOnce.Do(func() {})

	close(l.chanToBlock)

	return l.conn.Close()
}

func (l *TestListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func TestNewApp(t *testing.T) {
	_, testManagerToWorkerConn := net.Pipe()
	_, testManagerToControlConn := net.Pipe()

	testManagerToWorkerListener := InitTestListener(testManagerToWorkerConn)
	testManagerToControlListener := InitTestListener(testManagerToControlConn)

	testCert, testToken, testCertFingerprint := security.GenerateManagerIdentity()
	testManagerConfigs := configs.NewManagerConfig(0, 0, testCert, testToken, testCertFingerprint)

	testAppConfigs := manager.AppConfigs{
		LogDest:   io.Discard,
		Scheduler: &scheduler.RoundRobin{LastWorkerIdx: -1},

		AllowVerbose: true,
		IsPersistent: false,

		ManagerConfigs: *testManagerConfigs,

		WorkerConnectionListener: testManagerToWorkerListener,
		ManagerServerApiListener: testManagerToControlListener,
	}

	newApp, err := manager.NewApp(testAppConfigs)

	assert.NoError(t, err)
	assert.NotNil(t, newApp.Manager)
	assert.NotNil(t, newApp.Api)
	assert.NotNil(t, newApp.Logger)

	assert.NotNil(t, newApp.WorkerConnectionTlsListener)
	assert.NotNil(t, newApp.ControlConnectionTlsListener)

	assert.NotNil(t, newApp.ManagerConfigs)
}

func CreateTlsClientFromConn(conn net.Conn) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				tlsConfig := &tls.Config{
					InsecureSkipVerify: true,
				}
				tlsConn := tls.Client(conn, tlsConfig)

				return tlsConn, nil
			},
		},
	}
}

func TestApp_Run(t *testing.T) {
	mockWorker := worker.Worker{
		Name: "test-worker",
		Uuid: uuid.New(),
	}
	mockNodes := []node.Node{
		{
			Name:  "mock-node-1",
			Cores: 8,
		},
	}

	testWorkerToManagerConn, testManagerToWorkerConn := net.Pipe()
	testControlToManagerConn, testManagerToControlConn := net.Pipe()

	testManagerToWorkerListener := InitTestListener(testManagerToWorkerConn)
	testManagerToControlListener := InitTestListener(testManagerToControlConn)

	testCert, testToken, testCertFingerprint := security.GenerateManagerIdentity()
	testManagerConfigs := configs.NewManagerConfig(0, 0, testCert, testToken, testCertFingerprint)

	// Start the worker server
	testWorkerTlsConn := tls.Client(testWorkerToManagerConn, security.GenerateTlsConfig(testCertFingerprint))
	testWorkerTlsListener, err := yamux.Server(testWorkerTlsConn, nil)
	assert.NoError(t, err)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(&mockWorker)
		})
		http.Serve(testWorkerTlsListener, mux)
	}()

	testControlTlsClient := CreateTlsClientFromConn(testControlToManagerConn)

	mockManager := mocks.NewMockManagerService(t)

	mockManager.EXPECT().SendTasksForever(mock.Anything).Return()
	mockManager.EXPECT().UpdateTasksForever(mock.Anything).Return()

	mockManager.EXPECT().UpdateWorkerNodes().Return()
	mockManager.EXPECT().GetNodes().Return(mockNodes)

	// Channel with a buffer of length 1 to signal the function is called
	called := make(chan struct{}, 1)

	mockManager.EXPECT().AddWorkerAndClient(mock.Anything, mock.Anything).Run(func(_ *worker.Worker, _ *http.Client) {
		called <- struct{}{}
	}).Return()

	testApp := &manager.App{
		Manager: mockManager,
		Api: manager.Api{
			Manager:  mockManager,
			AuthToken: testToken,
			Logger:   *logger.NewManagerLogger(true, io.Discard),
		},
		Logger:                       *logger.NewManagerLogger(true, io.Discard),
		WorkerConnectionTlsListener:  manager.CreateTlsListener(testCert, testManagerToWorkerListener),
		ControlConnectionTlsListener: manager.CreateTlsListener(testCert, testManagerToControlListener),

		ManagerConfigs: *testManagerConfigs,
	}

	testCtx, cancel := context.WithCancel(context.Background())
	go testApp.Run(testCtx)

	req, err := http.NewRequest(http.MethodGet, "https://manager/nodes", nil)
	assert.NoError(t, err)
	req.Header.Set(manager.AuthenticationHeaderKey, testToken)
	resp, err := testControlTlsClient.Do(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assertReaderContent(t, resp.Body, mockNodes)

	// Wait for one second before calling `cancel()`
	// Prevent the test function from exiting too early, causing the expectation on `AddWorkerAndClient` to fail
	select {
	case <-called:
	// `AddWorkerAndClient` is called
	case <-time.After(time.Second):
		t.Error("AddWorkerAndClient is not called within 1 second")
	}

	cancel()
}
