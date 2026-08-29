package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type ipLimiter struct {
	mu      sync.Mutex
	entries map[string]*limitEntry
}

type limitEntry struct {
	count   int
	resetAt time.Time
}

func newIPLimiter(limit int, window time.Duration) gin.HandlerFunc {
	l := &ipLimiter{entries: make(map[string]*limitEntry)}
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP(), limit, window, time.Now()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, try again later"})
			return
		}
		c.Next()
	}
}

func (l *ipLimiter) allow(key string, limit int, window time.Duration, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || now.After(entry.resetAt) {
		l.entries[key] = &limitEntry{count: 1, resetAt: now.Add(window)}
		return true
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	return true
}
