package project

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// Live status, as the project cards and the detail page poll it.
//
// Nothing here is stored. A deployment row says what stacker last did; this says
// what docker is running right now, which is the only honest answer to "is it
// up?" — a stack can be scaled by hand, lose a node, or crash-loop long after a
// successful deploy.

// dockerServiceRow is one line of `docker service ls --format {{json .}}`.
type dockerServiceRow struct {
	Name  string
	Mode  string
	Image string
	// Replicas is docker's `2/3` column. `docker service ls` is the one place
	// that reports it in a single call for every service of a stack.
	Replicas string
}

// statusReader reads stack state out of docker.
type statusReader struct {
	exec Exec
}

// Environment reads one environment's live state.
func (s *statusReader) Environment(ctx context.Context, env Environment, stack string, deploying bool) EnvironmentStatus {
	status := EnvironmentStatus{
		EnvironmentID: env.ID,
		Name:          env.Name,
		Stack:         stack,
		Services:      []ServiceState{},
		Domains:       hosts(env.Domains),
	}

	rows, err := s.services(ctx, stack)
	if err != nil {
		status.State = RuntimeUnknown
		status.Message = err.Error()
		return status
	}

	for _, row := range rows {
		running, desired := parseReplicas(row.Replicas)
		status.Services = append(status.Services, ServiceState{
			// Docker reports the stack-prefixed name; the compose service name
			// is what the user wrote and what the domains refer to.
			Name:    strings.TrimPrefix(row.Name, stack+"_"),
			Stack:   stack,
			Image:   shortImage(row.Image),
			Mode:    row.Mode,
			Running: running,
			Desired: desired,
		})
		status.Running += running
		status.Desired += desired
	}

	switch {
	case deploying:
		// A run in flight outranks the reading: mid-rollout a stack is expected
		// to be short of tasks, and calling that degraded would flash an alarm
		// on every deploy.
		status.State = RuntimeDeploying
	case len(status.Services) == 0:
		status.State = RuntimeStopped
	case status.Desired > 0 && status.Running == status.Desired:
		status.State = RuntimeRunning
	default:
		status.State = RuntimeDegraded
		status.Message = "some tasks are not running"
	}
	return status
}

// services lists the swarm services belonging to a stack.
//
// The stack-namespace label is the filter rather than a name prefix: the label is
// what docker itself uses to group a stack, and a name filter would also match an
// unrelated service that happens to start with the same characters.
func (s *statusReader) services(ctx context.Context, stack string) ([]dockerServiceRow, error) {
	var output strings.Builder
	cmd := Command{
		Name: "docker",
		Args: []string{
			"service", "ls",
			"--filter", "label=com.docker.stack.namespace=" + stack,
			"--format", "{{json .}}",
		},
		Env: os.Environ(),
	}

	err := s.exec(ctx, cmd, func(line string) {
		output.WriteString(line)
		output.WriteString("\n")
	})
	if err != nil {
		return nil, err
	}

	rows := []dockerServiceRow{}
	for line := range strings.SplitSeq(output.String(), "\n") {
		line = strings.TrimSpace(line)
		// Docker writes warnings on the same stream, so anything that is not an
		// object is skipped rather than failing the read.
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var row dockerServiceRow
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// parseReplicas reads docker's `running/desired` column. A global service shows
// the same shape, so nothing special is needed for it.
func parseReplicas(value string) (running, desired int) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	// The desired half can carry a suffix on a global service ("3/3 (max 1 …)").
	running, runErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	desired, desErr := strconv.Atoi(strings.TrimSpace(strings.Fields(parts[1])[0]))
	if runErr != nil || desErr != nil {
		return 0, 0
	}
	return running, desired
}

// shortImage drops the digest docker appends to a resolved image, which is 71
// characters of noise in a table cell.
func shortImage(value string) string {
	if index := strings.Index(value, "@sha256:"); index != -1 {
		return value[:index]
	}
	return value
}

// rollUp reduces an environment's states to the one a project card shows. The
// worst state wins: a card that says "running" while one environment is down
// would be the one thing worse than no card at all.
func rollUp(items []EnvironmentStatus) RuntimeState {
	if len(items) == 0 {
		return RuntimeStopped
	}

	ranked := map[RuntimeState]int{
		RuntimeRunning:   0,
		RuntimeStopped:   1,
		RuntimeDeploying: 2,
		RuntimeUnknown:   3,
		RuntimeDegraded:  4,
	}

	worst := items[0].State
	for _, item := range items[1:] {
		if ranked[item.State] > ranked[worst] {
			worst = item.State
		}
	}
	return worst
}

func hosts(domains []Domain) []string {
	out := make([]string, 0, len(domains))
	for _, domain := range domains {
		out = append(out, domain.Host)
	}
	return out
}
