package worker

import (
	"crypto/tls"
	"fmt"
	"log"
	"time"

	"github.com/e-hua/netbula/internal/configs"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

const (
	WorkerRetryTime = time.Second * 5

	WorkerConfigDirPath  = "."
	WorkerConfigFileName = "worker_config.json"
)

func connectAndCreateSession(address string, tlsConfig *tls.Config) (*yamux.Session, error) {
	connection, err := tls.Dial("tcp", address, tlsConfig)
	if err != nil {
		return nil, err
	}

	session, err := yamux.Server(connection, nil)
	if err != nil {
		connection.Close()
		return nil, err
	}

	return session, nil
}

type argsParsedResult struct {
	managerAddress string
	tlsToken       string
	workerName     string
}

func Run(managerAddr string, token string, name string) {
	parsedArgs, err := parseWorkerArgs([]string{managerAddr, token, name})
	workerConfigs := setupWorkerConfig(parsedArgs, err)

	var address string = workerConfigs.ManagerAddress
	var tlsToken string = workerConfigs.TlsToken
	var workerName string = workerConfigs.WorkerName
	var workerUuid uuid.UUID = workerConfigs.Uuid

	newWorker := NewWorker(workerUuid, workerName, *queue.New(), "persistent")
	tlsConfig := security.GetWorkerTlsConfig(tlsToken)

	go newWorker.RunTasksForever()
	go newWorker.UpdateTaskStatsForever()

	for {
		session, err := connectAndCreateSession(address, tlsConfig)
		if err != nil {
			log.Printf("Connection failed: %v", err)
			time.Sleep(WorkerRetryTime)
			continue
		}

		api := Api{
			Session: session,
			Worker:  newWorker,
		}

		// This is a blocking call
		api.Start()
	}
}

func parseWorkerArgs(args []string) (*argsParsedResult, error) {
	if len(args) < 3 {
		return &argsParsedResult{}, fmt.Errorf("Not enough number of args")
	}

	return &argsParsedResult{
		managerAddress: args[0],
		tlsToken:       args[1],
		workerName:     args[2],
	}, nil
}

func setupWorkerConfig(parsedResult *argsParsedResult, parseErr error) *configs.WorkerConfig {
	workerConfig, err := configs.GetConfigFromFile[configs.WorkerConfig](WorkerConfigDirPath, WorkerConfigFileName)
	hasExistingConfig := (err == nil)

	// If parse failed
	if parseErr != nil {
		if !hasExistingConfig {
			log.Fatal(
				"Not enough arguments and cannot find configs in disk, program must be called with " +
					"go run main.go <manager_ip_address>:<port_number> <tls_token> <worker_name>",
			)
		}
		return workerConfig
	}

	// Parse successful, valid parsedResult

	// No existing(previous) configs
	// Need to create new ones
	if !hasExistingConfig {
		if parsedResult.managerAddress != "" || parsedResult.tlsToken != "" || parsedResult.workerName != "" {
			log.Fatalf("Not existing config, and missing flags: %v", err)
		}
		workerConfig = configs.NewWorkerConfig(uuid.New(), parsedResult.workerName, parsedResult.managerAddress, parsedResult.tlsToken)
		// Need to update old ones
	} else {
		if parsedResult.managerAddress != "" {
			workerConfig.ManagerAddress = parsedResult.managerAddress
		}
		if parsedResult.tlsToken != "" {
			workerConfig.TlsToken = parsedResult.tlsToken
		}
		if parsedResult.workerName != "" {
			workerConfig.WorkerName = parsedResult.workerName
		}
	}

	err = configs.StoreConfigToFile(WorkerConfigDirPath, WorkerConfigFileName, workerConfig)
	if err != nil {
		log.Fatalf("Error storing config to the disk: %v", err)
	}

	return workerConfig
}
