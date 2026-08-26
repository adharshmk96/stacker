package project

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module bundles the Project routes and their dependencies.
type Module struct {
	Service *Service
	handler *Handler
}

// New wires the module.
func New(db *gorm.DB, opts Options, log *slog.Logger) *Module {
	service := NewService(NewRepository(db), opts, log.With("module", "project"))

	return &Module{
		Service: service,
		handler: NewHandler(service),
	}
}

// RegisterRoutes mounts the module under the given API group.
//
// Deployments are mounted as their own collection rather than only under a
// project: the Deployments page lists runs across every project, and a run
// outlives the project it belonged to only as history, never as a child route.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	projects := r.Group("/projects")
	projects.GET("", m.handler.list)
	projects.POST("", m.handler.create)
	projects.POST("/compose-preview", m.handler.composePreview)
	// Ahead of /:id so `status` is not read as a project id.
	projects.GET("/status", m.handler.statusAll)

	byID := projects.Group("/:id")
	byID.GET("", m.handler.get)
	byID.PUT("", m.handler.update)
	byID.DELETE("", m.handler.remove)
	byID.GET("/status", m.handler.status)

	env := byID.Group("/environments/:envId")
	// A POST: it starts work on the host, and answers 202 rather than the result.
	env.POST("/deploy", m.handler.deploy)
	env.POST("/stop", m.handler.stop)

	deployments := r.Group("/deployments")
	deployments.GET("", m.handler.deployments)
	deployments.GET("/:id", m.handler.deployment)
	deployments.GET("/:id/logs", m.handler.logs)
	deployments.POST("/:id/cancel", m.handler.cancel)
}
