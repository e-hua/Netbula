package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/scheduler"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)


func createTlsListener(port string) net.Listener {
	cert, token := security.GenerateManagerIdentity()
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

func main() {
	if (len(os.Args) < 3) {
		fmt.Fprintln(
			os.Stderr, 
			"Not enough arguments, program must be called with" +  
			"go run main.go <port_number_for_worker_connection> <port_number_for_manager_api>");
		return;
	}

	managerApiPort, err := strconv.Atoi(os.Args[2])
	if (err != nil) {
		log.Fatalf("Invalid port number for manager API: %s", os.Args[2])
	}

	formattedPort := fmt.Sprintf(":%v", os.Args[1])
	listener := createTlsListener(formattedPort)

	newManager := manager.New(make([]uuid.UUID, 0), &scheduler.Epvm{}, "memory");
	managerApi := manager.Api{Manager: newManager, Port: managerApiPort}

	go newManager.SendTasksForever()
	go newManager.UpdateTasksForever()
	go managerApi.Start()

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
		newManager.AddWorkerAndClient(*workerInfo, httpClient)

		newManager.UpdateWorkerNodes()
	}	
}
