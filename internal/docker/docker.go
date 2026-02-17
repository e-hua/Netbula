package docker

import (
	"context"
	"io"
	"log"
	"math"
	"os"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// Configurations for a container/task
type Config struct {
	Name string

	AttachStdin bool
	AttachStdout bool
	AttachStderr bool

	ExposedPorts network.PortSet

	Cmd []string
	// Image url: "docker.io/library/alpine"
	Image string

	Cpu float64
	Memory int
	Disk int
	Env []string
	// always, unless-stopped, or on-failure
	RestartPolicy string
}

// All we need to interact with Docker API and run a container 
type Docker struct {
	Client *client.Client
	Config Config
}

type DockerResult struct {
	Error error
	Action string
	ContainerId string
	Result string
}

type DockerInspectResponse struct {
	Error error
	Container *container.InspectResponse
}


func (docker *Docker) Run() DockerResult {
	emptyContext := context.Background()
	reader, err := docker.Client.ImagePull(
		emptyContext, 
		docker.Config.Image,
		client.ImagePullOptions{},
	)

	if (err != nil) {
		// Log prints to error by default 
		log.Println("Failed to pull image", err)
		return DockerResult{Error: err}
	}

	// Pipe the reader to stdout 
	io.Copy(os.Stdout, reader)

	restartPolicy := container.RestartPolicy{
		Name: container.RestartPolicyMode(docker.Config.RestartPolicy),
	}

	resources := container.Resources{
		Memory: int64(docker.Config.Memory),
		// 1 nanoCPU = 10 ^ -9 of a CPU core 
		NanoCPUs: int64(docker.Config.Cpu * math.Pow(10, 9)),
	}

	containerConfig := container.Config{
		Image: docker.Config.Image,
		Tty: false,
		Env: docker.Config.Env,
		ExposedPorts: docker.Config.ExposedPorts,
	}

	hostConfig := container.HostConfig{
		RestartPolicy: restartPolicy,
		Resources: resources,
		PublishAllPorts: true,
	}

	response, err := docker.Client.ContainerCreate(
		emptyContext, 
		client.ContainerCreateOptions{
			Config: &containerConfig, 
			HostConfig: &hostConfig, 
			Name: docker.Config.Name,
		},
	)

	_, err = docker.Client.ContainerStart(
		emptyContext, 
		response.ID, 
		client.ContainerStartOptions{},
	)

	if (err != nil) {
		log.Printf("Error starting container %s: %v\n", response.ID, err)	
		return DockerResult{Error: err}
	}


	out, err := docker.Client.ContainerLogs(
		emptyContext,
		response.ID,
		client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true},
	)

	if (err != nil) {
		log.Printf("Error egtting logs for container %s: %v\n", response.ID, err)	
		return DockerResult{Error: err}
	}


	stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	return DockerResult{ContainerId: response.ID, Action: "start", Result: "success"}
}

func (docker *Docker) Stop(id string) DockerResult {
	log.Printf("Attempting to stop container %v", id)
	emptyContext := context.Background()
	_, err := docker.Client.ContainerStop(emptyContext, id, client.ContainerStopOptions{});

	if (err != nil) {
		log.Printf("Error stopping the container: %s: %v\n", id, err)
		return DockerResult{Error: err}
	}

	// Stop and remove the container
	// You won't be able to see the container 
	// using `docker ps` or `docker ps -a` after this 

	_, err = docker.Client.ContainerRemove(
		emptyContext,
		id,
		client.ContainerRemoveOptions{
			RemoveVolumes: true,
			RemoveLinks: false,
			Force: false,
		},
	)

	if (err != nil) {
		log.Printf("Error removing container %s: %v\n", id, err)
		return DockerResult{Error: err }
	}

	return DockerResult{
		Action: "stop", 
		Result: "success", 
		Error: nil, 
		ContainerId: id,
	}
}

func NewDocker(config Config) Docker {
	apiClient, _ := client.New(client.FromEnv)

	return Docker {
		Config: config,	
		Client: apiClient,
	}
}

func (docker *Docker) Inspect(containerId string) DockerInspectResponse {
	emptyContext := context.Background()
	resp, err := docker.Client.ContainerInspect(emptyContext, containerId, client.ContainerInspectOptions{})

	if (err != nil) {
		log.Printf("Error inspecting container: %s\n", err)
		return DockerInspectResponse{Error: err}
	}

	return DockerInspectResponse{Container: &resp.Container}
}
