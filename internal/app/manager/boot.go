package manager

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/configs"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/hashicorp/yamux"
)

type AppConfigs struct {
	LogDest   io.Writer
	Scheduler scheduler.Scheduler

	AllowVerbose bool
	// Set this to true if wants persistent storage
	IsPersistent bool
	// Only need these if `IsPersistent` is true
	ManagerDbPath     string
	ManagerDbFileMode os.FileMode

	ManagerConfigs configs.ManagerConfig

	WorkerConnectionListener net.Listener
	ManagerServerApiListener net.Listener
}

type App struct {
	Manager ManagerService
	Api     Api
	Logger  logger.ManagerLogger

	WorkerConnectionTlsListener  net.Listener
	ControlConnectionTlsListener net.Listener

	ManagerConfigs configs.ManagerConfig
}

func NewApp(configs AppConfigs) (*App, error) {
	// Create TLS listeners
	cert := tls.Certificate{
		Certificate: configs.ManagerConfigs.TlsCertificateInBytes,
		PrivateKey:  ed25519.PrivateKey(configs.ManagerConfigs.TlsPrivateKey),
	}

	workerConnectionTlsListener := CreateTlsListener(cert, configs.WorkerConnectionListener)
	controlConnectionTlsListener := CreateTlsListener(cert, configs.ManagerServerApiListener)

	// Initialize the Manager + API
	managerLogger := logger.NewManagerLogger(configs.AllowVerbose, configs.LogDest)

	var stateStores *stateStores
	var err error

	if configs.IsPersistent {
		stateStores, err = CreatePersistentStateStores(configs.ManagerDbPath, configs.ManagerDbFileMode)
	} else {
		stateStores = CreateInMemoryStores()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create DBs providing persistent storage for manager: %w", err)
	}

	newManager, err := New(configs.Scheduler, *managerLogger, stateStores)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize manager: %w", err)
	}

	managerApi := Api{
		Manager:  newManager,
		AuthToken: configs.ManagerConfigs.AuthToken,
		Logger:   *logger.NewManagerLoggerWithSubsystem(*managerLogger, "api"),
	}

	return &App{
		Manager: newManager,
		Api:     managerApi,
		Logger:  *managerLogger,

		WorkerConnectionTlsListener:  workerConnectionTlsListener,
		ControlConnectionTlsListener: controlConnectionTlsListener,

		ManagerConfigs: configs.ManagerConfigs,
	}, nil
}

func CreateTcpListener(port int) (net.Listener, error) {
	tcpListener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))

	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	return tcpListener, nil
}

func CreateTlsListener(cert tls.Certificate, listener net.Listener) net.Listener {
	tlsConfig := security.GetManagerTlsConfig(cert)

	return tls.NewListener(listener, tlsConfig)
}

func (a *App) Run(ctx context.Context) {
	go a.Manager.SendTasksForever(ctx)
	go a.Manager.UpdateTasksForever(ctx)
	go a.Api.Start(ctx, a.ControlConnectionTlsListener)

	waitForWorkersForever(ctx, a.WorkerConnectionTlsListener, a.Manager, a.Logger)
}

// Blocks until the client connects
func connectAndCreateHttpClient(listener net.Listener) (*http.Client, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	session, err := yamux.Client(conn, nil)
	if err != nil {
		return nil, errors.Join(err, conn.Close())
	}

	httpConnection := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return session.Open()
			},
		},
	}
	return httpConnection, nil
}

// Returns the config read from disk / generated
func SetupConfig(
	configDirPath, configFileName string,
	newWorkerConnectionPort, newManagerServerApiPort int,
) (*configs.ManagerConfig, error) {

	// Loading the existing configs
	config, err := configs.GetConfigFromFile[configs.ManagerConfig](configDirPath, configFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Info("The config file does not exist yet, creating a new configuration with ports provided", "error", err)
		} else {
			slog.Warn("The config file is corrupted or cannot be read, creating a new configuration with ports provided", "error", err)
		}
	}

	if err != nil {
		// We don't have previous configurations, have to generate from scratch
		cert, token, certHash := security.GenerateManagerIdentity()
		config = configs.NewManagerConfig(newWorkerConnectionPort, newManagerServerApiPort, cert, token, certHash)
	} else {
		// Update the two fields if the values provided by flags is not default value 0
		if newWorkerConnectionPort != 0 {
			config.WorkerConnectionPort = newWorkerConnectionPort
		}
		if newManagerServerApiPort != 0 {
			config.ServerApiPort = newManagerServerApiPort
		}
	}

	err = configs.StoreConfigToFile(configDirPath, configFileName, config)
	if err != nil {
		return nil, fmt.Errorf("failed to store the manager configs to file: %w", err)
	}

	return config, nil
}

func waitForWorkersForever(ctx context.Context, listener net.Listener, newManager ManagerService, logger logger.ManagerLogger) {
	go func() {
		// Blocks until the context is sending cancel signal
		<-ctx.Done()
		// Causing all the following connections to fail
		listener.Close()
	}()

	for {
		httpClient, err := connectAndCreateHttpClient(listener)
		if err != nil {
			// Returning if the context is closed
			if ctx.Err() != nil {
				return
			}
			logger.Error("Failed to connect to worker and create an HTTP client", "error", err)
			continue
		}

		// Making requests
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://worker/info", nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("Failed to get worker info", "error", err)
			continue
		}

		workerInfo := &worker.Worker{}
		err = json.NewDecoder(resp.Body).Decode(workerInfo)
		resp.Body.Close()
		if err != nil {
			logger.Error("Failed to decode worker info", "error", err)
			continue
		}

		newManager.AddWorkerAndClient(workerInfo, httpClient)

		newManager.UpdateWorkerNodes()
	}
}
