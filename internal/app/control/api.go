package control

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
)

func createCustomHttpsClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // Allowing self-signed certs
			},
		},
	}
}

// POST {manager_address}/tasks
// Make an API request to the manager to start a task
func StartTask(managerAddress string, token string, taskData []byte) {
	log.Printf("Task data sent: %v\n", string(taskData))	

	url := fmt.Sprintf("https://%s/tasks", managerAddress)	
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(taskData))
	if (err != nil)	{
		log.Panicf("Error creating http request: %v", err)
	}

	req.Header.Set("X-Netbula-Token", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := createCustomHttpsClient().Do(req)
	if (err != nil) {
		log.Panic(err)
	}

	if (resp.StatusCode != http.StatusCreated) { 
		log.Printf("Error sending request, response code: %v", resp.StatusCode)
	}

	defer resp.Body.Close()
	log.Println("Successfully sent task request to manager")
}

// GET {manager_address}/tasks
// Make an API request to the manager to get all the tasks
func GetTasks(managerAddress string, token string) []*task.Task {
	url := fmt.Sprintf("https://%s/tasks", managerAddress)	
	req, err := http.NewRequest("GET", url, nil)
	if (err != nil)	{
		log.Panicf("Error creating http request: %v", err)
	}

	req.Header.Set("X-Netbula-Token", token)

	resp, err := createCustomHttpsClient().Do(req)
	if (err != nil) {
		log.Panic(err)
	}
	body, err := io.ReadAll(resp.Body)
	if (err != nil) {
		log.Panic(err)
	}
	if (resp.StatusCode != http.StatusOK) { 
		log.Printf("Error sending request, response code: %v", resp.StatusCode)
		log.Printf("Body of response: %s\n", string(body))
	}
	defer resp.Body.Close()

	var tasks []*task.Task
	err = json.Unmarshal(body, &tasks)
	if (err != nil) {
		log.Panic(err)
	}

	return tasks
}

// DELETE {manager_address}/tasks/{taskId}
// Make an API request to the manager to stop the task
func StopTask(managerAddress string, token string, taskId string) {
	url := fmt.Sprintf("https://%s/tasks/%s", managerAddress, taskId)	
	req, err := http.NewRequest("DELETE", url, nil)
	if (err != nil)	{
		log.Panicf("Error creating http request: %v", err)
	}

	req.Header.Set("X-Netbula-Token", token)

	resp, err := createCustomHttpsClient().Do(req)
	if (err != nil) {
		log.Panic(err)
	}

	if (resp.StatusCode != http.StatusNoContent) { 
		log.Printf("Error sending request, response code: %v", resp.StatusCode)
		return 
	}

	log.Printf("Task %v has been stopped.", taskId)
}

// GET {manager_address}/nodes
// Make an API request to the manager to get all the nodes
func GetNodes(managerAddress string, token string) []*node.Node {
	url := fmt.Sprintf("https://%s/nodes", managerAddress)	
	req, err := http.NewRequest("GET", url, nil)
	if (err != nil)	{
		log.Panicf("Error creating http request: %v", err)
	}

	req.Header.Set("X-Netbula-Token", token)

	resp, err := createCustomHttpsClient().Do(req)
	if (err != nil) {
		log.Panic(err)
	}
	body, err := io.ReadAll(resp.Body)
	if (err != nil) {
		log.Panic(err)
	}
	if (resp.StatusCode != http.StatusOK) { 
		log.Printf("Error sending request, response code: %v", resp.StatusCode)
		log.Printf("Body of response: %s\n", string(body))
	}
	defer resp.Body.Close()

	var nodes []*node.Node
	err = json.Unmarshal(body, &nodes)
	if (err != nil) {
		log.Panic(err)
	}

	return nodes
}
