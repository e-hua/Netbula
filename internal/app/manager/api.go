package manager

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// The Api receiving requests from localhost:<Port>
type Api struct {
	Port int
	Manager	*Manager
	Router *chi.Mux
}

func (a *Api) initRouter() {
	a.Router = chi.NewRouter()
	a.Router.Use(middleware.Logger)

	a.Router.Route("/tasks", func (r chi.Router) {
		r.Post("/", a.StartTaskHandler)
		r.Get("/", a.GetTasksHandler)
		r.Route("/{taskId}", func(r chi.Router) {
			r.Delete("/", a.StopTaskHandler)
		})		
	})
}

func (a *Api) Start() {
	a.initRouter()

	http.ListenAndServe(fmt.Sprintf("localhost:%d", a.Port), a.Router)
}

// POST localhost:<Port>/tasks

// Only responsible for putting the task in the queue of pending task events 
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

	a.Manager.AddTaskEvent(newTaskEvent)
	log.Printf("Added task %v\n", newTaskEvent.Task.ID)
	routers.RespondJSON(responseWriter, http.StatusCreated, newTaskEvent.Task)
}

// GET localhost:<Port>/tasks
func (a *Api) GetTasksHandler(responseWriter http.ResponseWriter, request *http.Request) {
	tasks, err := a.Manager.State.TaskDb.List()
	if (err != nil) {
		routers.RespondError(responseWriter, http.StatusInternalServerError, err.Error())
	}

	if (tasks == nil) {
		tasks = []*task.Task{}
	}

	routers.RespondJSON(responseWriter, http.StatusOK, tasks)
}

// DELETE localhost:<Port>/tasks/{taskId}

// Only responsible for putting the task in the queue of pending task events 
func (a *Api) StopTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	taskId := chi.URLParam(request, "taskId")
	if (taskId == "") {
		routers.RespondError(responseWriter, 400, "No taskId passed in the request.\n")
	}

	parsedId, _	:= uuid.Parse(taskId)
	fmt.Printf("Parsed Id: %v\n", parsedId)

	taskToStop, err := a.Manager.State.TaskDb.Get(parsedId.String())
	if (err != nil) {
		message := fmt.Sprintf("No task with ID %v found in storage\n", parsedId)
		routers.RespondError(responseWriter, 404, message)
		return
	}

	// Pass by value
	taskCopy := *taskToStop
	taskCopy.State = task.Completed

	stopTaskEvent := task.TaskEvent {
		ID: uuid.New(),
		TargetState: task.Completed,
		Timestamp: time.Now(),
	}

	stopTaskEvent.Task = taskCopy
	a.Manager.AddTaskEvent(stopTaskEvent)
	
	responseWriter.WriteHeader(204)
}