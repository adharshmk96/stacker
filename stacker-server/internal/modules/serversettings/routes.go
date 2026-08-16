package serversettings

import "github.com/gin-gonic/gin"

type Module struct{ handler *Handler }

func New(configPath, stackName string) *Module {
	service := NewService(configPath, stackName)
	return &Module{handler: &Handler{service: service}}
}

func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	server := r.Group("/server")
	server.GET("", m.handler.get)
	server.PUT("/domain", m.handler.updateDomain)
	server.POST("/restart", m.handler.restart)
}
