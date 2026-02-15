package main

import (
	"crypto/tls"
	"log"
	"os"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/task"
	"github.com/golang-collections/collections/queue"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

const WorkerRetryTime = time.Second * 5;

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
	if (len(os.Args) < 4) {
		log.Fatal(
			"Not enough arguments, program must be called with" +  
			"go run main.go <manager_ip_address>:<port_number> <tls_token> <worker_name>",
			);
	}

	address := os.Args[1];
	tlsToken := os.Args[2]			
	workerName := os.Args[3]

	newWorker := worker.NewWorker(workerName, *queue.New(), make(map[uuid.UUID]*task.Task))

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
		// This is a blocking call
		api.Start()
	}
}