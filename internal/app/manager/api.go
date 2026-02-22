package manager

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/networks/security"
	"github.com/e-hua/netbula/internal/node"
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
	TlsToken string
}

func (a *Api) initRouter() {
	a.Router = chi.NewRouter()
	a.Router.Use(middleware.Logger)
	a.Router.Use(a.Authenticate)

	a.Router.Route("/tasks", func (r chi.Router) {
		r.Post("/", a.StartTaskHandler)
		r.Get("/", a.GetTasksHandler)
		r.Route("/{taskId}", func(r chi.Router) {
			r.Delete("/", a.StopTaskHandler)
		})		
	})

	a.Router.Route("/nodes", func (r chi.Router) {
		r.Get("/", a.GetNodesHandler)
	})
}

func (a *Api) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Netbula-Token")
		if token != a.TlsToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Api) Start(certs tls.Certificate) {
	a.initRouter()
	tlsConfig := security.GetManagerTlsConfig(certs)
	server := &http.Server{
		Addr:      fmt.Sprintf("0.0.0.0:%d", a.Port),
		Handler:   a.Router,
		TLSConfig: tlsConfig,
	}

	log.Printf("Manager API (Secure) listening on %d\n", a.Port)
	log.Fatal(server.ListenAndServeTLS("", ""))
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

// GET localhost:<Port>/nodes
func (a *Api) GetNodesHandler(responseWriter http.ResponseWriter, request *http.Request) {
	nodes := a.Manager.GetNodes()
	if (nodes == nil)	{
		nodes = []node.Node{}
	}

	routers.RespondJSON(responseWriter, http.StatusOK, nodes)
}