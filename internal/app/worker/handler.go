package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/e-hua/netbula/internal/networks/types"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// The struct providing request handlers to the worker API
type Handlers struct {
	Worker *Worker
}

// GET <worker-manager-connection>/stats
// Sending the specs and loads of the machine worker program is running on
func (h *Handlers) GetWorkerStatsHandler(responseWriter http.ResponseWriter, request *http.Request) {
	stats, err := types.GetStats()

	if err != nil {
		routers.RespondError(responseWriter, http.StatusInternalServerError, err.Error())
		return
	}

	routers.RespondJSON(responseWriter, http.StatusOK, stats)
}

// GET <worker-manager-connection>/info
// Sending the info of the worker(mainly the name and uuid)
func (h *Handlers) GetWorkerInfoHandler(responseWriter http.ResponseWriter, request *http.Request) {
	routers.RespondJSON(responseWriter, http.StatusOK, h.Worker)
}

// POST <worker-manager-connection>/tasks
// Accepts a `TaskEvent` struct from the request body
// Add the assigned task to the pending queue of tasks `a.Worker.AddTask()`
func (h *Handlers) StartTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	newTaskEvent := task.TaskEvent{}
	err := decoder.Decode(&newTaskEvent)
	if err != nil {
		msg := fmt.Sprintf("Error unmarshalling body: %v\n", err)
		log.Printf("%s", msg)
		routers.RespondError(responseWriter, http.StatusBadRequest, msg)
		return
	}

	h.Worker.AddTask(newTaskEvent.Task)
	log.Printf("Added task %v\n", newTaskEvent.Task.ID)
	routers.RespondJSON(responseWriter, http.StatusCreated, newTaskEvent.Task)
}

// GET <worker-manager-connection>/tasks
// List all the tasks in this worker
// And send this array of tasks
func (h *Handlers) GetTasksHandler(responseWriter http.ResponseWriter, request *http.Request) {
	tasks, err := h.Worker.taskDb.List()
	if err != nil {
		log.Printf("Error getting list of tasks: %v\n", err)
		routers.RespondError(responseWriter, http.StatusOK, err.Error())
		return
	}
	routers.RespondJSON(responseWriter, http.StatusOK, tasks)
}

// DELETE <worker-manager-connection>/tasks/{taskId}
// Creates a `Task` struct from the `taskId` in the request params
// Mark its desired state as "complete"
// And put it in the "pending queue" of desired task states
func (h *Handlers) StopTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	taskId := chi.URLParam(request, "taskId")

	if taskId == "" {
		routers.RespondError(responseWriter, http.StatusBadRequest, "No taskId passed in the request.\n")
	}

	parsedId, _ := uuid.Parse(taskId)
	taskToStop, err := h.Worker.taskDb.Get(parsedId.String())

	if taskToStop == nil {
		if err != nil {
			message := fmt.Sprintf("Error getting task from storage: %v\n", err.Error())
			routers.RespondError(responseWriter, http.StatusNotFound, message)
		} else {
			message := fmt.Sprintf("No task with ID %v found\n", parsedId)
			routers.RespondError(responseWriter, http.StatusNotFound, message)
		}
		return
	}

	fmt.Printf("Parsed Id: %v\n", parsedId)
	// Pass by value
	taskCopy := *taskToStop
	taskCopy.State = task.Completed
	h.Worker.AddTask(taskCopy)

	responseWriter.WriteHeader(http.StatusNoContent)
}
