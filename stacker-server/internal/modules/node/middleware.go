package node

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// requireID rejects requests with an empty :id before any handler runs, so the
// handlers can assume the parameter is present.
func requireID() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Param("id") == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "id is required"})
			return
		}
		c.Next()
	}
}
