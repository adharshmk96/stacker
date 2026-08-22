package monitoring

import "github.com/gin-gonic/gin"

type Module struct{ handler *Handler }

func New(nodes nodeLookup, metricsURL string) *Module {
	return &Module{handler: NewHandler(NewService(nodes, metricsURL))}
}
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	items := r.Group("/nodes/:id/metrics")
	items.GET("/summary", m.handler.summary)
	items.GET("/dashboard", m.handler.dashboard)
}
