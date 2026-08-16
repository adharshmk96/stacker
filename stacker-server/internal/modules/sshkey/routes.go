package sshkey

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module bundles what the rest of the app needs from ssh keys: its routes and
// its service (the node module resolves private key paths through it).
type Module struct {
	Service *Service
	handler *Handler
}

// New wires the module's repository, service and handler.
func New(db *gorm.DB, keyDir string, log *slog.Logger) *Module {
	repo := NewRepository(db)
	service := NewService(repo, keyDir, log.With("module", "sshkey"))

	return &Module{
		Service: service,
		handler: NewHandler(service),
	}
}

// RegisterRoutes mounts the module under the given API group.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	keys := r.Group("/ssh-keys")
	keys.GET("", m.handler.list)
	keys.POST("", m.handler.create)
	keys.GET("/:id", m.handler.get)
	keys.POST("/:id/rotate", m.handler.rotate)
	keys.DELETE("/:id", m.handler.remove)
}
