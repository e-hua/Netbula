package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/e-hua/netbula/internal/app/worker"
	"github.com/e-hua/netbula/internal/logger"
	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
	"github.com/e-hua/netbula/lib/routers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

// The Api receiving requests from localhost:<Port>
type Api struct {
	Manager  ManagerService
	Router   *chi.Mux
	AuthToken string

	Logger logger.ManagerLogger
}

type ManagerService interface {
	// Managing workers
	UpdateWorkerNodes()
	AddWorkerAndClient(workerInfo *worker.Worker, client *http.Client)
	SelectWorker(t task.Task) (uuid.UUID, error)
	GetNodes() []node.Node

	SendWork() (targetEvent *task.TaskEvent, retErr error)

	AddTaskEvent(te task.TaskEvent)
	GetTask(taskId uuid.UUID) (task *task.Task, err error)
	GetTasks() (task []*task.Task, err error)

	UpdateTasksForever(ctx context.Context)
	SendTasksForever(ctx context.Context)
}

const (
	AuthenticationHeaderKey = "Netbula-Token"
)

func (a *Api) InitRouter(allowVerbose bool) {
	a.Router = chi.NewRouter()
	if allowVerbose {
		a.Router.Use(middleware.Logger)
	}
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
		token := r.Header.Get(AuthenticationHeaderKey)
		if token != a.AuthToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Api) Start(ctx context.Context, tlsListener net.Listener) {
	a.InitRouter(true)

	server := &http.Server{
		Handler: a.Router,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	server.Serve(tlsListener)
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
	tasks, err := a.Manager.GetTasks()

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
		resErr = fmt.Errorf("failed to parse taskId %s as UUID: %w", taskId, err)
		routers.RespondError(responseWriter, http.StatusBadRequest, resErr.Error())
		return
	}

	taskToStop, err := a.Manager.GetTask(parsedId)
	if taskToStop == nil {
		err = errors.New("no target task in TaskDb")
	}
	if err != nil {
		resErr = fmt.Errorf("failed to get task with ID [%s] from TaskDb: %w", parsedId.String(), err)
		routers.RespondError(responseWriter, http.StatusNotFound, resErr.Error())
		return
	}

	// Pass by value
	taskCopy := *taskToStop
	// TODO: Figure out the use of `stopTaskEvent.Task.State`
	// taskCopy.State = task.Running

	stopTaskEvent := task.TaskEvent{
		ID:          uuid.New(),
		TargetState: task.Completed,
		Timestamp:   time.Now(),
		Task:        taskCopy,
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
