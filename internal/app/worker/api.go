package worker

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hashicorp/yamux"
)

type Api struct {
	Handlers
	Session *yamux.Session
	Router  *chi.Mux
}

func InitRouterFromHandlers(handlers Handlers) *chi.Mux {
	newRouter := chi.NewRouter()

	newRouter.Route("/tasks", func(router chi.Router) {
		router.Post("/", handlers.StartTaskHandler)
		router.Get("/", handlers.GetTasksHandler)
		router.Route("/{taskId}", func(router chi.Router) {
			router.Delete("/", handlers.StopTaskHandler)
		})
	})

	newRouter.Route("/info", func(router chi.Router) {
		router.Get("/", handlers.GetWorkerInfoHandler)
	})

	newRouter.Route("/stats", func(router chi.Router) {
		router.Get("/", handlers.GetWorkerStatsHandler)
	})

	return newRouter
}

func (a *Api) Start() {
	newRouter := InitRouterFromHandlers(a.Handlers)
	http.Serve(a.Session, newRouter)
}
