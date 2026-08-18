package github

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module bundles the GitHub routes. Service is exported because other modules
// consume it directly — the project module mints a clone token through it, the
// same way node consumes the ssh key service.
type Module struct {
	Service *Service
	handler *Handler
}

func New(db *gorm.DB, log *slog.Logger) *Module {
	service := NewService(NewRepositoryStore(db), log.With("module", "github"))
	return &Module{Service: service, handler: NewHandler(service)}
}

func (m *Module) SetPushHandler(handler PushHandler) { m.handler.SetPushHandler(handler) }

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
