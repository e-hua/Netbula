package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

func main() {
	if (len(os.Args) < 2) {
		fmt.Fprintln(
			os.Stderr, 
			"Not enough arguments, program must be called with" +  
			"go run main.go <port_number>");
		return;
	}

	formattedPort := fmt.Sprintf(":%v", os.Args[1])

	cert, token := security.GenerateManagerIdentity()
	tlsConfig := security.GetManagerTlsConfig(cert)

	listener, err := tls.Listen("tcp", formattedPort, tlsConfig)
	if (err != nil) {
		log.Fatalf("Error listening to port %s: %v\n", formattedPort, err)	
	}
	fmt.Printf("Connection token: %v (Enter this when registering workers)\n", token)

	httpClientMap := make(map[string]*http.Client)

	for {
		// Accept() blocks until the client connects 
		conn, err := listener.Accept()
		if (err != nil) {
			fmt.Fprintf(os.Stderr, "Error accepting connection: %s", err) 
			continue
		}

		session, err := yamux.Client(conn, nil)
		if (err != nil) {
			conn.Close()
			continue
		}

		httpConnection := &http.Client {
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return session.Open()
				},
			},
		}

		resp, err := httpConnection.Get("http://worker/info")
		if (err != nil) {
			conn.Close()
			continue
		}

		workerInfo := &worker.WorkerInfo{}
		json.NewDecoder(resp.Body).Decode(workerInfo)
	
		fmt.Printf("Connected worker name: %v\n", workerInfo.Name)
		httpClientMap[workerInfo.Name] = httpConnection


		taskEvent := task.TaskEvent {
			ID: uuid.New(),
			TargetState: task.Running,
			Task: task.Task { 
				ID: uuid.New(),
				Name: "test-chapter-5-1",
				State: task.Scheduled,
				Image: "strm/helloworld-http",
			},
		}
		taskEventData, err := json.Marshal(taskEvent)
		if (err != nil) {
			panic(err)
		}

		// Start the task 
		newTask := &task.Task{}
		resp, _  = httpClientMap[workerInfo.Name].Post("http://worker/tasks", "application/json", bytes.NewBuffer(taskEventData))	
		err = json.NewDecoder(resp.Body).Decode(newTask)
		if (err != nil) {
			panic(err)
		}
		fmt.Printf("New task: %v\n", newTask)

		time.Sleep(30 * time.Second)

		// Delete the task 
		path := "http://worker/tasks/" + newTask.ID.String()

		req, err := http.NewRequest(http.MethodDelete, path , nil)
		if (err != nil) {
			panic(err)
		}
		httpClientMap[workerInfo.Name].Do(req)
		fmt.Println("Task deleted")
	}	
}