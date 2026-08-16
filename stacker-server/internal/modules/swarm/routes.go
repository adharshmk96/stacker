package swarm

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// Module bundles the Swarm routes and their dependencies.
type Module struct {
	Service *Service
	handler *Handler
}

// New wires the module. It takes the node service directly — Swarm depends on
// Nodes for the roster and for running docker, never the other way round.
func New(nodes nodes, log *slog.Logger) *Module {
	service := NewService(nodes, log.With("module", "swarm"))

	return &Module{
		Service: service,
		handler: NewHandler(service),
	}
}

// RegisterRoutes mounts the module under the given API group.
//
// The resource is a path parameter rather than nine sets of routes because
// every list behaves identically from HTTP's point of view — what differs is
// the docker command behind it, which is the service's business.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	items := r.Group("/swarm/:resource")
	items.GET("", m.handler.list)
	// One endpoint for every row action: the UI's action menu is table-driven
	// and posts the action key, so the routes need not enumerate them.
	items.POST("/action", m.handler.action)
	items.POST("", m.handler.create)
}
