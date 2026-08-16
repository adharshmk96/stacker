package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) current(c *gin.Context) {
	app, err := h.service.Current()
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusOK, gin.H{"data": nil})
		return
	}
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": app})
}

func (h *Handler) start(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.Start(req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) repositories(c *gin.Context) {
	repos, err := h.service.Repositories(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": repos})
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.service.Delete(); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) callback(c *gin.Context) {
	if err := h.service.Convert(c.Request.Context(), c.Param("id"), c.Query("token"), c.Query("code")); err != nil {
		_ = c.Error(err)
		c.Redirect(http.StatusFound, "/dashboard/settings/git-provider?github=error")
		return
	}
	c.Redirect(http.StatusFound, "/dashboard/settings/git-provider?github=created")
}

func (h *Handler) installationCallback(c *gin.Context) {
	installationID, err := strconv.ParseInt(c.Query("installation_id"), 10, 64)
	if err != nil {
		_ = c.Error(err)
		c.Redirect(http.StatusFound, "/dashboard/settings/git-provider?github=error")
		return
	}
	if err := h.service.CompleteInstallation(c.Request.Context(), c.Param("id"), c.Query("token"), installationID); err != nil {
		_ = c.Error(err)
		c.Redirect(http.StatusFound, "/dashboard/settings/git-provider?github=error")
		return
	}
	c.Redirect(http.StatusFound, "/dashboard/settings/git-provider?github=installed")
}

func (h *Handler) webhook(c *gin.Context) {
	app, err := h.service.Current()
	if err != nil || app.WebhookSecret == "" {
		c.Status(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 10<<20))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	signature := c.GetHeader("X-Hub-Signature-256")
	want := hmac.New(sha256.New, []byte(app.WebhookSecret))
	_, _ = want.Write(body)
	provided, err := hex.DecodeString(trimSHA256(signature))
	if err != nil || !hmac.Equal(provided, want.Sum(nil)) {
		c.Status(http.StatusUnauthorized)
		return
	}
	c.Status(http.StatusNoContent)
}

func trimSHA256(value string) string {
	if len(value) > 7 && value[:7] == "sha256=" {
		return value[7:]
	}
	return ""
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrInvalidBaseURL), errors.Is(err, ErrInvalidCallback), errors.Is(err, ErrNotInstalled):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.Error(err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
	}
}
