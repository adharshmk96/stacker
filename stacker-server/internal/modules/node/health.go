package node

import (
	"context"
	"strings"
	"sync"
	"time"
)

// pingTimeout keeps a reachability sweep snappy: this is a liveness check, not
// a diagnosis, so a host that has not answered in a few seconds is offline for
// the purposes of the table.
const pingConnectTimeout = 5 * time.Second

// Reachability is whether the host answered the last time stacker looked.
type Reachability string

const (
	// ReachabilityUnknown means no probe has run since stacker started.
	ReachabilityUnknown Reachability = "unknown"
	ReachabilityOnline  Reachability = "online"
	ReachabilityOffline Reachability = "offline"
)

// health is one node's last reachability reading. It lives in memory rather
// than the database: it describes the host right now, and a reading that
// outlived a restart would only be a stale claim.
type health struct {
	State   Reachability
	At      time.Time
	Message string
}

// healthCache holds the last reading per node id.
type healthCache struct {
	mu      sync.RWMutex
	entries map[string]health
}

func newHealthCache() *healthCache {
	return &healthCache{entries: map[string]health{}}
}

func (c *healthCache) get(id string) (health, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[id]
	return entry, ok
}

func (c *healthCache) set(id string, entry health) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry
}

func (c *healthCache) drop(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}

// decorate fills the transient reachability fields on a node from the cache.
func (s *Service) decorate(item *Node) {
	if entry, ok := s.health.get(item.ID); ok {
		item.Reachability = entry.State
		item.ReachabilityMessage = entry.Message
		at := entry.At
		item.ReachableCheckedAt = &at
		return
	}
	item.Reachability = ReachabilityUnknown
}

// Ping probes one node and records the result.
func (s *Service) Ping(ctx context.Context, id string) (Node, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return Node{}, err
	}

	s.health.set(item.ID, s.probe(ctx, item))
	s.decorate(&item)
	return item, nil
}

// PingAll probes every node at once and answers with the full, decorated list.
// The sweep is parallel because it is what the table waits on, and one
// unreachable host must not hold up the rest.
func (s *Service) PingAll(ctx context.Context) ([]Node, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	results := make([]health, len(items))

	for i := range items {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = s.probe(ctx, items[i])
		}(i)
	}
	wg.Wait()

	for i := range items {
		s.health.set(items[i].ID, results[i])
		s.decorate(&items[i])
	}
	return items, nil
}

// probe answers the only question the status column asks: does this host
// respond right now? For a remote node that is a key-only ssh round trip — the
// same thing every other operation depends on, so a node reported online is one
// stacker can actually drive.
func (s *Service) probe(ctx context.Context, item Node) health {
	now := time.Now()

	// The machine stacker itself runs on is reachable by definition.
	if item.Local {
		return health{State: ReachabilityOnline, At: now, Message: "This is the machine stacker runs on"}
	}

	keyPath, err := s.keys.PrivateKeyPath(item.SshKeyID)
	if err != nil {
		return health{State: ReachabilityOffline, At: now, Message: "the ssh key for this node is missing"}
	}

	ctx, cancel := context.WithTimeout(ctx, pingConnectTimeout+sshCommandTimeout)
	defer cancel()

	result := probeHost(ctx, keyPath, item.Ssh, item.Port)
	if result.OK {
		return health{State: ReachabilityOnline, At: now, Message: "The host answered over ssh"}
	}
	return health{State: ReachabilityOffline, At: now, Message: strings.TrimSpace(result.Message)}
}
