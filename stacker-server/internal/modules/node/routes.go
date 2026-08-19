package node

import (
	"log/slog"

	"stacker/internal/modules/sshkey"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Module bundles the Node routes and their dependencies.
type Module struct {
	Service *Service
	handler *Handler
}

// New wires the module. It takes the ssh key service directly — Node depends on
// ssh keys, never the other way round.
func New(db *gorm.DB, keySvc *sshkey.Service, log *slog.Logger) *Module {
	repo := NewRepository(db)
	service := NewService(repo, keySvc, log.With("module", "node"))

	return &Module{
		Service: service,
		handler: NewHandler(service),
	}
}

// RegisterRoutes mounts the module under the given API group.
func (m *Module) RegisterRoutes(r *gin.RouterGroup) {
	items := r.Group("/nodes")
	items.GET("", m.handler.list)
	items.POST("", m.handler.create)
	// Not under /:id — the key is installed while the host is still being
	// entered, before there is a record to hang it off.
	items.POST("/install-key", m.handler.installKey)
	// A reachability sweep over every node at once — what the table polls.
	items.POST("/ping", m.handler.pingAll)
	// The same sweep for swarm state, which the page runs on load so a host
	// that changed behind stacker's back shows up straight away.
	items.POST("/refresh-swarm", m.handler.refreshSwarmAll)

	byID := items.Group("/:id", requireID())
	byID.GET("", m.handler.get)
	byID.PUT("", m.handler.update)
	byID.DELETE("", m.handler.remove)
	byID.POST("/check-key", m.handler.checkKey)
	byID.POST("/ping", m.handler.ping)
	// A websocket, not a request/response call: it carries an interactive shell
	// on the node for the browser terminal.
	byID.GET("/terminal", m.handler.terminal)

	swarm := byID.Group("/swarm")
	// Configure is one verb for both cases: it inits the local node as the
	// first manager and joins every other node to it, so the UI does not have
	// to know which applies.
	swarm.POST("/configure", m.handler.configure)
	// The checklist of the run configure started, polled while it works.
	swarm.GET("/configure", m.handler.provisionStatus)
	swarm.POST("/promote", m.handler.promote)
	swarm.POST("/demote", m.handler.demote)
	swarm.POST("/leave", m.handler.leaveSwarm)
	swarm.POST("/refresh", m.handler.refreshSwarm)
}
