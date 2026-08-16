package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"stacker/internal/config"
	"stacker/internal/modules/node"
	"stacker/internal/modules/sshkey"
	"stacker/internal/modules/swarm"
	"stacker/internal/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// newRouter builds the gin engine and mounts every module under /api.
func newRouter(cfg config.Config, db *gorm.DB, log *slog.Logger) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery(), requestLogger(log), cors())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Modules are constructed here so the dependency direction is visible in
	// one place: ssh keys first, then nodes which consume the key service.
	keyModule := sshkey.New(db, cfg.KeyDir, log)
	nodeModule := node.New(db, keyModule.Service, log)
	// Swarm browses docker through the nodes it is given, so it is built last.
	swarmModule := swarm.New(nodeModule.Service, log)

	// The machine stacker is installed on is a node like any other, so it is
	// seeded on every start. A failure here is not fatal — the rest of the API
	// works without it.
	if _, err := nodeModule.Service.EnsureLocal(); err != nil {
		log.Error("could not register the local node", "error", err)
	}

	// The first node is the swarm manager, so the local node is swarm-enabled
	// as soon as stacker starts. It runs in the background because reaching
	// docker can take seconds, and it must never hold the server off its port
	// — a failure only means the node shows "Configure" in the UI.
	go nodeModule.Service.BootstrapSwarm(context.Background())

	api := r.Group("/api")
	keyModule.RegisterRoutes(api)
	nodeModule.RegisterRoutes(api)
	swarmModule.RegisterRoutes(api)

	// The embedded UI is the fallback, so it must be mounted after the API.
	if err := web.Register(r); err != nil {
		log.Error("could not mount the embedded UI", "error", err)
	}

	return r
}

// requestLogger logs one line per request, including any error the handler
// recorded with c.Error.
func requestLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start).Round(time.Millisecond).String(),
		}
		if err := c.Errors.Last(); err != nil {
			log.Error("request failed", append(attrs, "error", err.Error())...)
			return
		}
		log.Info("request", attrs...)
	}
}

// cors allows the Nuxt dev server to call the API from another origin.
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
