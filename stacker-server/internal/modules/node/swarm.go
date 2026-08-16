package node

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// swarmCommandTimeout is generous compared with the ssh probe: `docker swarm
// join` has to reach the manager's raft port and settle before it returns.
const swarmCommandTimeout = 90 * time.Second

// swarmState is what `docker info` reports about the host itself. It is the
// source of truth — the database only ever mirrors it.
type swarmState struct {
	// LocalNodeState is docker's own word: inactive, pending, active, error or
	// locked.
	LocalNodeState string
	NodeID         string
	// ControlAvailable is true on a manager.
	ControlAvailable bool
}

// Active reports whether the host is part of a working swarm.
func (s swarmState) Active() bool { return s.LocalNodeState == "active" }

// Role maps the docker state onto the role stored on the node.
func (s swarmState) Role() SwarmRole {
	switch {
	case !s.Active():
		return SwarmRoleNone
	case s.ControlAvailable:
		return SwarmRoleManager
	default:
		return SwarmRoleWorker
	}
}

// runner executes a command on a node — directly when it is the machine stacker
// runs on, over ssh otherwise. Every docker call in this module goes through it,
// so local and remote nodes are driven by exactly the same code.
type runner struct {
	keys keys
}

// run executes argv on the node under the default timeout.
func (r runner) run(ctx context.Context, item Node, argv ...string) (string, error) {
	return r.runFor(ctx, item, swarmCommandTimeout, argv...)
}

// shell runs a shell snippet on the node. It exists for the few commands that
// are genuinely shell — pipelines like the docker install script — where argv
// alone cannot express what has to happen.
func (r runner) shell(ctx context.Context, item Node, timeout time.Duration, script string) (string, error) {
	return r.runFor(ctx, item, timeout, "sh", "-c", script)
}

// runFor executes argv on the node and returns its combined output. A non-zero
// exit is returned as an error carrying the most useful line of that output,
// because docker's failure messages are what the user needs to see.
//
// The timeout is per call: probing takes seconds, while installing docker can
// take minutes, and one limit cannot serve both.
func (r runner) runFor(ctx context.Context, item Node, timeout time.Duration, argv ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if item.Local {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	} else {
		keyPath, err := r.keys.PrivateKeyPath(item.SshKeyID)
		if err != nil {
			return "", ErrSshKeyMissing
		}

		args := []string{
			"-i", keyPath,
			"-p", strconv.Itoa(portOrDefault(item.Port)),
			"-o", "BatchMode=yes",
			"-o", "IdentitiesOnly=yes",
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", fmt.Sprintf("ConnectTimeout=%d", int(sshConnectTimeout.Seconds())),
			item.Ssh,
			// The remote side runs a shell, so every argument is quoted rather
			// than passed through as-is.
			shellJoin(argv),
		}
		cmd = exec.CommandContext(ctx, "ssh", args...)
	}

	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() != nil {
			return text, fmt.Errorf("%w: %s", ErrSwarmUnreachable, "the command timed out")
		}
		// Wrapped so the handler can tell a command that ran and failed from a
		// bug, and answer with docker's own words instead of a bare 500.
		return text, fmt.Errorf("%w: %s", ErrSwarmCommand, firstLine(text, err))
	}
	return text, nil
}

// docker runs a docker subcommand on the node, translating "docker is not
// installed" and "the daemon is not running" into errors the API can explain.
//
// A user added to the `docker` group only picks up that membership on their
// next login, so a freshly provisioned node refuses the socket over the ssh
// session that provisioned it. Rather than force a reconnect, a permission
// failure is retried once through sudo.
func (r runner) docker(ctx context.Context, item Node, args ...string) (string, error) {
	out, err := r.run(ctx, item, append([]string{"docker"}, args...)...)
	if err == nil {
		return out, nil
	}

	// A command that never started produces no output at all — the reason is
	// only in the error — so both are examined.
	text := out + " " + err.Error()

	if isPermissionDenied(text) {
		if sudoOut, sudoErr := r.run(ctx, item, append([]string{"sudo", "-n", "docker"}, args...)...); sudoErr == nil {
			return sudoOut, nil
		}
	}

	switch {
	case isMissingBinary(text):
		return out, ErrDockerMissing
	case isDaemonDown(text):
		return out, ErrDockerNotRunning
	}
	return out, err
}

// dockerInput is docker() with a payload piped to the command's stdin. It
// exists for the commands that read their content that way — deploying a
// compose file, creating a secret — where passing the value as an argument
// would put it on the process list of whichever host runs it.
//
// The sudo retry docker() does is deliberately not repeated here: stdin has
// already been consumed by the first attempt, so a retry would send an empty
// payload. A node whose socket needs sudo fails with docker's own message.
func (r runner) dockerInput(ctx context.Context, item Node, stdin string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, swarmCommandTimeout)
	defer cancel()

	argv := append([]string{"docker"}, args...)

	var cmd *exec.Cmd
	if item.Local {
		cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	} else {
		keyPath, err := r.keys.PrivateKeyPath(item.SshKeyID)
		if err != nil {
			return "", ErrSshKeyMissing
		}
		cmd = exec.CommandContext(ctx, "ssh",
			"-i", keyPath,
			"-p", strconv.Itoa(portOrDefault(item.Port)),
			"-o", "BatchMode=yes",
			"-o", "IdentitiesOnly=yes",
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", fmt.Sprintf("ConnectTimeout=%d", int(sshConnectTimeout.Seconds())),
			item.Ssh,
			shellJoin(argv),
		)
	}
	cmd.Stdin = strings.NewReader(stdin)

	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if ctx.Err() != nil {
			return text, fmt.Errorf("%w: %s", ErrSwarmUnreachable, "the command timed out")
		}
		return text, fmt.Errorf("%w: %s", ErrSwarmCommand, firstLine(text, err))
	}
	return text, nil
}

// state reads the node's current swarm membership straight from docker.
func (r runner) state(ctx context.Context, item Node) (swarmState, error) {
	const format = "{{.Swarm.LocalNodeState}}\t{{.Swarm.NodeID}}\t{{.Swarm.ControlAvailable}}"

	out, err := r.docker(ctx, item, "info", "--format", format)
	if err != nil {
		return swarmState{}, err
	}

	// `docker info` can print warnings before the formatted line, so take the
	// last non-empty line rather than the whole output.
	fields := strings.Split(lastLine(out), "\t")
	if len(fields) < 3 {
		return swarmState{}, fmt.Errorf("could not read the swarm state from docker: %q", out)
	}

	return swarmState{
		LocalNodeState:   strings.TrimSpace(fields[0]),
		NodeID:           strings.TrimSpace(fields[1]),
		ControlAvailable: strings.TrimSpace(fields[2]) == "true",
	}, nil
}

// joinToken fetches the worker or manager token from a manager. Tokens are
// never stored: they are read at join time and dropped with the request.
func (r runner) joinToken(ctx context.Context, manager Node, role SwarmRole) (string, error) {
	out, err := r.docker(ctx, manager, "swarm", "join-token", "-q", string(role))
	if err != nil {
		return "", err
	}
	token := lastLine(out)
	if token == "" {
		return "", fmt.Errorf("the manager returned an empty join token")
	}
	return token, nil
}

// isMissingBinary spots a docker that is not installed on the host — the most
// common reason configuring a node fails. The wording differs by source: a
// remote shell says "command not found", while a local run fails in Go's own
// exec lookup before any shell is involved.
func isMissingBinary(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "docker: not found") ||
		strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "no such file or directory") && strings.Contains(lower, "docker")
}

// isDaemonDown spots a docker CLI that cannot reach its own daemon, including
// the permission error a user outside the `docker` group gets.
func isDaemonDown(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "cannot connect to the docker daemon") ||
		strings.Contains(lower, "is the docker daemon running") ||
		isPermissionDenied(lower)
}

// isPermissionDenied spots the socket being refused to a user who is not (yet)
// in the docker group — the one failure worth retrying with sudo.
func isPermissionDenied(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "permission denied while trying to connect") ||
		strings.Contains(lower, "got permission denied")
}

// shellJoin renders argv as a single string safe to hand to a remote shell.
func shellJoin(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

// shellQuote wraps a value in single quotes, which the shell takes literally,
// and escapes any single quote inside it.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// joinEndpoint is the `host:2377` a worker joins a manager against.
func joinEndpoint(manager Node) string {
	return net.JoinHostPort(manager.SwarmAddr, strconv.Itoa(SwarmPort))
}
