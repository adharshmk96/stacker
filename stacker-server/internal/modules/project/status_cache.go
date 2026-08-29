package project

import (
	"sync"
	"time"
)

const statusCacheTTL = 4 * time.Second

// statusCache holds a short-lived StatusAll result so dashboard polls do not
// spawn one docker subprocess per environment every few seconds.
type statusCache struct {
	mu      sync.Mutex
	expires time.Time
	items   []ProjectStatus
}

func (c *statusCache) get(now time.Time) ([]ProjectStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Before(c.expires) && c.items != nil {
		out := make([]ProjectStatus, len(c.items))
		copy(out, c.items)
		return out, true
	}
	return nil, false
}

func (c *statusCache) set(now time.Time, items []ProjectStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expires = now.Add(statusCacheTTL)
	c.items = make([]ProjectStatus, len(items))
	copy(c.items, items)
}

func (c *statusCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expires = time.Time{}
	c.items = nil
}
