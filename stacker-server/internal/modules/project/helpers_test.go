package project

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestRollUp(t *testing.T) {
	cases := []struct {
		name  string
		items []RuntimeState
		want  RuntimeState
	}{
		{"empty", nil, RuntimeStopped},
		{"one running", []RuntimeState{RuntimeRunning}, RuntimeRunning},
		{"stopped beats running", []RuntimeState{RuntimeRunning, RuntimeStopped}, RuntimeStopped},
		{"deploying beats stopped", []RuntimeState{RuntimeStopped, RuntimeDeploying}, RuntimeDeploying},
		{"unknown beats deploying", []RuntimeState{RuntimeDeploying, RuntimeUnknown}, RuntimeUnknown},
		{"degraded is worst", []RuntimeState{RuntimeRunning, RuntimeUnknown, RuntimeDegraded}, RuntimeDegraded},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envs := make([]EnvironmentStatus, len(tc.items))
			for i, state := range tc.items {
				envs[i] = EnvironmentStatus{State: state}
			}
			if got := rollUp(envs); got != tc.want {
				t.Errorf("rollUp = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseReplicasGarbage(t *testing.T) {
	cases := []struct {
		in          string
		wantRunning int
		wantDesired int
	}{
		{"", 0, 0},
		{"2", 0, 0},
		{"n/a", 0, 0},
		{"abc/3", 0, 0},
		{"2/xyz", 0, 0},
		{"2/3", 2, 3},
		{"1/1 (max 1 per node)", 1, 1},
	}

	for _, tc := range cases {
		running, desired := parseReplicas(tc.in)
		if running != tc.wantRunning || desired != tc.wantDesired {
			t.Errorf("parseReplicas(%q) = %d/%d, want %d/%d",
				tc.in, running, desired, tc.wantRunning, tc.wantDesired)
		}
	}
}

func TestRenderRoutesUnknownTLSFallsBackToAuto(t *testing.T) {
	// "custom" is a retired mode: rows stored before it was removed must keep
	// serving https rather than silently dropping to plain http.
	out, err := renderRoutes("stk-shop-production", []Domain{
		{Host: "shop.acme.dev", Service: "web", Port: 443, TLS: TLSMode("custom")},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc routes
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("yaml: %v\n%s", err, out)
	}

	router := doc.HTTP.Routers["stk-shop-production-0"]
	if router.TLS == nil || router.TLS.CertResolver == "" {
		t.Fatal("a retired TLS mode should be served with the ACME resolver")
	}
	if got := strings.Join(router.EntryPoints, ","); got != "websecure" {
		t.Errorf("entrypoints = %q, want websecure", got)
	}
}

func TestStackNameFallsBackToIDs(t *testing.T) {
	item := Project{ID: "projid", Name: "!!!"}
	env := Environment{ID: "envid", Name: "***"}

	got := StackName(item, env)
	if got != "stk-projid-envid" {
		t.Fatalf("stack = %q, want the ids when the names slug to nothing", got)
	}
}

func TestSlugCapsLength(t *testing.T) {
	got := slug(strings.Repeat("A", 50) + "!!!")
	if len(got) > 40 {
		t.Errorf("slug length = %d, want at most 40", len(got))
	}
	if strings.ContainsAny(got, "A!") {
		t.Errorf("slug = %q, want lowercase alphanumerics", got)
	}
}

func TestExecCommandViaFalseAndSh(t *testing.T) {
	t.Run("false fails", func(t *testing.T) {
		err := execCommand(context.Background(), Command{Name: "false"}, func(string) {})
		if err == nil {
			t.Fatal("false succeeded")
		}
		if !strings.Contains(err.Error(), "false failed") {
			t.Errorf("error = %v, want it to name the command", err)
		}
	})

	t.Run("sh streams stdout and stderr", func(t *testing.T) {
		var lines []string
		err := execCommand(context.Background(), Command{
			Name: "sh",
			Args: []string{"-c", "echo hello; echo world >&2"},
		}, func(line string) { lines = append(lines, line) })
		if err != nil {
			t.Fatalf("sh: %v", err)
		}
		joined := strings.Join(lines, "|")
		if !strings.Contains(joined, "hello") || !strings.Contains(joined, "world") {
			t.Errorf("lines = %v, want hello and world merged", lines)
		}
	})

	t.Run("sh failure keeps the tail", func(t *testing.T) {
		err := execCommand(context.Background(), Command{
			Name: "sh",
			Args: []string{"-c", "echo boom; exit 1"},
		}, func(string) {})
		if err == nil {
			t.Fatal("sh should have failed")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("error = %v, want the command's last line", err)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		err := execCommand(context.Background(), Command{Name: "stacker-no-such-command"}, func(string) {})
		if err == nil {
			t.Fatal("missing binary succeeded")
		}
		if !strings.Contains(err.Error(), "could not start") {
			t.Errorf("error = %v, want a start failure", err)
		}
	})
}

func TestCommandString(t *testing.T) {
	got := Command{Name: "git", Args: []string{"clone", "repo"}}.String()
	if got != "git clone repo" {
		t.Errorf("String = %q", got)
	}
}

func TestChunkOutOfRangeCursor(t *testing.T) {
	got := chunk("d1", StatusSucceeded, []string{"a", "b"}, 99)
	if len(got.Lines) != 0 || got.Next != 2 || !got.Done {
		t.Errorf("chunk = %+v, want an empty tail at the end", got)
	}

	got = chunk("d1", StatusRunning, []string{"a"}, -1)
	if len(got.Lines) != 0 || got.Next != 1 || got.Done {
		t.Errorf("negative cursor = %+v, want it clamped", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("short = %q", got)
	}
	if got := truncate("abcdefghij", 6); got != "abcde…" {
		t.Errorf("long = %q", got)
	}
}

func TestFinishTimeoutAndCancelOutcomes(t *testing.T) {
	service, _ := testService(t, Options{})
	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	timeout := Deployment{
		ID: newID(), ProjectID: item.ID, ProjectName: item.Name,
		EnvironmentID: item.Environments[0].ID, Environment: "production",
		Status: StatusRunning, StartedAt: timeNow(),
	}
	if err := service.repo.CreateDeployment(&timeout); err != nil {
		t.Fatalf("seed timeout: %v", err)
	}
	service.engine.finish(&run{status: StatusRunning}, &timeout, context.DeadlineExceeded)
	stored, _ := service.Deployment(timeout.ID)
	if stored.Status != StatusFailed || stored.Error != "the deployment timed out" {
		t.Fatalf("timeout = %s %q", stored.Status, stored.Error)
	}

	cancelled := Deployment{
		ID: newID(), ProjectID: item.ID, ProjectName: item.Name,
		EnvironmentID: item.Environments[0].ID, Environment: "production",
		Status: StatusRunning, StartedAt: timeNow(),
	}
	if err := service.repo.CreateDeployment(&cancelled); err != nil {
		t.Fatalf("seed cancel: %v", err)
	}
	service.engine.finish(&run{status: StatusRunning}, &cancelled, context.Canceled)
	stored, _ = service.Deployment(cancelled.ID)
	if stored.Status != StatusCancelled {
		t.Fatalf("cancel = %s, want cancelled", stored.Status)
	}
}

func TestValidHostRejectsNoise(t *testing.T) {
	cases := map[string]bool{
		"shop.acme.dev":    true,
		"*.acme.dev":       true,
		"localhost":        false,
		"shop.acme.dev.":   false,
		"shop.acme.dev:80": false,
		"":                 false,
	}
	for host, want := range cases {
		if got := validHost(host); got != want {
			t.Errorf("validHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestParseComposeRejectsUnusableServiceName(t *testing.T) {
	_, err := parseCompose("services:\n  \"bad name\":\n    image: nginx\n")
	if !errors.Is(err, ErrComposeInvalid) {
		t.Fatalf("error = %v, want %v", err, ErrComposeInvalid)
	}
}
