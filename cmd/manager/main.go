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
	"time"

	"github.com/e-hua/netbula/internal/app/manager"
	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/task"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

const UpdateTasksPeriod = 15 * time.Second

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
	if (len(os.Args) < 2) {
		fmt.Fprintln(
			os.Stderr, 
			"Not enough arguments, program must be called with" +  
			"go run main.go <port_number>");
		return;
	}

	formattedPort := fmt.Sprintf(":%v", os.Args[1])
	listener := createTlsListener(formattedPort)

	newManager := manager.New(make([]uuid.UUID, 0));

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

		workerStats := getWorkerStatus(httpClient)
		printWorkerStatus(workerStats)

		// Add and send demo taskevents
		for i := range 3 {
			t := task.Task{
				ID: uuid.New(),
				Name: fmt.Sprintf("test-container-%d", i),
				State: task.Scheduled,
				Image: "strm/helloworld-http",
			}

			te := task.TaskEvent{
				ID: uuid.New(),
				TargetState: task.Running,
				Task: t,
			}

			newManager.AddTaskEvent(te)
			newManager.SendWork()
		}

		go func() {
			for {
				fmt.Printf("[Manager] Updating tasks from %d workers\n", len(newManager.Workers))
				newManager.UpdateTasks()
				time.Sleep(UpdateTasksPeriod)
			}
		}()

		time.Sleep(90 * time.Second)

		for taskId := range(newManager.TaskMap) {
			path := "http://worker/tasks/" + taskId.String()

			req, err := http.NewRequest(http.MethodDelete, path , nil)
			if (err != nil) {
				panic(err)
			}

			httpClient.Do(req)
		}

		for { 
			for _, t := range newManager.TaskMap { 
				fmt.Printf("[Manager] Task: id: %s, state: %d\n", t.ID, t.State)
			}
			time.Sleep(15 * time.Second)
		}
		/*
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

		workerStats = getWorkerStatus(httpClientMap[workerInfo.Name])
		printWorkerStatus(workerStats)

		// Delete the task 
		path := "http://worker/tasks/" + newTask.ID.String()

		req, err := http.NewRequest(http.MethodDelete, path , nil)
		if (err != nil) {
			panic(err)
		}
		httpClientMap[workerInfo.Name].Do(req)
		fmt.Println("Task deleted")
		*/
	}	
}

func getWorkerStatus(client *http.Client) types.Stats {
	workerStats := &types.Stats{}
	resp, _ := client.Get("http://worker/stats")

	err := json.NewDecoder(resp.Body).Decode(workerStats)
	if (err != nil) {
		panic(err)
	}

	return *workerStats
}

func printWorkerStatus(workerStats types.Stats) {
	fmt.Printf("Worker machine status: \n")
	fmt.Printf(
		"[%v] RAM: %.2f GB, RAM usage: %.2f%%, Disk: %.2f GB, Disk usage: %.2f%%, CPU: %d cores, Average CPU usage: %.2f%%, CPU load index: %v\n", 
		time.Now().String(), 
		float64(workerStats.MemTotalInBytes) / float64(types.GigabyteInBytes), 
		workerStats.MemUsedPercent,
		float64(workerStats.DiskTotalInBytes) / float64(types.GigabyteInBytes), 
		workerStats.DiskUsedPercent,
		workerStats.CpuCount,
		workerStats.CpuPercents[0],
		workerStats.LoadAvg / float64(workerStats.CpuCount),
	)
}