package node

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateGetUpdateDelete(t *testing.T) {
	s := testService(t)

	_, err := s.Create(CreateRequest{Name: "edge", Ssh: "bad", SshKeyID: "k"})
	if !errors.Is(err, ErrInvalidSsh) {
		t.Fatalf("invalid ssh = %v", err)
	}

	s.keys = stubKeys{err: errors.New("missing")}
	_, err = s.Create(CreateRequest{Name: "edge", Ssh: "deploy@host", SshKeyID: "k"})
	if !errors.Is(err, ErrSshKeyMissing) {
		t.Fatalf("missing key = %v", err)
	}
	s.keys = stubKeys{}

	item, err := s.Create(CreateRequest{Name: " edge ", Ssh: " deploy@host ", Port: 0, SshKeyID: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "edge" || item.Port != 22 || item.KeyStatus != KeyStatusUnknown {
		t.Fatalf("create item = %+v", item)
	}

	_, err = s.Create(CreateRequest{Name: "edge", Ssh: "deploy@other", SshKeyID: "k1"})
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("name taken = %v", err)
	}

	got, err := s.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reachability != ReachabilityUnknown {
		t.Errorf("reachability = %q", got.Reachability)
	}

	items, err := s.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("list = %d %v", len(items), err)
	}

	_, err = s.Update("missing", UpdateRequest{Name: "x", Ssh: "u@h", SshKeyID: "k"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing = %v", err)
	}

	_, err = s.Update(item.ID, UpdateRequest{Name: "edge-2", Ssh: "bad", SshKeyID: "k1"})
	if !errors.Is(err, ErrInvalidSsh) {
		t.Fatalf("update ssh = %v", err)
	}

	s.keys = stubKeys{err: errors.New("missing")}
	_, err = s.Update(item.ID, UpdateRequest{Name: "edge-2", Ssh: "deploy@host", SshKeyID: "k1"})
	if !errors.Is(err, ErrSshKeyMissing) {
		t.Fatalf("update key = %v", err)
	}
	s.keys = stubKeys{}

	updated, err := s.Update(item.ID, UpdateRequest{Name: "edge-2", Ssh: "deploy@other", Port: 2200, SshKeyID: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "edge-2" || updated.Port != 2200 || updated.KeyStatus != KeyStatusUnknown || updated.KeyCheckedAt != nil {
		t.Fatalf("updated = %+v", updated)
	}

	if err := s.Delete(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v", err)
	}
	if err := s.Delete(context.Background(), item.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureLocal(t *testing.T) {
	s := testService(t)

	first, err := s.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != LocalID || !first.Local || first.KeyStatus != KeyStatusOK {
		t.Fatalf("local = %+v", first)
	}
	if first.Name != localName() {
		t.Errorf("name = %q want %q", first.Name, localName())
	}

	again, err := s.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.Name != first.Name {
		t.Fatal("EnsureLocal is not idempotent")
	}

	// A remote node already owns the hostname: the local row suffixes.
	s2 := testService(t)
	taken := Node{ID: "r1", Name: localName(), Ssh: "u@h", Port: 22, SshKeyID: "k"}
	if err := s2.repo.Create(&taken); err != nil {
		t.Fatal(err)
	}
	local, err := s2.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}
	want := localName() + " (2)"
	if local.Name != want {
		t.Errorf("suffixed name = %q want %q", local.Name, want)
	}
}

func TestUpdateRenameLocal(t *testing.T) {
	s := testService(t)
	local, err := s.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Create(CreateRequest{Name: "taken", Ssh: "deploy@host", SshKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Update(local.ID, UpdateRequest{Name: "taken"})
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("local name taken = %v", err)
	}

	renamed, err := s.Update(local.ID, UpdateRequest{Name: "my-box"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "my-box" {
		t.Errorf("name = %q", renamed.Name)
	}

	if err := s.Delete(context.Background(), local.ID); !errors.Is(err, ErrLocalNode) {
		t.Fatalf("delete local = %v", err)
	}
}

func TestUpdateNameTakenRemote(t *testing.T) {
	s := testService(t)
	a, err := s.Create(CreateRequest{Name: "a", Ssh: "deploy@a", SshKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Create(CreateRequest{Name: "b", Ssh: "deploy@b", SshKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(a.ID, UpdateRequest{Name: "b", Ssh: "deploy@a", SshKeyID: "k"})
	if !errors.Is(err, ErrNameTaken) {
		t.Fatalf("remote name taken = %v", err)
	}
}

func TestApplyState(t *testing.T) {
	s := testService(t)
	item := seedWorker(t, s, "w1", "edge")
	item.SwarmAddr = "should-clear"

	updated, err := s.applyState(item, swarmState{LocalNodeState: "active", NodeID: "n1", ControlAvailable: false}, "10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if updated.SwarmRole != SwarmRoleWorker || updated.SwarmAddr != "" || updated.SwarmNodeID != "n1" || updated.SwarmError != "" {
		t.Fatalf("worker state = %+v", updated)
	}

	mgr := seedManager(t, s)
	updated, err = s.applyState(mgr, swarmState{LocalNodeState: "active", NodeID: "m1", ControlAvailable: true}, "192.168.1.10")
	if err != nil {
		t.Fatal(err)
	}
	if updated.SwarmRole != SwarmRoleManager || updated.SwarmAddr != "192.168.1.10" {
		t.Fatalf("manager state = %+v", updated)
	}
}

func TestLockBusy(t *testing.T) {
	s := testService(t)

	unlock, err := s.lock("n1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.lock("n1")
	if !errors.Is(err, ErrSwarmBusy) {
		t.Fatalf("busy = %v", err)
	}
	unlock2, err := s.lock("n2")
	if err != nil {
		t.Fatal(err)
	}
	unlock2()
	unlock()

	unlock, err = s.lock("n1")
	if err != nil {
		t.Fatal(err)
	}
	unlock()
}

func TestCheckKey(t *testing.T) {
	s := testService(t)
	local, err := s.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.CheckKey(context.Background(), local.ID)
	if err != nil || !got.OK {
		t.Fatalf("local check = %+v %v", got, err)
	}

	_, err = s.CheckKey(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}

	remote, err := s.Create(CreateRequest{Name: "edge", Ssh: "deploy@host", SshKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}

	s.keys = stubKeys{err: errors.New("missing")}
	_, err = s.CheckKey(context.Background(), remote.ID)
	if !errors.Is(err, ErrSshKeyMissing) {
		t.Fatalf("key missing = %v", err)
	}
	s.keys = stubKeys{}

	stubProbe(t, true, "")
	ok, err := s.CheckKey(context.Background(), remote.ID)
	if err != nil || !ok.OK {
		t.Fatalf("remote ok = %+v %v", ok, err)
	}
	stored, _ := s.repo.Get(remote.ID)
	if stored.KeyStatus != KeyStatusOK || stored.KeyCheckedAt == nil {
		t.Fatalf("stored ok = %+v", stored)
	}

	stubProbe(t, false, "Permission denied")
	fail, err := s.CheckKey(context.Background(), remote.ID)
	if err != nil || fail.OK {
		t.Fatalf("remote fail = %+v %v", fail, err)
	}
	stored, _ = s.repo.Get(remote.ID)
	if stored.KeyStatus != KeyStatusFailed {
		t.Fatalf("stored fail = %+v", stored)
	}
}

func TestInstallKeyValidation(t *testing.T) {
	s := testService(t)

	_, err := s.InstallKey(context.Background(), InstallKeyRequest{Ssh: "bad", SshKeyID: "k"})
	if !errors.Is(err, ErrInvalidSsh) {
		t.Fatalf("ssh = %v", err)
	}

	s.keys = stubKeys{err: errors.New("missing")}
	_, err = s.InstallKey(context.Background(), InstallKeyRequest{Ssh: "deploy@host", SshKeyID: "k"})
	if !errors.Is(err, ErrSshKeyMissing) {
		t.Fatalf("key = %v", err)
	}
	s.keys = stubKeys{}

	stubProbe(t, true, "")
	ok, err := s.InstallKey(context.Background(), InstallKeyRequest{Ssh: "deploy@host", SshKeyID: "k"})
	if err != nil || !ok.OK {
		t.Fatalf("already installed = %+v %v", ok, err)
	}

	stubProbe(t, false, "denied")
	_, err = s.InstallKey(context.Background(), InstallKeyRequest{Ssh: "deploy@host", SshKeyID: "k"})
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("password = %v", err)
	}

	stubLookPath(t, map[string]bool{})
	_, err = s.InstallKey(context.Background(), InstallKeyRequest{Ssh: "deploy@host", SshKeyID: "k", Password: "pw"})
	if !errors.Is(err, ErrCopyIDMissing) {
		t.Fatalf("sshpass missing = %v", err)
	}

	stubLookPath(t, map[string]bool{"sshpass": true})
	_, err = s.InstallKey(context.Background(), InstallKeyRequest{Ssh: "deploy@host", SshKeyID: "k", Password: "pw"})
	if !errors.Is(err, ErrCopyIDMissing) {
		t.Fatalf("ssh-copy-id missing = %v", err)
	}
}

func TestPingLocalAndRemote(t *testing.T) {
	s := testService(t)
	local, err := s.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.Ping(context.Background(), local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Reachability != ReachabilityOnline {
		t.Errorf("local reachability = %q", got.Reachability)
	}

	remote, err := s.Create(CreateRequest{Name: "edge", Ssh: "deploy@host", SshKeyID: "k"})
	if err != nil {
		t.Fatal(err)
	}

	s.keys = stubKeys{err: errors.New("missing")}
	off, err := s.Ping(context.Background(), remote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if off.Reachability != ReachabilityOffline {
		t.Errorf("missing key reachability = %q", off.Reachability)
	}
	s.keys = stubKeys{}

	stubProbe(t, true, "")
	on, err := s.Ping(context.Background(), remote.ID)
	if err != nil {
		t.Fatal(err)
	}
	if on.Reachability != ReachabilityOnline {
		t.Errorf("remote online = %q", on.Reachability)
	}

	stubProbe(t, false, "timed out")
	all, err := s.PingAll(context.Background())
	if err != nil || len(all) != 2 {
		t.Fatalf("ping all = %d %v", len(all), err)
	}

	_, err = s.Ping(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ping missing = %v", err)
	}
}

func TestDecorateUnknown(t *testing.T) {
	s := testService(t)
	item := Node{ID: "n"}
	s.decorate(&item)
	if item.Reachability != ReachabilityUnknown {
		t.Errorf("reachability = %q", item.Reachability)
	}
}

func TestHealthDrop(t *testing.T) {
	s := testService(t)
	s.health.set("n1", health{State: ReachabilityOnline, At: time.Now(), Message: "up"})
	s.health.drop("n1")
	if _, ok := s.health.get("n1"); ok {
		t.Fatal("expected drop")
	}
}
