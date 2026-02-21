package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/configs"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
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

type argsParsedResult struct {
	managerAddress string
	tlsToken string	
	workerName string
}

func main() {
	parsedArgs, err := parseWorkerArgs(os.Args)
	workerConfigs := setupWorkerConfig(parsedArgs, err)

	var address string = workerConfigs.ManagerAddress
	var tlsToken string = workerConfigs.TlsToken
	var workerName string = workerConfigs.WorkerName
	var workerUuid uuid.UUID = workerConfigs.Uuid

	newWorker := worker.NewWorker(workerUuid, workerName, *queue.New(), "persistent")
	tlsConfig := security.GetWorkerTlsConfig(tlsToken)

	go newWorker.RunTasksForever()
	go newWorker.UpdateTaskStatsForever()

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

		// This is a blocking call
		api.Start()
	}
}

func parseWorkerArgs(args []string) (*argsParsedResult, error) {
	if (len(args) < 4) {
		return &argsParsedResult{}, fmt.Errorf("Not enough number of args")
	} 

	return &argsParsedResult{
		managerAddress: args[0],
		tlsToken: args[1],
		workerName: args[2],
	}, nil
}

func setupWorkerConfig(parsedResult *argsParsedResult, parseErr error) *configs.WorkerConfig {
	workerConfig, err := configs.GetConfigFromFile[configs.WorkerConfig](WorkerConfigDirPath, WorkerConfigFileName)
	hasExistingConfig := (err == nil)

	// If parse failed 
	if (parseErr != nil)	{
		if (!hasExistingConfig)	{
			log.Fatal(
				"Not enough arguments and cannot find configs in disk, program must be called with " +  
				"go run main.go <manager_ip_address>:<port_number> <tls_token> <worker_name>",
				);
		}
		return workerConfig
	}

	// Parse successful, valid parsedResult 

	// No existing(previous) configs 
	// Need to create new ones
	if (!hasExistingConfig) {
		workerConfig = configs.NewWorkerConfig(uuid.New(), parsedResult.workerName, parsedResult.managerAddress, parsedResult.tlsToken)
	// Need to update old ones
	} else {
		workerConfig.ManagerAddress = parsedResult.managerAddress	
		workerConfig.TlsToken = parsedResult.tlsToken	
		workerConfig.WorkerName = parsedResult.workerName
	}
	
	err = configs.StoreConfigToFile(WorkerConfigDirPath, WorkerConfigFileName, workerConfig)
	if (err != nil) {
		log.Fatalf("Error storing config to the disk: %v", err)
	}

	return workerConfig
}