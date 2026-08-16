package auth

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

/* ---- public ---- */

func (h *Handler) status(c *gin.Context) {
	status, err := h.service.Status()
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": status})
}

func (h *Handler) register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.Register(req, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.service.Login(req, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *Handler) forgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.ForgotPassword(req, appOrigin(c)); err != nil {
		respondError(c, err)
		return
	}
	// Deliberately the same answer for a known and an unknown address.
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"sent": true}})
}

func (h *Handler) resetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.ResetPassword(req); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"reset": true}})
}

/* ---- signed in ---- */

func (h *Handler) me(c *gin.Context) {
	user, _ := CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{"data": user})
}

func (h *Handler) logout(c *gin.Context) {
	session, _ := CurrentSession(c)
	if err := h.service.Logout(session); err != nil && !errors.Is(err, ErrSessionNotFound) {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"loggedOut": true}})
}

func (h *Handler) updateProfile(c *gin.Context) {
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, _ := CurrentUser(c)
	updated, err := h.service.UpdateProfile(user.ID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *Handler) changePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, _ := CurrentUser(c)
	session, _ := CurrentSession(c)
	if err := h.service.ChangePassword(user.ID, session.ID, req); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"changed": true}})
}

// sessions lists the signed-in devices, flagging the one asking so the UI can
// label it and refuse to revoke it from the list.
func (h *Handler) sessions(c *gin.Context) {
	user, _ := CurrentUser(c)
	current, _ := CurrentSession(c)

	sessions, err := h.service.Sessions(user.ID)
	if err != nil {
		respondError(c, err)
		return
	}

	type item struct {
		Session
		Current bool `json:"current"`
	}
	items := make([]item, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, item{Session: s, Current: s.ID == current.ID})
	}

	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) revokeSession(c *gin.Context) {
	user, _ := CurrentUser(c)
	if err := h.service.RevokeSession(user.ID, c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// resetData is the "reset everything" button in Settings → Account. It erases
// the account too, so the caller is signed out by definition.
func (h *Handler) resetData(c *gin.Context) {
	if err := h.service.ResetAllData(); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"reset": true}})
}

// appOrigin reconstructs the URL the browser reached stacker on, so the reset
// link in the log is one the operator can actually click.
func appOrigin(c *gin.Context) string {
	if origin := c.GetHeader("Origin"); origin != "" {
		return origin
	}

	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

// respondError maps this module's sentinel errors onto status codes.
func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrInvalidCredentials):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrSessionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrAlreadyRegistered), errors.Is(err, ErrEmailTaken), errors.Is(err, ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrInvalidUsername), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrInvalidResetToken):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.Error(err) //nolint:errcheck // recorded for the logging middleware
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
