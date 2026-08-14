// Package web embeds the built Nuxt UI and serves it from the binary.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// dist holds the output of `scripts/build-ui.sh` (`task build:ui`). Only
// dist/.gitkeep is committed — it keeps the directory, and therefore this
// embed, valid on a fresh clone where the UI has never been built.
//
//go:embed all:dist
var dist embed.FS

const notBuilt = "The stacker UI is not embedded in this binary. Run `task build:ui`, then rebuild.\n"

// Register mounts the embedded UI as the fallback for anything the API did not
// claim. Assets are served straight from the embedded FS; every other path
// falls back to index.html so client-side routes like /dashboard/vps resolve on
// a hard refresh.
func Register(r *gin.Engine) error {
	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		return err
	}

	fileServer := http.FileServer(http.FS(assets))

	// A binary built without running the UI build still serves the API happily;
	// it just says so instead of pretending the page is missing.
	index, indexErr := fs.ReadFile(assets, "index.html")

	r.NoRoute(func(c *gin.Context) {
		// The API owns /api — an unmatched route there is a 404, not the UI.
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		if indexErr != nil {
			c.String(http.StatusServiceUnavailable, notBuilt)
			return
		}

		if exists(assets, c.Request.URL.Path) {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// A missing build asset is a missing file, not a page. Falling through
		// would answer a .js request with HTML and fail as a MIME error.
		if strings.HasPrefix(c.Request.URL.Path, "/_nuxt/") {
			c.String(http.StatusNotFound, "not found")
			return
		}

		// Unknown path with no file behind it: hand the SPA its entry point and
		// let the router decide whether it is a real page.
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})

	return nil
}

func exists(assets fs.FS, urlPath string) bool {
	name := path.Clean(strings.TrimPrefix(urlPath, "/"))
	if name == "" || name == "." {
		return false
	}

	info, err := fs.Stat(assets, name)
	return err == nil && !info.IsDir()
}
