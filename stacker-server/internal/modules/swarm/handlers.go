package swarm

import (
	"errors"
	"net/http"

	"stacker/internal/modules/node"

	"github.com/gin-gonic/gin"
)

// Handler turns HTTP in and out; all decisions live in the service.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) list(c *gin.Context) {
	result, err := h.service.List(c.Request.Context(), Resource(c.Param("resource")), c.Query("node"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) action(c *gin.Context) {
	var req ActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.Action(c.Request.Context(), Resource(c.Param("resource")), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.Create(c.Request.Context(), Resource(c.Param("resource")), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

// respondError maps this module's errors, and the node module's docker errors
// it passes through, onto status codes.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnknownResource), errors.Is(err, ErrUnknownAction):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNodeRequired), errors.Is(err, ErrUnknownNode),
		errors.Is(err, ErrNameRequired), errors.Is(err, ErrImageRequired),
		errors.Is(err, ErrContentRequired), errors.Is(err, ErrReplicasNeeded),
		errors.Is(err, ErrGlobalService):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNoManager), errors.Is(err, node.ErrDockerMissing),
		errors.Is(err, node.ErrDockerNotRunning):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": err.Error()})
	case errors.Is(err, node.ErrSwarmUnreachable), errors.Is(err, node.ErrSwarmCommand),
		errors.Is(err, node.ErrSshKeyMissing):
		// A docker command that ran and failed is the daemon's answer, not a
		// bug here — its own words are what the user needs.
		c.JSON(http.StatusBadGateway, gin.H{"error": cleanErr(err)})
	default:
		c.Error(err) //nolint:errcheck // recorded for the logging middleware
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
