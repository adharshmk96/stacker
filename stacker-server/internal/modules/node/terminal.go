package node

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/creack/pty"
)

// A terminal session is a real interactive shell on the node, running under a
// pty so full-screen programs (top, vim, less) behave the way they would in a
// normal terminal. It is the same connection model as everything else in this
// module: the local node runs a shell directly, a remote node runs one over
// ssh with the node's key.
//
// Nothing about the session is stored. It lives for as long as the websocket
// that opened it, and dies with it.

// TerminalSession is one running shell and the pty it is attached to. Read and
// Write move bytes to and from the shell; Resize follows the browser window.
type TerminalSession struct {
	pty *os.File
	cmd *exec.Cmd
}

func (t *TerminalSession) Read(p []byte) (int, error)  { return t.pty.Read(p) }
func (t *TerminalSession) Write(p []byte) (int, error) { return t.pty.Write(p) }

// Resize tells the shell how big its window is, so curses programs redraw at
// the right size instead of assuming 80x24.
func (t *TerminalSession) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return nil
	}
	return pty.Setsize(t.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

// Close ends the session: the pty is closed first so the shell sees EOF, then
// the process is killed in case it ignored that, and finally reaped.
func (t *TerminalSession) Close() {
	_ = t.pty.Close()
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	_ = t.cmd.Wait()
}

// Wait blocks until the shell exits and returns how it ended.
func (t *TerminalSession) Wait() error { return t.cmd.Wait() }

// StartTerminal opens an interactive shell on the node.
//
// A remote node is reached with the same key-only ssh the rest of the module
// uses, so a node whose key works everywhere else works here too — and one
// whose key was never installed is refused up front rather than dropping the
// user into a password prompt they cannot answer over a websocket.
func (s *Service) StartTerminal(id string, cols, rows uint16) (*TerminalSession, Node, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return nil, Node{}, err
	}

	argv, err := s.terminalCommand(item)
	if err != nil {
		return nil, Node{}, err
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	// TERM is what the shell and everything it runs use to pick their escape
	// sequences; xterm is what the browser side actually emulates.
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, Node{}, fmt.Errorf("%w: %s", ErrTerminalStart, err)
	}

	s.log.Info("terminal session opened", "node", item.ID, "name", item.Name)
	return &TerminalSession{pty: file, cmd: cmd}, item, nil
}

// terminalCommand is the argv for the node's shell: the login shell directly on
// the local machine, ssh with a forced pty on a remote one.
func (s *Service) terminalCommand(item Node) ([]string, error) {
	if item.Local {
		return []string{localShell(), "-l"}, nil
	}

	if item.KeyStatus != KeyStatusOK {
		return nil, ErrKeyNotVerified
	}

	keyPath, err := s.keys.PrivateKeyPath(item.SshKeyID)
	if err != nil {
		return nil, ErrSshKeyMissing
	}

	return []string{
		"ssh",
		"-tt",
		"-i", keyPath,
		"-p", strconv.Itoa(portOrDefault(item.Port)),
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(sshConnectTimeout.Seconds())),
		item.Ssh,
	}, nil
}

// localShell picks the shell to open on this machine: the user's own if the
// environment names one, otherwise the two that are always there.
func localShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		if _, err := os.Stat(shell); err == nil {
			return shell
		}
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}
