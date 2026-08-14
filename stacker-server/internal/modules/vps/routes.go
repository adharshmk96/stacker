package vps

import (
	"log/slog"

	"stacker/internal/modules/sshkey"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module bundles the VPS routes and their dependencies.
type Module struct {
	Service *Service
	handler *Handler
}

// New wires the module. It takes the ssh key service directly — VPS depends on
// ssh keys, never the other way round.
func New(db *gorm.DB, keySvc *sshkey.Service, log *slog.Logger) *Module {
	repo := NewRepository(db)
	service := NewService(repo, keySvc, log.With("module", "vps"))

	return &Module{
		Service: service,
		handler: NewHandler(service),
	}
}

// RegisterRoutes mounts the module under the given API group.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	items := r.Group("/vps")
	items.GET("", m.handler.list)
	items.POST("", m.handler.create)
	// Not under /:id — the key is installed while the host is still being
	// entered, before there is a record to hang it off.
	items.POST("/install-key", m.handler.installKey)

	byID := items.Group("/:id", requireID())
	byID.GET("", m.handler.get)
	byID.PUT("", m.handler.update)
	byID.DELETE("", m.handler.remove)
	byID.POST("/check-key", m.handler.checkKey)
}
