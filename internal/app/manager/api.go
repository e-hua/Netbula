package manager

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/logger"
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
	Port     int
	Manager  *Manager
	Router   *chi.Mux
	TlsToken string

	Logger logger.ManagerLogger
}

func (a *Api) initRouter() {
	a.Router = chi.NewRouter()
	a.Router.Use(middleware.Logger)
	a.Router.Use(a.Authenticate)

	a.Router.Route("/tasks", func(r chi.Router) {
		r.Post("/", a.StartTaskHandler)
		r.Get("/", a.GetTasksHandler)
		r.Route("/{taskId}", func(r chi.Router) {
			r.Delete("/", a.StopTaskHandler)
		})
	})

	a.Router.Route("/nodes", func(r chi.Router) {
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

	// Printing to stdout for normal users to see 
	log.Printf("Manager API (Secure) listening on %d\n", a.Port)

	// Log the critical info to the stderr
	a.Logger.Info("Manager API started", slog.Int("port_number", a.Port))

	err := server.ListenAndServeTLS("", "")
	if (err != nil) {
		a.Logger.Error("Failed to start server for control program to connect to", "error", err)	
	}
}

// POST localhost:<Port>/tasks
//
// Only responsible for putting the task in the queue of pending task events
// TODO: Combine [StartTaskHandler] with [StopTaskHandler]
func (a *Api) StartTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	newTaskEvent := task.TaskEvent{}
	err := decoder.Decode(&newTaskEvent)
	if err != nil {
		resErr := fmt.Errorf("failed to unmarshal body: %w", err)
		routers.RespondError(responseWriter, http.StatusBadRequest, resErr.Error())
		a.Logger.Error("Failed when handling request to start a task", "error", resErr)
		return
	}

	a.Logger.TaskReceived(&newTaskEvent)

	a.Manager.AddTaskEvent(newTaskEvent)
	routers.RespondJSON(responseWriter, http.StatusCreated, newTaskEvent.Task)
}

// GET localhost:<Port>/tasks
func (a *Api) GetTasksHandler(responseWriter http.ResponseWriter, request *http.Request) {
	tasks, err := a.Manager.State.TaskDb.List()
	if err != nil {
		resErr := fmt.Errorf("failed to get tasks from TaskDb: %w", err)
		routers.RespondError(responseWriter, http.StatusInternalServerError, resErr.Error())
		a.Logger.Error("Failed when handling request to list tasks", "error", resErr)
		return
	}

	if tasks == nil {
		tasks = []*task.Task{}
	}

	routers.RespondJSON(responseWriter, http.StatusOK, tasks)
}

// DELETE localhost:<Port>/tasks/{taskId}

// Only responsible for putting the task in the queue of pending task events
func (a *Api) StopTaskHandler(responseWriter http.ResponseWriter, request *http.Request) {
	var resErr error

	defer func() {
		if resErr != nil {
			a.Logger.Error("Failed when handling request to stop task", "error", resErr)
		}
	}()

	taskId := chi.URLParam(request, "taskId")
	parsedId, err := uuid.Parse(taskId)
	if err != nil {
		resErr = fmt.Errorf("failed to parse taskId %s as UUID: %w", taskId ,err)
		routers.RespondError(responseWriter, http.StatusBadRequest, resErr.Error())
		return 
	}


	taskToStop, err := a.Manager.State.TaskDb.Get(parsedId.String())
	if err != nil {
		resErr = fmt.Errorf("failed to get task with ID [%s] from TaskDb: %w",parsedId.String(), err)
		routers.RespondError(responseWriter, http.StatusNotFound, resErr.Error())
		return
	}

	// Pass by value
	taskCopy := *taskToStop
	taskCopy.State = task.Completed

	stopTaskEvent := task.TaskEvent{
		ID:          uuid.New(),
		TargetState: task.Completed,
		Timestamp:   time.Now(),
		Task: taskCopy,
	}

	a.Logger.TaskReceived(&stopTaskEvent)

	a.Manager.AddTaskEvent(stopTaskEvent)

	responseWriter.WriteHeader(http.StatusNoContent)
}

// GET localhost:<Port>/nodes
func (a *Api) GetNodesHandler(responseWriter http.ResponseWriter, request *http.Request) {
	// Make sure nodes stats are up to date 
	a.Manager.UpdateWorkerNodes()

	nodes := a.Manager.GetNodes()
	if nodes == nil {
		nodes = []node.Node{}
	}

	routers.RespondJSON(responseWriter, http.StatusOK, nodes)
}
