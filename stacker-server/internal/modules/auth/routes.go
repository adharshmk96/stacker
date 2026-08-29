package auth

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module bundles what the rest of the app needs from auth: its routes, its
// service, and the RequireAuth middleware every other module is mounted behind.
type Module struct {
	Service *Service
	handler *Handler
}

// New wires the module's repository, service and handler. resetData may be nil,
// which only disables the reset-everything endpoint.
func New(db *gorm.DB, mail MailSender, resetData ResetDataFunc, log *slog.Logger) (*Module, error) {
	repo := NewRepository(db)
	service, err := NewService(repo, resetData, mail, log.With("module", "auth"))
	if err != nil {
		return nil, err
	}

	return &Module{Service: service, handler: NewHandler(service)}, nil
}

// RegisterRoutes mounts the module under the given API group. The endpoints
// split in two: the ones a signed-out browser must reach, and the rest.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	authLimit := newIPLimiter(10, time.Minute)

	public := r.Group("/auth")
	public.POST("/login", authLimit, m.handler.login)
	public.POST("/forgot-password", authLimit, m.handler.forgotPassword)
	public.GET("/status", m.handler.status)
	public.POST("/register", m.handler.register)
	public.POST("/reset-password", m.handler.resetPassword)

	private := r.Group("/auth")
	private.Use(m.RequireAuth())
	private.GET("/me", m.handler.me)
	private.POST("/logout", m.handler.logout)
	private.PUT("/profile", m.handler.updateProfile)
	private.POST("/change-password", m.handler.changePassword)
	private.GET("/sessions", m.handler.sessions)
	private.DELETE("/sessions/:id", m.handler.revokeSession)
	private.POST("/reset-data", m.handler.resetData)
}
