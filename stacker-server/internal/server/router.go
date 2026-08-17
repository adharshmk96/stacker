package server

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"stacker/internal/config"
	"stacker/internal/database"
	"stacker/internal/modules/auth"
	githubprovider "stacker/internal/modules/github"
	"stacker/internal/modules/node"
	"stacker/internal/modules/project"
	"stacker/internal/modules/serversettings"
	"stacker/internal/modules/sshkey"
	"stacker/internal/modules/swarm"
	"stacker/internal/web"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// newRouter builds the gin engine and mounts every module under /api.
func newRouter(cfg config.Config, db *gorm.DB, log *slog.Logger) (*gin.Engine, error) {
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
	if _, err := keyModule.Service.EnsureDefault(); err != nil {
		return nil, err
	}
	nodeModule := node.New(db, keyModule.Service, log)

	// Auth guards every other module, so it is built first — and it is handed
	// the data wipe rather than importing it, since erasing nodes is the node
	// module's business, not auth's. The closure runs long after startup.
	authModule, err := auth.New(db, func() error {
		if err := database.Reset(db, log); err != nil {
			return err
		}
		if _, err := keyModule.Service.EnsureDefault(); err != nil {
			return err
		}
		// Bootstrapped records were removed by the wipe, so recreate both.
		_, err := nodeModule.Service.EnsureLocal()
		return err
	}, log)
	if err != nil {
		return nil, err
	}
	// Swarm browses docker through the nodes it is given, so it is built last.
	swarmModule := swarm.New(nodeModule.Service, log)
	githubModule := githubprovider.New(db, log)
	serverModule := serversettings.New(cfg.TraefikDynamicPath, cfg.StackName, cfg.AdvertiseAddr)

	// Projects deploy on this machine's docker, so they take the installation's
	// own paths rather than a node: the workspace they clone and build in, the
	// Traefik directory their hostnames are published through, and the overlay
	// network Traefik shares with whatever they deploy.
	projectModule := project.New(db, project.Options{
		WorkRoot:           filepath.Join(cfg.DataDir, "workspaces"),
		TraefikDynamicPath: cfg.TraefikDynamicPath,
		Network:            cfg.StackName + "_proxy",
		Token:              githubModule.Service.CloneToken,
	}, log)
	// Runs live in the process that started them, so a row still marked running
	// belongs to a process that is gone. Closing those out is startup work.
	if err := projectModule.Service.Recover(); err != nil {
		log.Error("could not reconcile deployments after restart", "error", err)
	}

	// The machine stacker is installed on is a node like any other, so it is
	// seeded on every start. A failure here is not fatal — the rest of the API
	// works without it.
	if _, err := nodeModule.Service.EnsureLocal(); err != nil {
		log.Error("could not register the local node", "error", err)
	}

	// install.sh owns swarm creation. The app only mirrors the manager state
	// that already exists on the Docker host.
	go nodeModule.Service.SyncLocalSwarm(cfg.AdvertiseAddr)

	api := r.Group("/api")
	// Auth mounts its own public/private split; everything else is signed-in only.
	authModule.RegisterRoutes(api)
	githubModule.RegisterPublicRoutes(api)

	protected := api.Group("")
	protected.Use(authModule.RequireAuth())
	keyModule.RegisterRoutes(protected)
	nodeModule.RegisterRoutes(protected)
	swarmModule.RegisterRoutes(protected)
	githubModule.RegisterRoutes(protected)
	serverModule.RegisterRoutes(protected)
	projectModule.RegisterRoutes(protected)

	// The embedded UI is the fallback, so it must be mounted after the API.
	if err := web.Register(r); err != nil {
		log.Error("could not mount the embedded UI", "error", err)
	}

	return r, nil
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
