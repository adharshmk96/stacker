package node

import (
	"strings"
	"testing"
)

func TestTerminalCommandLocal(t *testing.T) {
	svc := testService(t)

	argv, err := svc.terminalCommand(Node{ID: LocalID, Local: true})
	if err != nil {
		t.Fatalf("terminalCommand: %v", err)
	}
	if len(argv) != 2 || argv[1] != "-l" {
		t.Fatalf("argv = %v, want a login shell", argv)
	}
	if !strings.HasPrefix(argv[0], "/") {
		t.Fatalf("shell = %q, want an absolute path", argv[0])
	}
}

func TestTerminalCommandRemote(t *testing.T) {
	svc := NewService(NewRepository(testDB(t)), stubKeys{path: "/keys/id_ed25519"}, silentLog())

	argv, err := svc.terminalCommand(Node{
		Ssh:       "ubuntu@10.0.0.5",
		Port:      2222,
		SshKeyID:  "key-1",
		KeyStatus: KeyStatusOK,
	})
	if err != nil {
		t.Fatalf("terminalCommand: %v", err)
	}

	line := strings.Join(argv, " ")
	// -tt is what makes the remote side allocate a pty, which is the whole
	// point of the terminal.
	for _, want := range []string{"ssh", "-tt", "/keys/id_ed25519", "2222", "ubuntu@10.0.0.5"} {
		if !strings.Contains(line, want) {
			t.Errorf("argv = %q, want it to contain %q", line, want)
		}
	}
}

// A node whose key was never verified cannot be reached without a password,
// which a websocket cannot supply — so it is refused before anything starts.
func TestTerminalCommandUnverifiedKey(t *testing.T) {
	svc := testService(t)

	_, err := svc.terminalCommand(Node{Ssh: "ubuntu@10.0.0.5", SshKeyID: "key-1"})
	if err != ErrKeyNotVerified {
		t.Fatalf("err = %v, want %v", err, ErrKeyNotVerified)
	}
}

func TestStartTerminalUnknownNode(t *testing.T) {
	svc := testService(t)

	if _, _, err := svc.StartTerminal("missing", 80, 24); err != ErrNotFound {
		t.Fatalf("err = %v, want %v", err, ErrNotFound)
	}
}
