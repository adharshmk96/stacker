package node

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// joinWorker joins a remote node to the existing manager as a worker.
func (s *Service) joinWorker(ctx context.Context, item Node) (SwarmResult, error) {
	// A node stacker cannot log into cannot be configured, and the ssh failure
	// would otherwise surface as a confusing docker error.
	if item.KeyStatus != KeyStatusOK {
		return SwarmResult{}, ErrKeyNotVerified
	}

	manager, err := s.repo.Manager()
	if err != nil {
		return SwarmResult{}, err
	}
	if manager.ID == item.ID {
		return SwarmResult{}, ErrAlreadyInSwarm
	}

	// The manager has to be healthy before the worker is touched: a join
	// against a dead control plane leaves the worker in `pending`.
	managerState, err := s.rt.state(ctx, manager)
	if err != nil {
		return SwarmResult{}, fmt.Errorf("%w: %s", ErrManagerUnhealthy, err)
	}
	if managerState.Role() != SwarmRoleManager {
		// The manager left the swarm behind stacker's back; record that so the
		// UI stops offering joins against it.
		if _, err := s.applyState(manager, managerState, manager.SwarmAddr); err != nil {
			s.log.Error("could not record the manager's state", "error", err)
		}
		return SwarmResult{}, ErrManagerUnhealthy
	}

	endpoint, err := s.managerEndpoint(manager)
	if err != nil {
		return SwarmResult{}, err
	}

	state, err := s.rt.state(ctx, item)
	if err != nil {
		return SwarmResult{}, s.recordSwarmFailure(item, err)
	}

	// A node that is already in a swarm is only ours if the manager knows its
	// id. Joining a foreign swarm's node would silently do nothing useful.
	if state.Active() {
		known, err := s.managerKnows(ctx, manager, state.NodeID)
		if err != nil {
			return SwarmResult{}, err
		}
		if !known {
			return SwarmResult{}, ErrForeignSwarm
		}

		item, err = s.applyState(item, state, "")
		if err != nil {
			return SwarmResult{}, err
		}
		return SwarmResult{Node: item, Message: "This node was already part of the swarm"}, nil
	}

	token, err := s.rt.joinToken(ctx, manager, SwarmRoleWorker)
	if err != nil {
		return SwarmResult{}, fmt.Errorf("%w: %s", ErrManagerUnhealthy, err)
	}

	if _, err := s.rt.docker(ctx, item, "swarm", "join", "--token", token, endpoint); err != nil {
		return SwarmResult{}, s.recordSwarmFailure(item, err)
	}

	state, err = s.rt.state(ctx, item)
	if err != nil {
		return SwarmResult{}, s.recordSwarmFailure(item, err)
	}

	item, err = s.applyState(item, state, "")
	if err != nil {
		return SwarmResult{}, err
	}

	s.log.Info("node joined the swarm", "node", item.Name, "manager", manager.Name)
	return SwarmResult{Node: item, Message: fmt.Sprintf("Joined the swarm as a worker via %s", manager.Name)}, nil
}

// SyncLocalSwarm mirrors the swarm manager prepared by install.sh. It never
// mutates Docker; infrastructure setup is deliberately outside the app.
func (s *Service) SyncLocalSwarm(advertiseAddr string) {
	ctx, cancel := context.WithTimeout(context.Background(), swarmCommandTimeout)
	defer cancel()

	item, err := s.repo.Get(LocalID)
	if err != nil {
		s.log.Warn("could not find the installed node", "error", err)
		return
	}
	state, err := s.rt.state(ctx, item)
	if err != nil {
		s.recordSwarmFailure(item, err) //nolint:errcheck // best-effort startup sync
		s.log.Warn("could not read the installed swarm manager", "error", err)
		return
	}
	item, err = s.applyState(item, state, advertiseAddr)
	if err != nil {
		s.log.Warn("could not record the installed swarm manager", "error", err)
		return
	}
	if item.SwarmRole != SwarmRoleManager {
		s.log.Error("the installed node is not a swarm manager; rerun install.sh")
	}
}

// Promote turns a worker into a manager. Docker is told from an existing
// manager — the worker itself has no control plane to run the command on.
func (s *Service) Promote(ctx context.Context, id string) (SwarmResult, error) {
	return s.changeRole(ctx, id, SwarmRoleManager)
}

// Demote turns a manager back into a worker, refusing when it is the last one.
func (s *Service) Demote(ctx context.Context, id string) (SwarmResult, error) {
	return s.changeRole(ctx, id, SwarmRoleWorker)
}

func (s *Service) changeRole(ctx context.Context, id string, target SwarmRole) (SwarmResult, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return SwarmResult{}, err
	}

	unlock, err := s.lock(id)
	if err != nil {
		return SwarmResult{}, err
	}
	defer unlock()

	switch {
	case item.SwarmRole == SwarmRoleNone:
		return SwarmResult{}, ErrNotInSwarm
	case target == SwarmRoleManager && item.SwarmRole != SwarmRoleWorker:
		return SwarmResult{}, ErrNotWorker
	case target == SwarmRoleWorker && item.SwarmRole != SwarmRoleManager:
		return SwarmResult{}, ErrNotManagerRole
	}

	// Demoting the only manager would leave the swarm with no control plane and
	// no way back — docker itself refuses, but failing here is clearer.
	if target == SwarmRoleWorker {
		if err := s.assertNotLastManager(); err != nil {
			return SwarmResult{}, err
		}
	}

	manager, err := s.controlNode(item)
	if err != nil {
		return SwarmResult{}, err
	}
	if item.SwarmNodeID == "" {
		return SwarmResult{}, fmt.Errorf("docker's id for this node is unknown — refresh it first")
	}

	verb := "promote"
	if target == SwarmRoleWorker {
		verb = "demote"
	}
	if _, err := s.rt.docker(ctx, manager, "node", verb, item.SwarmNodeID); err != nil {
		return SwarmResult{}, s.recordSwarmFailure(item, err)
	}

	// The role change is asynchronous: docker returns as soon as raft accepts
	// it, seconds before the node itself reports the new role. Reading once
	// here would record the old role and leave the UI wrong until a manual
	// refresh, so the node is polled until its own view agrees.
	item, err = s.awaitRole(ctx, item, target)
	if err != nil {
		return SwarmResult{}, err
	}

	s.log.Info("swarm role changed", "node", item.Name, "role", item.SwarmRole)

	message := fmt.Sprintf("%s is now a %s", item.Name, item.SwarmRole)
	if warning := s.quorumWarning(ctx, manager); warning != "" {
		message += ". " + warning
	}
	return SwarmResult{Node: item, Message: message}, nil
}

// quorumWarning reports raft trouble straight after a role change.
//
// Managers form a raft quorum, so a manager that the others cannot reach on
// port 2377 does not just fail to help — it takes the quorum with it, and an
// even number of managers loses the swarm the moment one drops. Docker accepts
// the promotion regardless, and the damage only shows up later, so the control
// plane is asked whether it still has a leader while the user is still here to
// read the answer.
func (s *Service) quorumWarning(ctx context.Context, manager Node) string {
	if _, err := s.rt.docker(ctx, manager, "node", "ls", "--format", "{{.ID}}"); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "leader") {
			return "Warning: the swarm has lost its leader — the new manager is probably not reachable on port 2377 from the others. " +
				"Recover with `docker swarm init --force-new-cluster` on a manager."
		}
		return "Warning: the swarm could not be queried after the change: " + err.Error()
	}

	count, err := s.repo.CountByRole(SwarmRoleManager)
	if err == nil && count > 1 && count%2 == 0 {
		return fmt.Sprintf("Note: %d managers is an even number — raft needs a majority, so keep an odd number (1, 3 or 5).", count)
	}
	return ""
}

// Leave takes a node out of the swarm: the node leaves, then the manager drops
// the stale entry so `docker node ls` does not keep showing it as down.
func (s *Service) Leave(ctx context.Context, id string) (SwarmResult, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return SwarmResult{}, err
	}

	unlock, err := s.lock(id)
	if err != nil {
		return SwarmResult{}, err
	}
	defer unlock()

	if item.SwarmRole == SwarmRoleNone {
		return SwarmResult{}, ErrNotInSwarm
	}
	if item.SwarmRole == SwarmRoleManager {
		if err := s.assertNotLastManager(); err != nil {
			return SwarmResult{}, err
		}
	}

	// --force is required for a manager, and harmless for a worker whose tasks
	// we are deliberately abandoning.
	if _, err := s.rt.docker(ctx, item, "swarm", "leave", "--force"); err != nil {
		// A node already out of the swarm is the state we wanted, so treat it
		// as success rather than blocking the cleanup below.
		if !strings.Contains(strings.ToLower(err.Error()), "not part of a swarm") {
			return SwarmResult{}, s.recordSwarmFailure(item, err)
		}
	}

	// Best effort: the swarm is still usable with a down entry in it, and the
	// node has already left, so a failure here must not fail the call.
	if manager, mErr := s.repo.Manager(); mErr == nil && manager.ID != item.ID && item.SwarmNodeID != "" {
		if _, err := s.rt.docker(ctx, manager, "node", "rm", "--force", item.SwarmNodeID); err != nil {
			s.log.Warn("could not remove the node from the manager", "node", item.Name, "error", err)
		}
	}

	now := time.Now()
	item.SwarmRole = SwarmRoleNone
	item.SwarmNodeID = ""
	item.SwarmAddr = ""
	item.SwarmError = ""
	item.SwarmSyncedAt = &now
	if err := s.repo.Save(&item); err != nil {
		return SwarmResult{}, err
	}

	s.log.Info("node left the swarm", "node", item.Name)
	return SwarmResult{Node: item, Message: fmt.Sprintf("%s left the swarm", item.Name)}, nil
}

// RefreshSwarm re-reads one node's swarm state from docker. It is what the UI
// calls to reconcile changes made outside stacker.
func (s *Service) RefreshSwarm(ctx context.Context, id string) (SwarmResult, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return SwarmResult{}, err
	}

	unlock, err := s.lock(id)
	if err != nil {
		return SwarmResult{}, err
	}
	defer unlock()

	item, err = s.refreshFrom(ctx, item)
	if err != nil {
		return SwarmResult{Node: item}, err
	}
	return SwarmResult{Node: item, Message: swarmSummary(item)}, nil
}

// RefreshAll re-reads the swarm state of every configured node at once.
//
// It exists because a node's role is only ever what stacker last saw, and the
// host can change underneath it: docker removed, a rebuilt machine, someone
// running `docker swarm leave` by hand. The page calls this on load, so what
// the table shows is what docker says now rather than what it said the last
// time someone clicked something.
//
// Nodes are read in parallel — every reading is an ssh round trip — but written
// one at a time, because sqlite takes one writer and a sweep is not worth
// contending over. A node that cannot be read is not an error for the sweep:
// its reason is recorded on the row, which is the whole point of running it.
func (s *Service) RefreshAll(ctx context.Context) ([]Node, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	type reading struct {
		state swarmState
		err   error
	}

	readings := make([]*reading, len(items))
	unlocks := make([]func(), len(items))
	var wg sync.WaitGroup

	for i := range items {
		// A node that has never been configured has no swarm state to read,
		// and no docker worth reaching for.
		if items[i].SwarmRole == SwarmRoleNone {
			continue
		}
		// A node mid-configure is being written by that run; reading it here
		// would only race to store an answer that is about to change.
		unlock, err := s.lock(items[i].ID)
		if err != nil {
			continue
		}
		unlocks[i] = unlock

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			state, err := s.rt.state(ctx, items[i])
			readings[i] = &reading{state: state, err: err}
		}(i)
	}
	wg.Wait()

	for i := range items {
		if unlocks[i] != nil {
			defer unlocks[i]()
		}
		if readings[i] == nil {
			continue
		}

		if readings[i].err != nil {
			s.recordSwarmFailure(items[i], readings[i].err) //nolint:errcheck // stored on the row
			continue
		}
		if _, err := s.applyState(items[i], readings[i].state, items[i].SwarmAddr); err != nil {
			s.log.Error("could not record the swarm state", "node", items[i].Name, "error", err)
		}
	}

	// Re-read rather than patch the slice in place: both branches above wrote
	// through the repository, so this is the one copy that is certainly current.
	return s.List()
}

// roleSettleTimeout bounds how long a promote or demote waits for the node to
// report its new role. Raft normally agrees in a second or two; the ceiling is
// there so a node that never converges does not hang the request.
const (
	roleSettleTimeout  = 30 * time.Second
	roleSettleInterval = 2 * time.Second
)

// awaitRole polls the node until it reports the role it was just given, and
// stores whatever it last saw. A node that never converges is not an error:
// the change was accepted by the swarm, so the recorded state is simply the
// most recent reading, and a later refresh will catch up.
func (s *Service) awaitRole(ctx context.Context, item Node, target SwarmRole) (Node, error) {
	deadline := time.Now().Add(roleSettleTimeout)

	for {
		state, err := s.rt.state(ctx, item)
		if err != nil {
			return item, s.recordSwarmFailure(item, err)
		}
		if state.Role() == target || time.Now().After(deadline) {
			return s.applyState(item, state, item.SwarmAddr)
		}

		select {
		case <-ctx.Done():
			return s.applyState(item, state, item.SwarmAddr)
		case <-time.After(roleSettleInterval):
		}
	}
}

// refreshFrom reads the node's state from docker and stores it.
func (s *Service) refreshFrom(ctx context.Context, item Node) (Node, error) {
	state, err := s.rt.state(ctx, item)
	if err != nil {
		return item, s.recordSwarmFailure(item, err)
	}
	return s.applyState(item, state, item.SwarmAddr)
}

// applyState writes an observed docker state onto the node. addr is only kept
// while the node is a manager: a worker has no advertise address worth storing,
// and a stale one would be offered to the next node that joins.
func (s *Service) applyState(item Node, state swarmState, addr string) (Node, error) {
	now := time.Now()
	item.SwarmRole = state.Role()
	item.SwarmNodeID = state.NodeID
	item.SwarmError = ""
	item.SwarmSyncedAt = &now

	if item.SwarmRole == SwarmRoleManager {
		item.SwarmAddr = addr
	} else {
		item.SwarmAddr = ""
	}

	if err := s.repo.Save(&item); err != nil {
		return item, err
	}
	return item, nil
}

// recordSwarmFailure stores why the last attempt failed and returns the error
// unchanged, so the row can explain itself without the user re-running anything.
func (s *Service) recordSwarmFailure(item Node, cause error) error {
	now := time.Now()
	item.SwarmError = truncate(cause.Error(), 500)
	item.SwarmSyncedAt = &now
	if err := s.repo.Save(&item); err != nil {
		s.log.Error("could not record the swarm error", "node", item.Name, "error", err)
	}
	return cause
}

// managerEndpoint is the `host:2377` workers join against. install.sh always
// provides this address; a missing value means the install must be reconciled.
func (s *Service) managerEndpoint(manager Node) (string, error) {
	if manager.SwarmAddr != "" {
		return joinEndpoint(manager), nil
	}

	return "", ErrAdvertiseAddr
}

// controlNode picks a manager that can run `docker node` commands for the given
// node. A manager administers itself; anything else needs another manager.
func (s *Service) controlNode(item Node) (Node, error) {
	manager, err := s.repo.Manager()
	if err != nil {
		return Node{}, err
	}
	if manager.ID != item.ID {
		return manager, nil
	}
	if item.SwarmRole == SwarmRoleManager {
		return item, nil
	}
	return Node{}, ErrNoManager
}

// managerKnows reports whether the swarm's manager has this docker node id in
// its roster — the test for "already in *our* swarm" versus someone else's.
func (s *Service) managerKnows(ctx context.Context, manager Node, nodeID string) (bool, error) {
	if nodeID == "" {
		return false, nil
	}

	out, err := s.rt.docker(ctx, manager, "node", "ls", "--format", "{{.ID}}")
	if err != nil {
		return false, fmt.Errorf("%w: %s", ErrManagerUnhealthy, err)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == nodeID {
			return true, nil
		}
	}
	return false, nil
}

// assertNotLastManager refuses a change that would leave the swarm with no
// control plane.
func (s *Service) assertNotLastManager() error {
	count, err := s.repo.CountByRole(SwarmRoleManager)
	if err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastManager
	}
	return nil
}

// lock serialises swarm operations per node. Two joins racing on the same host
// would each read "not in a swarm" and both run `docker swarm join`.
func (s *Service) lock(id string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.busy[id] {
		return nil, ErrSwarmBusy
	}
	s.busy[id] = true

	return func() {
		s.mu.Lock()
		delete(s.busy, id)
		s.mu.Unlock()
	}, nil
}

func swarmSummary(item Node) string {
	switch item.SwarmRole {
	case SwarmRoleManager:
		return "This node is a swarm manager"
	case SwarmRoleWorker:
		return "This node is a swarm worker"
	default:
		return "This node is not part of the swarm"
	}
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

// inOurSwarm reports whether the node belongs to the swarm stacker administers.
// Delete uses it to decide whether forgetting the node would strand it.
//
// Being in *a* swarm is not enough: a node can be active in a swarm that has
// nothing to do with this one — the same case Configure rejects with
// ErrForeignSwarm — and stacker has no business keeping that row alive. The
// test is therefore the manager's roster, not the node's own docker state.
func (s *Service) inOurSwarm(ctx context.Context, item Node) bool {
	state, err := s.rt.state(ctx, item)
	if err != nil {
		// An unreachable node falls back to what was last recorded, so an
		// offline host does not silently look like it left the swarm.
		return item.SwarmRole != SwarmRoleNone
	}
	if !state.Active() {
		return false
	}

	manager, err := s.repo.Manager()
	if err != nil {
		// With no manager of our own there is no swarm to be stranded in.
		return false
	}
	// A node that is our manager is in our swarm by definition, and asking it
	// about its own roster would be circular.
	if manager.ID == item.ID {
		return true
	}

	known, err := s.managerKnows(ctx, manager, state.NodeID)
	if err != nil {
		// The manager could not be asked. Fall back to the recorded role rather
		// than assume either way.
		return item.SwarmRole != SwarmRoleNone
	}
	return known
}
