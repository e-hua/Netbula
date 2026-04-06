package manager

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	ManagerConfigDirPath  = "."
	ManagerConfigFileName = "manager_config.json"
)

func createTlsListener(cert tls.Certificate, token string, port string) (net.Listener, error) {
	tlsConfig := security.GetManagerTlsConfig(cert)

	listener, err := tls.Listen("tcp", port, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	fmt.Printf("Connection token: %v (Enter this when registering workers)\n", token)
	return listener, nil
}

// Blocks until the client connects
func connectAndCreateHttpClient(listener net.Listener) (*http.Client, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	session, err := yamux.Client(conn, nil)
	if err != nil {
		conn.Close()
		return nil, err
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
func setupConfig(ports [2]int, logger logger.ManagerLogger) (*configs.ManagerConfig, error) {
	config, err := configs.GetConfigFromFile[configs.ManagerConfig](ManagerConfigDirPath, ManagerConfigFileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("The config file does not exist yet, creating a new configuration with ports provided", "error", err)
		} else {
			logger.Warn("The config file is corrupted or cannot be read, creating a new configuration with ports provided", "error", err)
		}
	}

	newWorkerConnectionPort := ports[0]
	newManagerServerApiPort := ports[1]

	// We don't have previous configurations
	if err != nil {
		cert, token := security.GenerateManagerIdentity()
		config = configs.NewManagerConfig(newWorkerConnectionPort, newManagerServerApiPort, cert, token)
	} else {
		// Update the two fields if the values provided by flags is not default value 0
		if newWorkerConnectionPort != 0 {
			config.WorkerConnectionPort = newWorkerConnectionPort
		}
		if newManagerServerApiPort != 0 {
			config.ServerApiPort = newManagerServerApiPort
		}
	}

	err = configs.StoreConfigToFile(ManagerConfigDirPath, ManagerConfigFileName, config)
	if err != nil {
		return nil, fmt.Errorf("failed to store the manager configs to file: %w", err)
	}

	return config, nil
}

func waitForWorkersForever(listener net.Listener, newManager *Manager) {
	for {
		httpClient, err := connectAndCreateHttpClient(listener)
		if err != nil {
			newManager.ManagerLogger.Error("Failed to connect to worker and create an HTTP client", "error", err)
			continue
		}

		resp, err := httpClient.Get("http://worker/info")
		if err != nil {
			newManager.ManagerLogger.Error("Failed to get worker info", "error", err)
			continue
		}

		workerInfo := &worker.Worker{}
		json.NewDecoder(resp.Body).Decode(workerInfo)

		newManager.AddWorkerAndClient(workerInfo, httpClient)

		newManager.UpdateWorkerNodes()
	}
}

func Run(ports [2]int, verbose bool) {
	managerLogger := logger.NewManagerLogger(verbose, os.Stderr)

	cfg, err := setupConfig(ports, *managerLogger)
	if err != nil {
		// Shutting down
		managerLogger.TerminateApplication("Failed to setup configurations of manager", err)
	}

	formattedPort := fmt.Sprintf(":%d", cfg.WorkerConnectionPort)
	cert := tls.Certificate{
		Certificate: cfg.TlsCertificateInBytes,
		PrivateKey:  ed25519.PrivateKey(cfg.TlsPrivateKey),
	}

	listener, err := createTlsListener(cert, cfg.TlsToken, formattedPort)
	if err != nil {
		// Shutting down
		managerLogger.TerminateApplication("Failed to initialize TCP listener with TLS encryption", err)
	}

	newManager, err := New(&scheduler.Epvm{}, *managerLogger)
	if err != nil {
		// Shutting down
		managerLogger.TerminateApplication("Failed to initialize manager", err)
	}

	managerApi := Api{
		Manager:  newManager,
		Port:     cfg.ServerApiPort,
		TlsToken: cfg.TlsToken,
		Logger:   *logger.NewManagerLoggerWithSubsystem(*managerLogger, "api"),
	}

	go newManager.SendTasksForever()
	go newManager.UpdateTasksForever()
	go managerApi.Start(cert)

	waitForWorkersForever(listener, newManager)
}
