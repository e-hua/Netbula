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
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

type Api struct {
	Worker *Worker
	Session *yamux.Session
	Router *chi.Mux
}

func (a *Api) initRouter() {
	a.Router = chi.NewRouter()
	a.Router.Use(middleware.Logger)

	a.Router.Route("/tasks", func (router chi.Router) {
		router.Post("/", a.StartTaskHandler) 
		router.Get("/", a.GetTasksHandler)
		router.Route("/{taskId}", func(router chi.Router) { 
			router.Delete("/", a.StopTaskHandler) 
		})
	})

	a.Router.Route("/info", func (router chi.Router) {
		router.Get("/", a.GetWorkerInfoHandler)
	})

	a.Router.Route("/stats", func (router chi.Router) {
		router.Get("/", a.GetWorkerStatsHandler)
	})
}

func (a *Api) Start() {
	a.initRouter()
	http.Serve(a.Session, a.Router)
}


// GET <worker-manager-connection>/stats
// Sending the specs and loads of the machine worker program is running on 
func (a *Api) GetWorkerStatsHandler(responseWriter http.ResponseWriter, request *http.Request) {
	stats, err := types.GetStats()

	if (err != nil) {
		routers.RespondError(responseWriter, http.StatusInternalServerError, err.Error())
		return;
	}

	routers.RespondJSON(responseWriter, http.StatusOK, stats)
}

// GET <worker-manager-connection>/info
// Sending the info of the worker(mainly the name and uuid)
func (a *Api) GetWorkerInfoHandler(responseWriter http.ResponseWriter, request *http.Request) {
	routers.RespondJSON(responseWriter, http.StatusOK, a.Worker)
}

// POST <worker-manager-connection>/tasks
// Accepts a `TaskEvent` struct from the request body 
// Add the assigned task to the pending queue of tasks `a.Worker.AddTask()`
func (a *Api) StartTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()		

	newTaskEvent := task.TaskEvent{}
	err := decoder.Decode(&newTaskEvent)
	if (err != nil) {
		msg := fmt.Sprintf("Error unmarshalling body: %v\n", err)
		log.Printf("%s", msg)
		routers.RespondError(responseWriter, http.StatusBadRequest, msg)
		return
	}

	a.Worker.AddTask(newTaskEvent.Task)
	log.Printf("Added task %v\n", newTaskEvent.Task.ID)
	routers.RespondJSON(responseWriter, http.StatusCreated, newTaskEvent.Task)
}

// GET <worker-manager-connection>/tasks
// List all the tasks in this worker 
// And send this array of tasks
func (a *Api) GetTasksHandler(responseWriter http.ResponseWriter, request *http.Request) {
	tasks, err := a.Worker.taskDb.List()
	if (err != nil) {
		log.Printf("Error getting list of tasks: %v\n", err)
		routers.RespondError(responseWriter, http.StatusOK, err.Error())
		return;
	}
	routers.RespondJSON(responseWriter, http.StatusOK, tasks)
}

// DELETE <worker-manager-connection>/tasks/{taskId}
// Creates a `Task` struct from the `taskId` in the request params
// Mark its desired state as "complete"
// And put it in the "pending queue" of desired task states
func (a *Api) StopTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	taskId := chi.URLParam(request, "taskId")

	if (taskId == "") {
		routers.RespondError(responseWriter, http.StatusBadRequest, "No taskId passed in the request.\n")
	}

	parsedId, _	:= uuid.Parse(taskId)
	taskToStop, err := a.Worker.taskDb.Get(parsedId.String())

	if (taskToStop == nil) {
		if (err != nil) {
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
	a.Worker.AddTask(taskCopy)
	
	responseWriter.WriteHeader(http.StatusNoContent)
}