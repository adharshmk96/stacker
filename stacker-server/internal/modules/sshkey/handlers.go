package sshkey

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
	keys, err := h.service.List()
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": keys})
}

func (h *Handler) get(c *gin.Context) {
	key, err := h.service.Get(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": key})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	key, err := h.service.Create(req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": key})
}

func (h *Handler) remove(c *gin.Context) {
	if err := h.service.Delete(c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) rotate(c *gin.Context) {
	key, err := h.service.Rotate(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": key})
}

// respondError maps this module's sentinel errors onto status codes.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNameTaken), errors.Is(err, ErrKeyInUse), errors.Is(err, ErrDefaultKey):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidName), errors.Is(err, ErrUnknownType):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.Error(err) //nolint:errcheck // recorded for the logging middleware
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
