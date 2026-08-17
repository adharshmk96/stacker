package node

import (
	"errors"
	"testing"
)

func TestRepositoryCRUD(t *testing.T) {
	repo := NewRepository(testDB(t))

	a := &Node{ID: "a", Name: "alpha", Ssh: "u@a", Port: 22, SshKeyID: "k", SwarmRole: SwarmRoleNone}
	b := &Node{ID: "b", Name: "beta", Ssh: "u@b", Port: 22, SshKeyID: "k", SwarmRole: SwarmRoleWorker, SwarmNodeID: "w1"}
	if err := repo.Create(a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(b); err != nil {
		t.Fatal(err)
	}

	items, err := repo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("list len = %d", len(items))
	}

	got, err := repo.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "alpha" {
		t.Errorf("name = %q", got.Name)
	}

	_, err = repo.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing = %v", err)
	}

	taken, err := repo.ExistsByName("alpha", "")
	if err != nil || !taken {
		t.Fatalf("exists alpha = %v %v", taken, err)
	}
	taken, err = repo.ExistsByName("alpha", "a")
	if err != nil || taken {
		t.Fatalf("exists excluding self = %v %v", taken, err)
	}

	a.Name = "alpha-renamed"
	if err := repo.Save(a); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing = %v", err)
	}
}

func TestRepositoryManagerAndCount(t *testing.T) {
	repo := NewRepository(testDB(t))

	_, err := repo.Manager()
	if !errors.Is(err, ErrNoManager) {
		t.Errorf("Manager empty = %v", err)
	}

	local := &Node{
		ID: LocalID, Name: "box", Local: true, SwarmRole: SwarmRoleManager, SwarmNodeID: "m1", SwarmAddr: "10.0.0.1",
	}
	remote := &Node{
		ID: "r1", Name: "other", SwarmRole: SwarmRoleManager, SwarmNodeID: "m2", SwarmAddr: "10.0.0.2",
	}
	worker := &Node{ID: "w1", Name: "w", SwarmRole: SwarmRoleWorker, SwarmNodeID: "wkr"}
	if err := repo.Create(remote); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(local); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(worker); err != nil {
		t.Fatal(err)
	}

	mgr, err := repo.Manager()
	if err != nil {
		t.Fatal(err)
	}
	if mgr.ID != LocalID {
		t.Errorf("preferred manager = %s", mgr.ID)
	}

	n, err := repo.CountByRole(SwarmRoleManager)
	if err != nil || n != 2 {
		t.Fatalf("manager count = %d %v", n, err)
	}
	n, err = repo.CountByRole(SwarmRoleWorker)
	if err != nil || n != 1 {
		t.Fatalf("worker count = %d %v", n, err)
	}
}
