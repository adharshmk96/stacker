package node

import (
	"errors"
	"net/http"

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

// pingAll probes every node and answers with the refreshed list. It is a POST
// because it reaches out to every host, which is an action, not a read.
func (h *Handler) pingAll(c *gin.Context) {
	items, err := h.service.PingAll(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// ping probes one node and answers with the refreshed row.
func (h *Handler) ping(c *gin.Context) {
	item, err := h.service.Ping(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

// refreshSwarmAll re-reads every configured node's swarm state and answers with
// the refreshed list. Like pingAll it is a POST: it reaches out to every host.
func (h *Handler) refreshSwarmAll(c *gin.Context) {
	items, err := h.service.RefreshAll(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
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

func (h *Handler) update(c *gin.Context) {
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.service.Update(c.Param("id"), req)
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

func (h *Handler) checkKey(c *gin.Context) {
	result, err := h.service.CheckKey(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) installKey(c *gin.Context) {
	var req InstallKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.InstallKey(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// configure runs the remote-node checklist and joins it to the swarm.
// Installing docker takes minutes, so the run happens in the
// background and this answers 202 with the checklist as it stands; the client
// polls provisionStatus for the rest.
func (h *Handler) configure(c *gin.Context) {
	job, err := h.service.Provision(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": job})
}

// provisionStatus reports the latest configure run for a node.
func (h *Handler) provisionStatus(c *gin.Context) {
	job, err := h.service.ProvisionStatus(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": job})
}

func (h *Handler) promote(c *gin.Context) {
	result, err := h.service.Promote(c.Request.Context(), c.Param("id"))
	h.respondSwarm(c, result, err)
}

func (h *Handler) demote(c *gin.Context) {
	result, err := h.service.Demote(c.Request.Context(), c.Param("id"))
	h.respondSwarm(c, result, err)
}

func (h *Handler) leaveSwarm(c *gin.Context) {
	result, err := h.service.Leave(c.Request.Context(), c.Param("id"))
	h.respondSwarm(c, result, err)
}

func (h *Handler) refreshSwarm(c *gin.Context) {
	result, err := h.service.RefreshSwarm(c.Request.Context(), c.Param("id"))
	h.respondSwarm(c, result, err)
}

// respondSwarm is the shared tail of every swarm handler: the node and a
// message on success, the mapped error otherwise.
func (h *Handler) respondSwarm(c *gin.Context, result SwarmResult, err error) {
	if err != nil {
		respondError(c, err)
		return
	}
	// Swarm writes return a repository-backed Node, so restore the transient
	// reachability fields before sending the row back to the UI.
	h.service.decorate(&result.Node)
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// respondError maps this module's sentinel errors onto status codes.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNoProvisionRun):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNameTaken), errors.Is(err, ErrAlreadyInSwarm),
		errors.Is(err, ErrForeignSwarm), errors.Is(err, ErrNodeInSwarm):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrSwarmBusy):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrLocalNode), errors.Is(err, ErrLocalSetupManaged):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidSsh), errors.Is(err, ErrSshKeyMissing),
		errors.Is(err, ErrNotWorker), errors.Is(err, ErrNotManagerRole),
		errors.Is(err, ErrNotInSwarm), errors.Is(err, ErrLastManager),
		errors.Is(err, ErrAdvertiseAddr), errors.Is(err, ErrPasswordRequired):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrCopyIDMissing), errors.Is(err, ErrDockerMissing),
		errors.Is(err, ErrDockerNotRunning), errors.Is(err, ErrNoManager),
		errors.Is(err, ErrKeyNotVerified), errors.Is(err, ErrUnsupportedOS),
		errors.Is(err, ErrLocalNotLinux), errors.Is(err, ErrSudoRequired),
		errors.Is(err, ErrCurlMissing), errors.Is(err, ErrDockerInstall):
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": err.Error()})
	case errors.Is(err, ErrManagerUnhealthy), errors.Is(err, ErrSwarmUnreachable),
		errors.Is(err, ErrSwarmCommand):
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	default:
		c.Error(err) //nolint:errcheck // recorded for the logging middleware
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
