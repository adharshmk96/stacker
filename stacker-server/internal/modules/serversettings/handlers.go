package serversettings

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func (h *Handler) get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context())
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) updateDomain(c *gin.Context) {
	var req DomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	domain, err := h.service.UpdateDomain(req.Domain)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"domain": domain}})
}

func (h *Handler) restart(c *gin.Context) {
	var req RestartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.service.Restart(c.Request.Context(), req.Target); err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"restarting": req.Target}})
}

func (h *Handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidDomain), errors.Is(err, ErrUnknownTarget):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrConfigMissing):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": err.Error()})
	default:
		c.Error(err) //nolint:errcheck
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
