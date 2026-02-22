package manager

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/configs"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/hashicorp/yamux"
)

const (
	ManagerConfigDirPath = "."
	ManagerConfigFileName = "manager_config.json"
)

func createTlsListener(cert tls.Certificate, token string, port string) net.Listener {
	tlsConfig := security.GetManagerTlsConfig(cert)

	listener, err := tls.Listen("tcp", port, tlsConfig)
	if (err != nil) {
		log.Fatalf("Error listening to port %s: %v\n", port, err)	
	}

	fmt.Printf("Connection token: %v (Enter this when registering workers)\n", token)
	return listener
}

// Blocks until the client connects 
func connectAndCreateHttpClient(listener net.Listener) (*http.Client, error) {
	conn, err := listener.Accept()
	if (err != nil) {
		return nil, err
	}

	session, err := yamux.Client(conn, nil)
	if (err != nil) {
		conn.Close()
		return nil, err
	}

	httpConnection := &http.Client {
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return session.Open()
			},
		},
	}
	return httpConnection, nil
}

// Returns an array of ints with length of 2
// First one is the port worker is going to connect to 
// Second one is the port the user is going to call the manager server 
func parseManagerArgs(args []string) ([2]int, error) {
	if (len(args) < 2) {
		return [2]int{}, fmt.Errorf("Not enough number of args")
	} 
	
	workerConnectionPort, err := strconv.Atoi(os.Args[0])
	if (err != nil) {
		return [2]int{}, fmt.Errorf("Invalid port number for the connection with workers: %s", os.Args[0])
	}

	managerServerApiPort, err := strconv.Atoi(os.Args[1])
	if (err != nil) {
		return [2]int{}, fmt.Errorf("Invalid port number for manager API: %s", os.Args[1])
	}

	return [2]int{workerConnectionPort, managerServerApiPort}, nil
}

// Returns the config read from disk / generated 
func setupConfig(ports [2]int, parseErr error) *configs.ManagerConfig {
	config, err := configs.GetConfigFromFile[configs.ManagerConfig](ManagerConfigDirPath, ManagerConfigFileName)
	hasExistingConfig := (err == nil)

	if (parseErr != nil) {
		if (!hasExistingConfig) {
			log.Fatalln("Critical: No CLI arguments and no config file found.")
		}
		return config
	}

	newWorkerConnectionPort := ports[0]
	newManagerServerApiPort := ports[1]

	if (!hasExistingConfig) {
		cert, token := security.GenerateManagerIdentity()
		config = configs.NewManagerConfig(newWorkerConnectionPort, newManagerServerApiPort, cert, token)
	} else {
		if (newWorkerConnectionPort != 0) {
			config.WorkerConnectionPort = newWorkerConnectionPort
		}
		if (newManagerServerApiPort != 0) {
			config.ServerApiPort = newManagerServerApiPort
		}
	}

	configs.StoreConfigToFile(ManagerConfigDirPath, ManagerConfigFileName, config)
	return config
}

func waitForWorkersForever(listener net.Listener, newManager *Manager) {
	for {
		httpClient, err := connectAndCreateHttpClient(listener)
		if (err != nil) {
			log.Printf("Error creating http client: %v", err)
			continue;
		}

		resp, err := httpClient.Get("http://worker/info")
		if (err != nil) {
			continue
		}

		workerInfo := &worker.Worker{}
		json.NewDecoder(resp.Body).Decode(workerInfo)
	
		fmt.Printf("Connected worker name: %v\n", workerInfo.Name)
		newManager.WorkerCluster.AddClient(workerInfo.Uuid, httpClient)

		newManager.UpdateWorkerNodes()
	}	
}

func Run(ports [2]int) {
	cfg := setupConfig(ports, nil)

	formattedPort := fmt.Sprintf(":%d", cfg.WorkerConnectionPort)
	cert := tls.Certificate{
			Certificate: cfg.TlsCertificateInBytes,
			PrivateKey: ed25519.PrivateKey(cfg.TlsPrivateKey),
	}

	listener := createTlsListener(
		cert, cfg.TlsToken, formattedPort,
	)

	newManager := New(&scheduler.Epvm{}, "persistent");
	managerApi := Api{Manager: newManager, Port: cfg.ServerApiPort, TlsToken: cfg.TlsToken}

	go newManager.SendTasksForever()
	go newManager.UpdateTasksForever()
	go managerApi.Start(cert)

	waitForWorkersForever(listener, newManager)
}