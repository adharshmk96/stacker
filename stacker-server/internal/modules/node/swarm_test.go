package node

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestJoinWorker(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	worker := seedWorker(t, s, "w1", "edge")

	got, err := s.joinWorker(context.Background(), worker)
	if err != nil {
		t.Fatal(err)
	}
	if got.Node.SwarmRole != SwarmRoleWorker {
		t.Fatalf("role = %q", got.Node.SwarmRole)
	}
	if !strings.Contains(got.Message, "worker") {
		t.Errorf("message = %q", got.Message)
	}

	// already in our swarm
	worker, _ = s.repo.Get(worker.ID)
	again, err := s.joinWorker(context.Background(), worker)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(again.Message, "already") {
		t.Errorf("already = %q", again.Message)
	}
}

func TestJoinWorkerErrors(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)

	worker := seedWorker(t, s, "w1", "edge")
	worker.KeyStatus = KeyStatusUnknown
	if _, err := s.joinWorker(context.Background(), worker); !errors.Is(err, ErrKeyNotVerified) {
		t.Fatalf("unverified = %v", err)
	}

	worker.KeyStatus = KeyStatusOK
	if _, err := s.joinWorker(context.Background(), worker); !errors.Is(err, ErrNoManager) {
		t.Fatalf("no manager = %v", err)
	}

	mgr := seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")

	self := mgr
	self.KeyStatus = KeyStatusOK
	if _, err := s.joinWorker(context.Background(), self); !errors.Is(err, ErrAlreadyInSwarm) {
		t.Fatalf("self = %v", err)
	}

	mgr.SwarmAddr = ""
	if err := s.repo.Save(&mgr); err != nil {
		t.Fatal(err)
	}
	if _, err := s.joinWorker(context.Background(), worker); !errors.Is(err, ErrAdvertiseAddr) {
		t.Fatalf("addr = %v", err)
	}
	mgr.SwarmAddr = "192.168.1.10"
	if err := s.repo.Save(&mgr); err != nil {
		t.Fatal(err)
	}

	fake.setRole(LocalID, SwarmRoleNone, "")
	if _, err := s.joinWorker(context.Background(), worker); !errors.Is(err, ErrManagerUnhealthy) {
		t.Fatalf("inactive manager = %v", err)
	}
	restored, err := s.repo.Get(LocalID)
	if err != nil {
		t.Fatal(err)
	}
	restored.SwarmRole = SwarmRoleManager
	restored.SwarmAddr = "192.168.1.10"
	if err := s.repo.Save(&restored); err != nil {
		t.Fatal(err)
	}
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")

	fake.fails = []cmdFail{{contains: "LocalNodeState", err: errors.New("down")}}
	if _, err := s.joinWorker(context.Background(), worker); !errors.Is(err, ErrManagerUnhealthy) {
		t.Fatalf("unreadable manager = %v", err)
	}
	fake.fails = nil

	fake.useRoster = true
	fake.roster = []string{"mgr1"}
	fake.setRole(worker.ID, SwarmRoleWorker, "foreign")
	if _, err := s.joinWorker(context.Background(), worker); !errors.Is(err, ErrForeignSwarm) {
		t.Fatalf("foreign = %v", err)
	}
	fake.useRoster = false

	fake.token = ""
	fake.setRole(worker.ID, SwarmRoleNone, "")
	if _, err := s.joinWorker(context.Background(), worker); err == nil {
		t.Fatal("empty token")
	}
}

func TestPromoteDemoteLeave(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")

	worker := seedWorker(t, s, "w1", "edge")
	worker.SwarmRole = SwarmRoleWorker
	worker.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	fake.setRole(worker.ID, SwarmRoleWorker, "wkr1")

	promoted, err := s.Promote(context.Background(), worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Node.SwarmRole != SwarmRoleManager {
		t.Fatalf("promoted role = %q", promoted.Node.SwarmRole)
	}
	if !strings.Contains(promoted.Message, "even number") {
		t.Errorf("expected even-manager note, got %q", promoted.Message)
	}

	demoted, err := s.Demote(context.Background(), worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if demoted.Node.SwarmRole != SwarmRoleWorker {
		t.Fatalf("demoted role = %q", demoted.Node.SwarmRole)
	}

	left, err := s.Leave(context.Background(), worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if left.Node.SwarmRole != SwarmRoleNone {
		t.Fatalf("left role = %q", left.Node.SwarmRole)
	}
}

func TestChangeRoleErrors(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)

	if _, err := s.Promote(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}

	plain := seedWorker(t, s, "w1", "edge")
	if _, err := s.Promote(context.Background(), plain.ID); !errors.Is(err, ErrNotInSwarm) {
		t.Fatalf("not in swarm = %v", err)
	}

	plain.SwarmRole = SwarmRoleManager
	plain.SwarmNodeID = "m"
	if err := s.repo.Save(&plain); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(context.Background(), plain.ID); !errors.Is(err, ErrNotWorker) {
		t.Fatalf("not worker = %v", err)
	}
	if _, err := s.Demote(context.Background(), plain.ID); !errors.Is(err, ErrLastManager) {
		t.Fatalf("last manager = %v", err)
	}

	plain.SwarmRole = SwarmRoleWorker
	if err := s.repo.Save(&plain); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Demote(context.Background(), plain.ID); !errors.Is(err, ErrNotManagerRole) {
		t.Fatalf("not manager = %v", err)
	}

	unlock, _ := s.lock(plain.ID)
	if _, err := s.Promote(context.Background(), plain.ID); !errors.Is(err, ErrSwarmBusy) {
		t.Fatalf("busy = %v", err)
	}
	unlock()
}

func TestPromoteUnknownDockerID(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")

	worker := seedWorker(t, s, "w1", "edge")
	worker.SwarmRole = SwarmRoleWorker
	worker.SwarmNodeID = ""
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Promote(context.Background(), worker.ID); err == nil {
		t.Fatal("expected unknown docker id")
	}
}

func TestQuorumLeaderWarning(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")

	worker := seedWorker(t, s, "w1", "edge")
	worker.SwarmRole = SwarmRoleWorker
	worker.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	fake.setRole(worker.ID, SwarmRoleWorker, "wkr1")
	fake.fails = []cmdFail{{contains: "node ls", out: "", err: errors.New("The swarm does not have a leader")}}

	got, err := s.Promote(context.Background(), worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Message, "lost its leader") {
		t.Errorf("message = %q", got.Message)
	}
}

func TestQuorumQueryWarning(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")

	worker := seedWorker(t, s, "w1", "edge")
	worker.SwarmRole = SwarmRoleWorker
	worker.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	fake.setRole(worker.ID, SwarmRoleWorker, "wkr1")
	fake.fails = []cmdFail{{contains: "node ls", err: errors.New("connection refused")}}

	got, err := s.Promote(context.Background(), worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Message, "could not be queried") {
		t.Errorf("message = %q", got.Message)
	}
}

func TestLeaveErrors(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)

	if _, err := s.Leave(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}

	plain := seedWorker(t, s, "w1", "edge")
	if _, err := s.Leave(context.Background(), plain.ID); !errors.Is(err, ErrNotInSwarm) {
		t.Fatalf("not in swarm = %v", err)
	}

	plain.SwarmRole = SwarmRoleManager
	if err := s.repo.Save(&plain); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Leave(context.Background(), plain.ID); !errors.Is(err, ErrLastManager) {
		t.Fatalf("last manager = %v", err)
	}

	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	plain.SwarmRole = SwarmRoleWorker
	plain.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&plain); err != nil {
		t.Fatal(err)
	}
	fake.setRole(plain.ID, SwarmRoleWorker, "wkr1")
	fake.fails = []cmdFail{{contains: "swarm leave", out: "this node is not part of a swarm", err: errors.New("exit 1")}}
	got, err := s.Leave(context.Background(), plain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Node.SwarmRole != SwarmRoleNone {
		t.Fatalf("already-left treated as success: %+v", got.Node)
	}

	plain.SwarmRole = SwarmRoleWorker
	plain.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&plain); err != nil {
		t.Fatal(err)
	}
	fake.fails = []cmdFail{{contains: "swarm leave", err: errors.New("boom")}}
	if _, err := s.Leave(context.Background(), plain.ID); err == nil {
		t.Fatal("expected leave error")
	}
}

func TestRefreshSwarmAndAll(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	idle := seedWorker(t, s, "idle", "idle")

	worker := seedWorker(t, s, "w1", "edge")
	worker.SwarmRole = SwarmRoleWorker
	worker.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	fake.setRole(worker.ID, SwarmRoleWorker, "wkr1")

	got, err := s.RefreshSwarm(context.Background(), worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Node.SwarmRole != SwarmRoleWorker {
		t.Fatalf("refresh = %q", got.Node.SwarmRole)
	}

	items, err := s.RefreshAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("refresh all len = %d", len(items))
	}

	unlock, _ := s.lock(worker.ID)
	items, err = s.RefreshAll(context.Background())
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	_ = items
	_ = idle

	fake.fails = []cmdFail{{contains: "LocalNodeState", err: errors.New("down")}}
	if _, err := s.RefreshSwarm(context.Background(), worker.ID); err == nil {
		t.Fatal("expected refresh error")
	}
	stored, err := s.repo.Get(worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SwarmError == "" {
		t.Error("expected swarm error recorded")
	}

	if _, err := s.RefreshSwarm(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
}

func TestSyncLocalSwarm(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)

	s.SyncLocalSwarm("10.0.0.1") // no local row

	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	s.SyncLocalSwarm("10.9.8.7")
	item, _ := s.repo.Get(LocalID)
	if item.SwarmAddr != "10.9.8.7" {
		t.Errorf("addr = %q", item.SwarmAddr)
	}

	fake.setRole(LocalID, SwarmRoleWorker, "mgr1")
	s.SyncLocalSwarm("10.9.8.7")

	fake.fails = []cmdFail{{contains: "LocalNodeState", err: errors.New("down")}}
	s.SyncLocalSwarm("10.9.8.7")
}

func TestInOurSwarmAndDelete(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)

	remote := seedWorker(t, s, "w1", "edge")
	if err := s.Delete(context.Background(), remote.ID); err != nil {
		t.Fatal(err)
	}

	remote = seedWorker(t, s, "w2", "edge2")
	remote.SwarmRole = SwarmRoleWorker
	remote.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&remote); err != nil {
		t.Fatal(err)
	}
	fake.fails = []cmdFail{{contains: "LocalNodeState", err: errors.New("offline")}}
	if err := s.Delete(context.Background(), remote.ID); !errors.Is(err, ErrNodeInSwarm) {
		t.Fatalf("offline fallback = %v", err)
	}
	fake.fails = nil

	fake.setRole(remote.ID, SwarmRoleNone, "")
	if err := s.Delete(context.Background(), remote.ID); err != nil {
		t.Fatal(err)
	}

	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	remote = seedWorker(t, s, "w3", "edge3")
	fake.setRole(remote.ID, SwarmRoleWorker, "wkr3")
	remote.SwarmRole = SwarmRoleNone
	if err := s.repo.Save(&remote); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), remote.ID); !errors.Is(err, ErrNodeInSwarm) {
		t.Fatalf("known by manager = %v", err)
	}

	fake.useRoster = true
	fake.roster = []string{"mgr1"}
	fake.setRole(remote.ID, SwarmRoleWorker, "other")
	if err := s.Delete(context.Background(), remote.ID); err != nil {
		t.Fatal(err)
	}
	fake.useRoster = false

	remote = seedWorker(t, s, "w4", "edge4")
	fake.setRole(remote.ID, SwarmRoleWorker, "wkr4")
	fake.fails = []cmdFail{{contains: "node ls", err: errors.New("nope")}}
	remote.SwarmRole = SwarmRoleWorker
	if err := s.repo.Save(&remote); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(context.Background(), remote.ID); !errors.Is(err, ErrNodeInSwarm) {
		t.Fatalf("managerKnows error fallback = %v", err)
	}
}

func TestControlNodeSelf(t *testing.T) {
	s := testService(t)
	mgr := seedManager(t, s)
	got, err := s.controlNode(mgr)
	if err != nil || got.ID != LocalID {
		t.Fatalf("self manager = %+v %v", got, err)
	}
}

func TestRecordSwarmFailureTruncates(t *testing.T) {
	s := testService(t)
	item := seedWorker(t, s, "w1", "edge")
	cause := fmt.Errorf("%s", strings.Repeat("x", 600))
	err := s.recordSwarmFailure(item, cause)
	if !errors.Is(err, cause) && err.Error() != cause.Error() {
		t.Fatalf("returned %v", err)
	}
	stored, _ := s.repo.Get(item.ID)
	if !strings.HasSuffix(stored.SwarmError, "…") || stored.SwarmError == cause.Error() {
		t.Errorf("expected truncated error, got len %d", len(stored.SwarmError))
	}
}

func TestManagerAPI(t *testing.T) {
	s := testService(t)
	if _, err := s.Manager(); !errors.Is(err, ErrNoManager) {
		t.Fatalf("empty = %v", err)
	}
	seedManager(t, s)
	got, err := s.Manager()
	if err != nil || got.ID != LocalID {
		t.Fatalf("manager = %+v %v", got, err)
	}
}

func TestStateParseAndToken(t *testing.T) {
	s := testService(t)
	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		cmd := strings.Join(argv, " ")
		if strings.Contains(cmd, "LocalNodeState") {
			return "WARNING: foo\nactive\tnid\ttrue", nil
		}
		if strings.Contains(cmd, "join-token") {
			return "\n\n", nil
		}
		return "", fmt.Errorf("unexpected %s", cmd)
	}
	st, err := s.rt.state(context.Background(), Node{Local: true})
	if err != nil || st.Role() != SwarmRoleManager || st.NodeID != "nid" {
		t.Fatalf("state = %+v %v", st, err)
	}
	if _, err := s.rt.joinToken(context.Background(), Node{Local: true}, SwarmRoleWorker); err == nil {
		t.Fatal("empty token")
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, _ []string, _ string) (string, error) {
		return "not-tab-separated", nil
	}
	if _, err := s.rt.state(context.Background(), Node{Local: true}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestAwaitRoleImmediate(t *testing.T) {
	s := testService(t)
	item := seedWorker(t, s, "w1", "edge")
	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		if strings.Contains(strings.Join(argv, " "), "LocalNodeState") {
			return "active\twkr\tfalse", nil
		}
		return "", fmt.Errorf("unexpected")
	}
	got, err := s.awaitRole(context.Background(), item, SwarmRoleWorker)
	if err != nil {
		t.Fatal(err)
	}
	if got.SwarmRole != SwarmRoleWorker {
		t.Fatalf("role = %q", got.SwarmRole)
	}
}

func TestDockerClassification(t *testing.T) {
	s := testService(t)
	item := Node{Local: true}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		cmd := strings.Join(argv, " ")
		if strings.HasPrefix(cmd, "docker ") {
			return "Got permission denied while trying to connect", errors.New("exit 1")
		}
		if strings.HasPrefix(cmd, "sudo -n docker ") {
			return "ok", nil
		}
		return "", fmt.Errorf("unexpected %s", cmd)
	}
	out, err := s.Docker(context.Background(), item, "info")
	if err != nil || out != "ok" {
		t.Fatalf("sudo retry = %q %v", out, err)
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, _ []string, _ string) (string, error) {
		return "", errors.New("executable file not found in $PATH")
	}
	if _, err := s.Docker(context.Background(), item, "ps"); !errors.Is(err, ErrDockerMissing) {
		t.Fatalf("missing = %v", err)
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, _ []string, _ string) (string, error) {
		return "Cannot connect to the Docker daemon", errors.New("exit 1")
	}
	if _, err := s.Docker(context.Background(), item, "ps"); !errors.Is(err, ErrDockerNotRunning) {
		t.Fatalf("daemon = %v", err)
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, _ []string, _ string) (string, error) {
		return "something else went wrong", errors.New("exit 1")
	}
	if _, err := s.Docker(context.Background(), item, "ps"); !errors.Is(err, ErrSwarmCommand) {
		t.Fatalf("other = %v", err)
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, stdin string) (string, error) {
		if stdin != "payload" {
			return "", fmt.Errorf("stdin = %q", stdin)
		}
		return "created", nil
	}
	out, err = s.DockerInput(context.Background(), item, "payload", "secret", "create", "n", "-")
	if err != nil || out != "created" {
		t.Fatalf("input = %q %v", out, err)
	}
}

func TestManagerKnowsEmpty(t *testing.T) {
	s := testService(t)
	known, err := s.managerKnows(context.Background(), Node{}, "")
	if err != nil || known {
		t.Fatalf("empty id = %v %v", known, err)
	}
}

func TestJoinWorkerJoinFails(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	worker := seedWorker(t, s, "w1", "edge")
	fake.fails = []cmdFail{{contains: "swarm join", err: errors.New("refused")}}
	if _, err := s.joinWorker(context.Background(), worker); err == nil {
		t.Fatal("expected join error")
	}
}
