package monitoring

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"stacker/internal/modules/node"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) summary(c *gin.Context) {
	result, err := h.service.Summary(c.Request.Context(), c.Param("id"))
	h.respond(c, result, err)
}
func (h *Handler) dashboard(c *gin.Context) {
	result, err := h.service.Dashboard(c.Request.Context(), c.Param("id"), c.DefaultQuery("range", "24h"))
	h.respond(c, result, err)
}
func (h *Handler) respond(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"data": data})
		return
	}
	if errors.Is(err, node.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "monitoring is unavailable"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
