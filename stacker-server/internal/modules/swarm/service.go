package swarm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"stacker/internal/modules/node"
)

// nodes is the slice of the node module this one depends on: the roster of
// hosts, and the ability to run a docker command on any of them. Declared here,
// on the consumer side, so the dependency stays one-way — swarm knows about
// nodes, nodes knows nothing about swarm.
type nodes interface {
	List() ([]node.Node, error)
	Manager() (node.Node, error)
	Docker(ctx context.Context, item node.Node, args ...string) (string, error)
	DockerInput(ctx context.Context, item node.Node, stdin string, args ...string) (string, error)
}

// logTail is how much of a log a row action pulls back. Enough to see why
// something failed, small enough to hand to a modal.
const logTail = "200"

// Service reads and mutates docker resources across the swarm.
//
// It stores nothing: docker is the only source of truth for what is deployed,
// and a cached copy would just be a second answer to disagree with.
type Service struct {
	nodes nodes
	log   *slog.Logger
}

func NewService(nodes nodes, log *slog.Logger) *Service {
	return &Service{nodes: nodes, log: log}
}

/* ---- listing ---- */

// List reads one resource. A swarm-scoped resource is asked of the manager; a
// node-scoped one is asked of every node in the swarm at once, and `nodeID`
// narrows that to a single host.
func (s *Service) List(ctx context.Context, resource Resource, nodeID string) (ListResult, error) {
	spec, ok := specs[resource]
	if !ok {
		return ListResult{}, ErrUnknownResource
	}

	roster, err := s.roster()
	if err != nil {
		return ListResult{}, err
	}

	result := ListResult{
		Resource: resource,
		Scope:    spec.scope,
		Rows:     []Row{},
		Nodes:    refs(roster),
		Errors:   []NodeError{},
	}

	if spec.scope == ScopeSwarm {
		manager, err := s.manager()
		if err != nil {
			return ListResult{}, err
		}

		rows, err := s.listSwarm(ctx, manager, resource, spec, roster)
		if err != nil {
			// The manager failing is the whole list failing — there is no
			// second host to fall back to — but it is still reported as a node
			// error so the page can show the table frame and the reason.
			result.Errors = append(result.Errors, NodeError{Node: manager.Name, Message: cleanErr(err)})
			return result, nil
		}
		result.Rows = rows
		return result, nil
	}

	targets := roster
	if nodeID != "" {
		match, ok := find(roster, nodeID)
		if !ok {
			return ListResult{}, ErrUnknownNode
		}
		targets = []node.Node{match}
	}

	rows, failures := s.listNodes(ctx, targets, spec)
	result.Rows = rows
	result.Errors = failures
	return result, nil
}

// listSwarm runs a swarm-wide list on the manager.
func (s *Service) listSwarm(ctx context.Context, manager node.Node, resource Resource, spec listSpec, roster []node.Node) ([]Row, error) {
	if resource == Tasks {
		return s.listTasks(ctx, manager, roster)
	}

	out, err := s.nodes.Docker(ctx, manager, spec.argv...)
	if err != nil {
		return nil, err
	}
	return parseLines(out, spec), nil
}

// listTasks reads every task in the swarm.
//
// Docker has no "list all tasks" command — `service ps` wants services — so the
// service names are fetched first and passed in one call, which keeps this to
// two round trips however many services there are.
func (s *Service) listTasks(ctx context.Context, manager node.Node, roster []node.Node) ([]Row, error) {
	out, err := s.nodes.Docker(ctx, manager, "service", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}

	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	// No services means no tasks — and `service ps` with no arguments is an
	// error, not an empty list.
	if len(names) == 0 {
		return []Row{}, nil
	}

	argv := append([]string{"service", "ps", "--no-trunc", "--format", jsonLines}, names...)
	out, err = s.nodes.Docker(ctx, manager, argv...)
	if err != nil {
		return nil, err
	}

	rows := parseLines(out, listSpec{row: func(rec record) (Row, bool) { return taskRow(rec), true }})

	// Docker names the node by its own hostname, which need not be what the
	// node is called in stacker. Rewriting it to stacker's name (and adding the
	// id) makes a task's node mean the same thing as a container's, so the same
	// filter and the same "go to node" work on both.
	hosts := s.hostnames(ctx, manager, roster)
	for _, row := range rows {
		if match, ok := hosts[row["node"]]; ok {
			row["node"] = match.Name
			row["nodeId"] = match.ID
		}
	}
	return rows, nil
}

// hostnames maps each docker node hostname onto the stacker node it is, via
// docker's own node id — the one identifier both sides already agree on.
//
// A failure is not worth failing the list over: without the map the tasks
// simply keep the hostnames docker reported.
func (s *Service) hostnames(ctx context.Context, manager node.Node, roster []node.Node) map[string]node.Node {
	names := map[string]node.Node{}

	out, err := s.nodes.Docker(ctx, manager, "node", "ls", "--format", "{{.ID}}\t{{.Hostname}}")
	if err != nil {
		s.log.Warn("could not read the docker node roster", "error", err)
		return names
	}

	byDockerID := map[string]node.Node{}
	for _, item := range roster {
		if item.SwarmNodeID != "" {
			byDockerID[item.SwarmNodeID] = item
		}
	}

	for _, line := range strings.Split(out, "\n") {
		id, hostname, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found {
			continue
		}
		if match, ok := byDockerID[strings.TrimSpace(id)]; ok {
			names[strings.TrimSpace(hostname)] = match
		}
	}
	return names
}

// listNodes reads a node-scoped resource from every target at once.
//
// The calls run in parallel because they are ssh round trips: done in sequence,
// a handful of nodes would make the page feel broken, and one slow host would
// hold up every other. A host that fails contributes an error rather than
// failing the list — the rows from the nodes that answered are still worth
// showing.
func (s *Service) listNodes(ctx context.Context, targets []node.Node, spec listSpec) ([]Row, []NodeError) {
	type outcome struct {
		rows []Row
		err  *NodeError
	}

	results := make([]outcome, len(targets))
	var wg sync.WaitGroup

	for i, target := range targets {
		wg.Add(1)
		go func(i int, target node.Node) {
			defer wg.Done()

			out, err := s.nodes.Docker(ctx, target, spec.argv...)
			if err != nil {
				results[i] = outcome{err: &NodeError{Node: target.Name, Message: cleanErr(err)}}
				return
			}

			rows := parseLines(out, spec)
			// Every row is stamped with where it was read from: without it a
			// node-scoped list is a pile of names with no way to tell two
			// hosts' containers apart, or to act on either.
			for _, row := range rows {
				row["node"] = target.Name
				row["nodeId"] = target.ID
			}
			results[i] = outcome{rows: rows}
		}(i, target)
	}
	wg.Wait()

	rows := []Row{}
	failures := []NodeError{}
	for _, result := range results {
		rows = append(rows, result.rows...)
		if result.err != nil {
			failures = append(failures, *result.err)
		}
	}
	return rows, failures
}

/* ---- actions ---- */

// Action runs one row action. Which docker command that is comes from the
// resource's action table; this resolves the host, runs it and shapes the
// answer.
func (s *Service) Action(ctx context.Context, resource Resource, req ActionRequest) (ActionResult, error) {
	spec, ok := specs[resource]
	if !ok {
		return ActionResult{}, ErrUnknownResource
	}

	action, ok := actions[resource][req.Action]
	if !ok {
		return ActionResult{}, ErrUnknownAction
	}

	host, err := s.host(spec.scope, req.Node)
	if err != nil {
		return ActionResult{}, err
	}

	argv, err := action.build(req)
	if err != nil {
		return ActionResult{}, err
	}

	out, err := s.nodes.Docker(ctx, host, argv...)
	if err != nil {
		return ActionResult{}, err
	}

	s.log.Info("swarm action", "resource", resource, "action", req.Action, "id", req.ID, "node", host.Name)

	result := ActionResult{Message: action.message(req)}
	if action.reads {
		result.Output = out
		if strings.TrimSpace(out) == "" {
			result.Output = "(no output)"
		}
	}
	return result, nil
}

// Create runs the one create action a resource offers.
func (s *Service) Create(ctx context.Context, resource Resource, req CreateRequest) (ActionResult, error) {
	spec, ok := specs[resource]
	if !ok {
		return ActionResult{}, ErrUnknownResource
	}

	host, err := s.host(spec.scope, req.Node)
	if err != nil {
		return ActionResult{}, err
	}

	name := strings.TrimSpace(req.Name)

	switch resource {
	case Stacks:
		if name == "" {
			return ActionResult{}, ErrNameRequired
		}
		if strings.TrimSpace(req.Content) == "" {
			return ActionResult{}, ErrContentRequired
		}
		// The compose file goes in on stdin so it never has to be written to
		// disk on the manager, or quoted into a remote shell.
		if _, err := s.nodes.DockerInput(ctx, host, req.Content,
			"stack", "deploy", "--compose-file", "-", "--detach=true", name); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: fmt.Sprintf("Deployed the stack %s", name)}, nil

	case Services:
		if name == "" {
			return ActionResult{}, ErrNameRequired
		}
		image := strings.TrimSpace(req.Image)
		if image == "" {
			return ActionResult{}, ErrImageRequired
		}

		argv := []string{"service", "create", "--detach=true", "--name", name}
		if req.Replicas != nil {
			argv = append(argv, "--replicas", fmt.Sprint(*req.Replicas))
		}
		argv = append(argv, image)

		if _, err := s.nodes.Docker(ctx, host, argv...); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: fmt.Sprintf("Created the service %s", name)}, nil

	case Networks:
		if name == "" {
			return ActionResult{}, ErrNameRequired
		}
		driver := strings.TrimSpace(req.Driver)
		if driver == "" {
			// A network created from the Swarm menu is meant to be reachable by
			// services on any node, which is what overlay means.
			driver = "overlay"
		}
		if _, err := s.nodes.Docker(ctx, host, "network", "create", "--driver", driver, "--attachable", name); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: fmt.Sprintf("Created the %s network %s", driver, name)}, nil

	case Volumes:
		if name == "" {
			return ActionResult{}, ErrNameRequired
		}
		argv := []string{"volume", "create"}
		if driver := strings.TrimSpace(req.Driver); driver != "" {
			argv = append(argv, "--driver", driver)
		}
		argv = append(argv, name)

		if _, err := s.nodes.Docker(ctx, host, argv...); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: fmt.Sprintf("Created the volume %s on %s", name, host.Name)}, nil

	case Images:
		image := strings.TrimSpace(req.Image)
		if image == "" {
			return ActionResult{}, ErrImageRequired
		}
		if _, err := s.nodes.Docker(ctx, host, "image", "pull", image); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: fmt.Sprintf("Pulled %s onto %s", image, host.Name)}, nil

	case Secrets, Configs:
		if name == "" {
			return ActionResult{}, ErrNameRequired
		}
		if req.Content == "" {
			return ActionResult{}, ErrContentRequired
		}
		// `-` reads the value from stdin, which is the only way a secret's
		// content never appears in a process list or a shell history.
		kind := "secret"
		if resource == Configs {
			kind = "config"
		}
		if _, err := s.nodes.DockerInput(ctx, host, req.Content, kind, "create", name, "-"); err != nil {
			return ActionResult{}, err
		}
		return ActionResult{Message: fmt.Sprintf("Created the %s %s", kind, name)}, nil
	}

	return ActionResult{}, ErrUnknownAction
}

/* ---- hosts ---- */

// host resolves which node a call runs against: the manager for swarm-wide
// work, the named node for anything node-scoped.
func (s *Service) host(scope Scope, nodeID string) (node.Node, error) {
	if scope == ScopeSwarm {
		return s.manager()
	}
	if strings.TrimSpace(nodeID) == "" {
		return node.Node{}, ErrNodeRequired
	}

	roster, err := s.roster()
	if err != nil {
		return node.Node{}, err
	}
	match, ok := find(roster, nodeID)
	if !ok {
		return node.Node{}, ErrUnknownNode
	}
	return match, nil
}

// manager is the node holding the control plane. Its absence is the ordinary
// first-run state, not a failure, so it gets its own message pointing at Nodes.
func (s *Service) manager() (node.Node, error) {
	manager, err := s.nodes.Manager()
	if err != nil {
		return node.Node{}, ErrNoManager
	}
	return manager, nil
}

// roster is every node stacker has configured into the swarm — the only hosts
// with a docker daemon worth asking anything of.
func (s *Service) roster() ([]node.Node, error) {
	items, err := s.nodes.List()
	if err != nil {
		return nil, err
	}

	roster := []node.Node{}
	for _, item := range items {
		if item.InSwarm() {
			roster = append(roster, item)
		}
	}
	sort.Slice(roster, func(i, j int) bool { return roster[i].Name < roster[j].Name })
	return roster, nil
}

// find matches a node by stacker's id or by name. Rows carry both, and an
// action posted straight from a row should work either way.
func find(roster []node.Node, ref string) (node.Node, bool) {
	for _, item := range roster {
		if item.ID == ref || item.Name == ref {
			return item, true
		}
	}
	return node.Node{}, false
}

func refs(roster []node.Node) []NodeRef {
	items := make([]NodeRef, 0, len(roster))
	for _, item := range roster {
		items = append(items, NodeRef{
			ID:     item.ID,
			Name:   item.Name,
			Role:   string(item.SwarmRole),
			Online: item.Reachability != node.ReachabilityOffline,
		})
	}
	return items
}

// cleanErr unwraps the node module's sentinels down to the sentence worth
// showing next to a node's name — the wrapper already says "the docker command
// failed", which adds nothing beside docker's own line.
func cleanErr(err error) string {
	var target error
	switch {
	case errors.Is(err, node.ErrDockerMissing):
		target = node.ErrDockerMissing
	case errors.Is(err, node.ErrDockerNotRunning):
		target = node.ErrDockerNotRunning
	}
	if target != nil {
		return target.Error()
	}

	message := err.Error()
	if _, rest, found := strings.Cut(message, ": "); found {
		return rest
	}
	return message
}
