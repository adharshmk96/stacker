package smtp

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Module struct {
	Service *Service
	handler *Handler
}

func New(db *gorm.DB, keyDir string, log *slog.Logger) (*Module, error) {
	repo := NewRepository(db)
	service, err := NewService(repo, keyDir, log.With("module", "smtp"))
	if err != nil {
		return nil, err
	}
	return &Module{Service: service, handler: &Handler{service: service}}, nil
}

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	group := r.Group("/settings/smtp")
	group.GET("", m.handler.get)
	group.PUT("", m.handler.update)
	group.POST("/test", m.handler.test)
}
