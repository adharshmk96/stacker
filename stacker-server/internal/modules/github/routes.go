package github

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Module struct{ handler *Handler }

func New(db *gorm.DB, log *slog.Logger) *Module {
	service := NewService(NewRepositoryStore(db), log.With("module", "github"))
	return &Module{handler: NewHandler(service)}
}

func (m *Module) RegisterPublicRoutes(api *gin.RouterGroup) {
	api.GET("/github/callback/:id", m.handler.callback)
	api.GET("/github/installations/:id/callback", m.handler.installationCallback)
	api.POST("/github/webhooks", m.handler.webhook)
}

func (m *Module) RegisterRoutes(api *gin.RouterGroup) {
	api.GET("/github", m.handler.current)
	api.POST("/github/apps", m.handler.start)
	api.GET("/github/repositories", m.handler.repositories)
	api.DELETE("/github", m.handler.remove)
}
