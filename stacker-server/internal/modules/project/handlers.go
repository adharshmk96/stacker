package project

import (
	"errors"
	"net/http"
	"strconv"

	"stacker/internal/modules/auth"

	"github.com/gin-gonic/gin"
)

// Handler turns HTTP in and out; every decision lives in the service.
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) list(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) get(c *gin.Context) {
	item, err := h.service.Get(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) create(c *gin.Context) {
	var req WriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.service.Create(req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (h *Handler) composePreview(c *gin.Context) {
	var req ComposePreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	compose, err := h.service.PreviewCompose(c.Request.Context(), req.Git)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"compose": compose}})
}

func (h *Handler) update(c *gin.Context) {
	var req WriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.service.Update(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// statusAll is the reading the card grid polls.
func (h *Handler) statusAll(c *gin.Context) {
	items, err := h.service.StatusAll(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// status is the reading the detail page polls.
func (h *Handler) status(c *gin.Context) {
	item, err := h.service.Status(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

// deploy answers 202 with the queued run: the work has started, not finished, and
// the client follows it through the run's logs and the environment's status.
func (h *Handler) deploy(c *gin.Context) {
	var req DeployRequest
	// The body is optional — the Deploy button sends none.
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// The signed-in user is the actor unless the caller named one, so a run's
	// history says who asked for it.
	if req.Actor == "" {
		if user, ok := auth.CurrentUser(c); ok {
			req.Actor = user.Username
		}
	}

	deployment, err := h.service.Deploy(c.Param("id"), c.Param("envId"), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": deployment})
}

// stop removes an environment's stack, leaving its configuration in place.
func (h *Handler) stop(c *gin.Context) {
	if err := h.service.Stop(c.Request.Context(), c.Param("id"), c.Param("envId")); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"stopped": c.Param("envId")}})
}

func (h *Handler) deployments(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := h.service.Deployments(c.Query("projectId"), limit)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) deployment(c *gin.Context) {
	item, err := h.service.Deployment(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

// logs answers the lines after `after`, which is the cursor the previous
// response handed back. It is a read, so it is a GET and safe to poll.
func (h *Handler) logs(c *gin.Context) {
	after, _ := strconv.Atoi(c.DefaultQuery("after", "0"))

	result, err := h.service.Logs(c.Param("id"), after)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// serviceLogs answers the current tail of one compose service's container
// output. It is a GET, safe to poll, and re-reads the tail on every call since
// docker's own log command has no cursor to resume from.
func (h *Handler) serviceLogs(c *gin.Context) {
	tail, _ := strconv.Atoi(c.DefaultQuery("tail", "300"))

	result, err := h.service.ServiceLogs(c.Request.Context(), c.Param("id"), c.Param("envId"), c.Param("service"), tail)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) cancel(c *gin.Context) {
	if err := h.service.Cancel(c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"cancelled": c.Param("id")}})
}

// respondError maps the module's errors onto status codes. Anything unrecognised
// is a bug rather than a bad request, so it is logged and answered generically.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrEnvNotFound), errors.Is(err, ErrDeployNotFound), errors.Is(err, ErrServiceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNameTaken), errors.Is(err, ErrDomainTaken), errors.Is(err, ErrAlreadyDeploying):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotRunning):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": err.Error()})
	case errors.Is(err, ErrTraefikMissing):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": err.Error()})
	case errIsValidation(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.Error(err) //nolint:errcheck // recorded for the request log
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
