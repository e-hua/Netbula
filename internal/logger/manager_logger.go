package logger

import (
	"log/slog"
	"os"

	"github.com/e-hua/netbula/internal/node"
	"github.com/e-hua/netbula/internal/task"
)

// Custom type with `slog.Logger` embedded
// Providing custom Logging methods for clarity and escape hatches from `slog.Logger`
type ManagerLogger struct {
	slog.Logger
}

// Initialize a structured logger for the Netbula Manager
//
// Set `allowVerbose` to be false if under deployment mode
// Set it to be true if developing the application
func NewManagerLogger(allowVerbose bool) *ManagerLogger {
	var options *slog.HandlerOptions
	// Set the minimum logging level for the new logger
	if allowVerbose {
		// Allowing DEBUG, INFO, WARN and ERROR level
		options = &slog.HandlerOptions{Level: slog.LevelDebug}
	} else {
		// Allowing only INFO, WARN and ERROR level
		// DEBUG level logging will be quitely discarded
		options = &slog.HandlerOptions{Level: slog.LevelInfo}
	}

	// Include the source file of the logging
	options.AddSource = allowVerbose

	handler := slog.NewJSONHandler(os.Stderr, options)
	newLogger := slog.New(handler)

	return &ManagerLogger{
		*newLogger,
	}
}

func NewManagerLoggerWithSubsystem(parentLogger ManagerLogger, subsystemTag string) *ManagerLogger {
	return &ManagerLogger{
		*parentLogger.Logger.With("subsystem", subsystemTag),
	}
}

// Received a request to add another TaskEvent from the manager API
// (Add the taskEvent to the queue of pending taskEvents afterwards)
//
// Triggered when [StartTaskHandler/StopTaskHandler] is invoked
// [github.com/e-hua/netbula/blob/main/internal/app/manager/api.go]
func (m *ManagerLogger) TaskReceived(taskEvent *task.TaskEvent) {
	m.Info("Task received from control", "task_event", taskEvent)
}

// Manager sends the target TaskEvent to the worker node
//
// Triggered when [SendWork] is invoked
// [github.com/e-hua/netbula/blob/main/internal/app/manager/manager.go]
func (m *ManagerLogger) TaskSent(taskEvent *task.TaskEvent) {
	m.Info("Task sent to worker", "task_event", taskEvent)
}

// Triggered when `SyncTasks` in /internal/app/manager/cluster.go is invoked
func (m *ManagerLogger) TasksSynced(taskEvent *task.TaskEvent) {
	// Less important since this check is done periodically
	m.Debug("Task synced", "task_event", taskEvent)
}

// TODO: Add checks
// Detected by `UpdateTask` in /internal/app/manager/state.go
func (m *ManagerLogger) TaskStatusChanged(prevTask task.Task, newTask task.Task) {
	m.Info("Task status change detected",
		slog.Any("before", prevTask),
		slog.Any("after", newTask),
	)
}

// Triggered when `UpdateWorkerNodes` in /internal/app/manager/cluster.go is invoked
func (m *ManagerLogger) WorkerNodesUpdated(nodes []*node.Node) {
	var nodeLogValues []slog.Value

	for _, nodeVal := range nodes {
		nodeLogValues = append(nodeLogValues, nodeVal.LogValue())
	}

	// Less important since the API for showing worker stats is provided
	m.Debug("Worker nodes stats updated", "nodes", nodeLogValues)
}

// TODO: Add checks
// Detected by `UpdateWorkerNodes` in /internal/app/manager/cluster.go
func (m *ManagerLogger) WorkerConnected(workerNode *node.Node) {
	m.Info("Worker Connected", "worker_node", workerNode)
}

// TODO: Add checks
// Detected by `UpdateWorkerNodes` in /internal/app/manager/cluster.go
func (m *ManagerLogger) WorkerReconnected(workerNode *node.Node) {
	m.Info("Worker Reconnected", "worker_node", workerNode)
}

// TODO: Add checks
// Detected by `UpdateWorkerNodes` in /internal/app/manager/cluster.go
func (m *ManagerLogger) WorkerDisconnected(workerNode *node.Node) {
	m.Error("Worker Disconnected", "worker_node", workerNode)
}
