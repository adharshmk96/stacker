package node

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestRunForLocalRealExec(t *testing.T) {
	r := runner{}
	out, err := r.runFor(context.Background(), Node{Local: true}, 2*time.Second, "echo", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello" {
		t.Errorf("out = %q", out)
	}

	_, err = r.runFor(context.Background(), Node{Local: true}, time.Second, "false")
	if !errors.Is(err, ErrSwarmCommand) {
		t.Fatalf("false = %v", err)
	}

	_, err = r.runFor(context.Background(), Node{Local: true}, 50*time.Millisecond, "sleep", "2")
	if !errors.Is(err, ErrSwarmUnreachable) {
		t.Fatalf("timeout = %v", err)
	}

	_, err = r.runFor(context.Background(), Node{Local: true}, time.Second, "stacker-no-such-binary-9f3a")
	if !errors.Is(err, ErrSwarmCommand) {
		t.Fatalf("missing = %v", err)
	}
}

func TestRunForRemoteMissingKey(t *testing.T) {
	r := runner{keys: stubKeys{err: errors.New("gone")}}
	_, err := r.runFor(context.Background(), Node{Ssh: "u@h", SshKeyID: "k"}, time.Second, "true")
	if !errors.Is(err, ErrSshKeyMissing) {
		t.Fatalf("runFor = %v", err)
	}
	_, err = r.dockerInput(context.Background(), Node{Ssh: "u@h", SshKeyID: "k"}, "in", "secret", "create", "n", "-")
	if !errors.Is(err, ErrSshKeyMissing) {
		t.Fatalf("dockerInput = %v", err)
	}
}

func TestRunForFakeExec(t *testing.T) {
	r := runner{exec: func(_ context.Context, item Node, _ time.Duration, argv []string, stdin string) (string, error) {
		if item.Ssh != "u@h" {
			t.Errorf("item = %+v", item)
		}
		if stdin != "" {
			t.Errorf("stdin = %q", stdin)
		}
		if argv[0] != "true" {
			t.Errorf("argv = %v", argv)
		}
		return "ok\n", nil
	}}
	out, err := r.runFor(context.Background(), Node{Ssh: "u@h"}, time.Second, "true")
	if err != nil || out != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestDockerInputFakeExec(t *testing.T) {
	r := runner{exec: func(_ context.Context, _ Node, _ time.Duration, argv []string, stdin string) (string, error) {
		if stdin != "secret" {
			t.Fatalf("stdin = %q", stdin)
		}
		if argv[0] != "docker" || argv[1] != "secret" {
			t.Fatalf("argv = %v", argv)
		}
		return "created", nil
	}}
	out, err := r.dockerInput(context.Background(), Node{Local: true}, "secret", "secret", "create", "name", "-")
	if err != nil || out != "created" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestShellUsesRunFor(t *testing.T) {
	r := runner{exec: func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		if argv[0] != "sh" || argv[1] != "-c" || argv[2] != "echo hi" {
			t.Fatalf("argv = %v", argv)
		}
		return "hi", nil
	}}
	out, err := r.shell(context.Background(), Node{Local: true}, time.Second, "echo hi")
	if err != nil || out != "hi" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestLookPathDefaultIsExec(t *testing.T) {
	if lookPath == nil {
		t.Fatal("lookPath unset")
	}
	// Real LookPath is a filesystem walk; safe to call.
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("true not on PATH")
	}
}
