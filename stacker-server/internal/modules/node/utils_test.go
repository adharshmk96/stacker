package node

import (
	"testing"
)

func TestValidateSsh(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"deploy@host", false},
		{"user.name@10.0.0.2", false},
		{"  deploy@host  ", false},
		{"", true},
		{"host", true},
		{"user@", true},
		{"@host", true},
		{"user@host:22", true},
		{"user@host name", true},
	}
	for _, tc := range tests {
		err := validateSsh(tc.in)
		if tc.wantErr && err != ErrInvalidSsh {
			t.Errorf("validateSsh(%q) = %v, want ErrInvalidSsh", tc.in, err)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateSsh(%q) = %v, want nil", tc.in, err)
		}
	}
}

func TestSplitSsh(t *testing.T) {
	user, host := splitSsh("deploy@10.0.0.2")
	if user != "deploy" || host != "10.0.0.2" {
		t.Errorf("splitSsh = %q %q", user, host)
	}
	user, host = splitSsh("no-at")
	if user != "" || host != "" {
		t.Errorf("splitSsh malformed = %q %q", user, host)
	}
	user, host = splitSsh(" a@b ")
	if user != "a" || host != "b" {
		t.Errorf("splitSsh trimmed = %q %q", user, host)
	}
}

func TestShellQuoteJoin(t *testing.T) {
	if got := shellQuote("plain"); got != "'plain'" {
		t.Errorf("shellQuote = %q", got)
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("shellQuote apostrophe = %q", got)
	}
	if got := shellJoin([]string{"docker", "info"}); got != "'docker' 'info'" {
		t.Errorf("shellJoin = %q", got)
	}
}

func TestLastLine(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"one", "one"},
		{"a\nb\n", "b"},
		{"warn\n\nactive\tid\ttrue\n\n", "active\tid\ttrue"},
		{"   \n  ", ""},
	}
	for _, tc := range tests {
		if got := lastLine(tc.in); got != tc.want {
			t.Errorf("lastLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSwarmStateRole(t *testing.T) {
	tests := []struct {
		s    swarmState
		want SwarmRole
	}{
		{swarmState{LocalNodeState: "inactive"}, SwarmRoleNone},
		{swarmState{LocalNodeState: "pending"}, SwarmRoleNone},
		{swarmState{LocalNodeState: "active", ControlAvailable: true}, SwarmRoleManager},
		{swarmState{LocalNodeState: "active", ControlAvailable: false}, SwarmRoleWorker},
	}
	for _, tc := range tests {
		if got := tc.s.Role(); got != tc.want {
			t.Errorf("Role(%+v) = %q, want %q", tc.s, got, tc.want)
		}
	}
	if !(swarmState{LocalNodeState: "active"}).Active() {
		t.Error("expected active")
	}
}

func TestFirstLine(t *testing.T) {
	fallback := errString("fallback")
	if got := firstLine("  \n hello \n world", fallback); got != "hello" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("  \n", fallback); got != "fallback" {
		t.Errorf("firstLine empty = %q", got)
	}
}

func TestPortOrDefault(t *testing.T) {
	if portOrDefault(0) != 22 {
		t.Fatal("zero should become 22")
	}
	if portOrDefault(2222) != 2222 {
		t.Fatal("explicit port kept")
	}
}

func TestJoinEndpoint(t *testing.T) {
	got := joinEndpoint(Node{SwarmAddr: "10.0.0.1"})
	if got != "10.0.0.1:2377" {
		t.Errorf("joinEndpoint = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("ab", 5); got != "ab" {
		t.Errorf("short = %q", got)
	}
	got := truncate("abcdefghij", 5)
	if got != "abcd…" {
		t.Errorf("truncated = %q", got)
	}
}

func TestMissingAndDaemonDetectors(t *testing.T) {
	if !isMissingBinary("bash: docker: command not found") {
		t.Error("expected missing binary")
	}
	if !isMissingBinary("executable file not found in $PATH") {
		t.Error("expected exec lookup miss")
	}
	if !isMissingBinary("fork/exec docker: no such file or directory") {
		t.Error("expected no such file with docker")
	}
	if isMissingBinary("permission denied") {
		t.Error("did not expect missing binary")
	}
	if !isDaemonDown("Cannot connect to the Docker daemon at unix:///var/run/docker.sock") {
		t.Error("expected daemon down")
	}
	if !isDaemonDown("Is the docker daemon running?") {
		t.Error("expected daemon-running hint")
	}
	if !isPermissionDenied("Got permission denied while trying to connect to the Docker daemon socket") {
		t.Error("expected permission denied")
	}
	if !isDaemonDown("got permission denied") {
		t.Error("permission denied is a form of daemon down")
	}
}

func TestSwarmSummary(t *testing.T) {
	if swarmSummary(Node{SwarmRole: SwarmRoleManager}) != "This node is a swarm manager" {
		t.Fatal("manager summary")
	}
	if swarmSummary(Node{SwarmRole: SwarmRoleWorker}) != "This node is a swarm worker" {
		t.Fatal("worker summary")
	}
	if swarmSummary(Node{SwarmRole: SwarmRoleNone}) != "This node is not part of the swarm" {
		t.Fatal("none summary")
	}
}

func TestNodeInSwarm(t *testing.T) {
	if (Node{SwarmRole: SwarmRoleNone}).InSwarm() {
		t.Fatal("none should not be in swarm")
	}
	if !(Node{SwarmRole: SwarmRoleWorker}).InSwarm() {
		t.Fatal("worker should be in swarm")
	}
}

func TestNewIDAndLocalName(t *testing.T) {
	a, b := newID(), newID()
	if a == "" || a == b {
		t.Fatalf("newID = %q %q", a, b)
	}
	if len(a) != 24 {
		t.Errorf("newID length = %d", len(a))
	}
	if name := localName(); name == "" {
		t.Fatal("localName empty")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestPrivilegedPrefix(t *testing.T) {
	p := &provisionCtx{}
	if got := p.privileged("echo hi"); got != "echo hi" {
		t.Errorf("root privileged = %q", got)
	}
	p.sudo = "sudo -n"
	got := p.privileged("echo hi")
	if got != "sudo -n sh -c 'echo hi'" {
		t.Errorf("sudo privileged = %q", got)
	}
}
