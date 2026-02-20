package main

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/configs"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/golang-collections/collections/queue"
	"github.com/hashicorp/yamux"
)

const (
	WorkerRetryTime = time.Second * 5;

	WorkerConfigDirPath = "."
	WorkerConfigFileName = "worker_config.json"
)

func connectAndCreateSession(address string, tlsConfig *tls.Config) (*yamux.Session, error) {
	connection, err := tls.Dial("tcp", address, tlsConfig)
	if (err != nil) {
		return nil, err
	}

	session, err := yamux.Server(connection, nil)
	if (err != nil) {
		connection.Close()
		return nil, err
	}

	return session, nil
}

func main() {
	var address string 
	var tlsToken string 
	var workerName string 

	if (len(os.Args) < 4) {
		workerConfig, err := configs.GetConfigFromFile[configs.WorkerConfig](WorkerConfigDirPath, WorkerConfigFileName)
		if (err != nil) {
			log.Fatal(
				"Not enough arguments and cannot find configs in disk, program must be called with " +  
				"go run main.go <manager_ip_address>:<port_number> <tls_token> <worker_name>",
				);
		}

		address = workerConfig.ManagerAddress
		tlsToken = workerConfig.TlsToken
		workerName = workerConfig.WorkerName
	} else {
		address = os.Args[1];
		tlsToken = os.Args[2]			
		workerName = os.Args[3]

		workerConfig := configs.NewWorkerConfig(workerName, address, tlsToken)
		err := configs.StoreConfigToFile(WorkerConfigDirPath, WorkerConfigFileName, workerConfig)
		if (err != nil) {
			log.Fatalf("Error storing config to the disk: %v", err)
		}
	}

	newWorker := worker.NewWorker(workerName, *queue.New(), "persistent")

	tlsConfig := security.GetWorkerTlsConfig(tlsToken)

	for {
		session, err := connectAndCreateSession(address, tlsConfig)
		if (err != nil) {
			log.Printf("Connection failed: %v", err)
			time.Sleep(WorkerRetryTime)
			continue
		}

		api := worker.Api{
			Session: session,	
			Worker: newWorker,
		}

		go newWorker.RunTasksForever()
		go newWorker.UpdateTaskStatsForever()
		// This is a blocking call
		api.Start()
	}
}