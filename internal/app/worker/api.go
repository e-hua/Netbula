package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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
}

func (a *Api) Start() {
	a.initRouter()
	http.Serve(a.Session, a.Router)
}

type WorkerInfo struct {
	Name string
}

func (a *Api) GetWorkerInfoHandler(responseWriter http.ResponseWriter, request *http.Request) {
	routers.RespondJSON(responseWriter, 200, WorkerInfo{Name: a.Worker.Name})
}

func (a *Api) StartTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()		

	newTaskEvent := task.TaskEvent{}
	err := decoder.Decode(&newTaskEvent)
	if (err != nil) {
		msg := fmt.Sprintf("Error unmarshalling body: %v\n", err)
		log.Printf("%s", msg)
		routers.RespondError(responseWriter, 400, msg)
		return
	}

	a.Worker.AddTask(newTaskEvent.Task)
	log.Printf("Added task %v\n", newTaskEvent.Task.ID)
	routers.RespondJSON(responseWriter, 201, newTaskEvent.Task)
}

func (a *Api) GetTasksHandler(responseWriter http.ResponseWriter, request *http.Request) {
	routers.RespondJSON(responseWriter, 200, a.Worker.GetTasks())
}

func (a *Api) StopTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	taskId := chi.URLParam(request, "taskId")

	if (taskId == "") {
		routers.RespondError(responseWriter, 400, "No taskId passed in the request.\n")
	}

	parsedId, _	:= uuid.Parse(taskId)
	_, ok := a.Worker.taskMap[parsedId]
	if (!ok) {
		message := fmt.Sprintf("No task with ID %v found\n", parsedId)
		routers.RespondError(responseWriter, 404, message)
	}

	fmt.Printf("Parsed Id: %v\n", parsedId)
	taskToStop := a.Worker.taskMap[parsedId]
	// Pass by value 
	taskCopy := *taskToStop
	taskCopy.State = task.Completed
	a.Worker.AddTask(taskCopy)
	
	responseWriter.WriteHeader(204)
}