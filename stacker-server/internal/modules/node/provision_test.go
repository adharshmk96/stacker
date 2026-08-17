package node

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProvisionHappyPathSkipsDockerInstall(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	worker := seedWorker(t, s, "w1", "edge")

	job, err := s.Provision(worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != ProvisionRunning {
		t.Fatalf("started = %q", job.State)
	}
	done := waitProvision(t, s, worker.ID)
	if done.State != ProvisionSucceeded {
		t.Fatalf("job = %+v", done)
	}
	if stepByKey(done, "docker").State != StepSkipped {
		t.Errorf("docker step = %+v", stepByKey(done, "docker"))
	}
	if stepByKey(done, "reach").State != StepDone {
		t.Errorf("reach = %+v", stepByKey(done, "reach"))
	}
	stored, _ := s.repo.Get(worker.ID)
	if stored.SwarmRole != SwarmRoleWorker {
		t.Errorf("joined role = %q", stored.SwarmRole)
	}
}

func TestProvisionGuards(t *testing.T) {
	s := testService(t)

	if _, err := s.Provision("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}

	local, err := s.EnsureLocal()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Provision(local.ID); !errors.Is(err, ErrLocalSetupManaged) {
		t.Fatalf("local = %v", err)
	}

	worker := seedWorker(t, s, "w1", "edge")
	worker.KeyStatus = KeyStatusUnknown
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Provision(worker.ID); !errors.Is(err, ErrKeyNotVerified) {
		t.Fatalf("key = %v", err)
	}

	worker.KeyStatus = KeyStatusOK
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	unlock, _ := s.lock(worker.ID)
	if _, err := s.Provision(worker.ID); !errors.Is(err, ErrSwarmBusy) {
		t.Fatalf("busy = %v", err)
	}
	unlock()

	if _, err := s.ProvisionStatus(worker.ID); !errors.Is(err, ErrNoProvisionRun) {
		t.Fatalf("no run = %v", err)
	}
}

func TestProvisionReachFails(t *testing.T) {
	s := testService(t)
	worker := seedWorker(t, s, "w1", "edge")
	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, _ []string, _ string) (string, error) {
		return "", errors.New("no route")
	}
	if _, err := s.Provision(worker.ID); err != nil {
		t.Fatal(err)
	}
	done := waitProvision(t, s, worker.ID)
	if done.State != ProvisionFailed {
		t.Fatalf("job = %+v", done)
	}
}

func TestStepReach(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	ctx := context.Background()

	p := &provisionCtx{node: Node{ID: "n"}}
	state, detail, err := s.stepReach(ctx, p)
	if err != nil || state != StepDone || !strings.Contains(detail, "Debian") {
		t.Fatalf("linux = %s %q %v", state, detail, err)
	}

	fake.uname = "Darwin"
	p = &provisionCtx{node: Node{ID: "n"}}
	state, _, err = s.stepReach(ctx, p)
	if err != nil || state != StepWarned || !p.skipInstall {
		t.Fatalf("darwin docker = %s skip=%v %v", state, p.skipInstall, err)
	}

	fake.hasDocker = false
	p = &provisionCtx{node: Node{ID: "n"}}
	_, _, err = s.stepReach(ctx, p)
	if !errors.Is(err, ErrUnsupportedOS) {
		t.Fatalf("remote not linux = %v", err)
	}

	p = &provisionCtx{node: Node{ID: "n", Local: true}}
	_, _, err = s.stepReach(ctx, p)
	if !errors.Is(err, ErrLocalNotLinux) {
		t.Fatalf("local not linux = %v", err)
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, _ []string, _ string) (string, error) {
		return "", errors.New("timeout")
	}
	_, _, err = s.stepReach(ctx, &provisionCtx{node: Node{ID: "n"}})
	if !errors.Is(err, ErrSwarmUnreachable) {
		t.Fatalf("unreachable = %v", err)
	}

	// Linux with no os-release still succeeds using the kernel name.
	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		cmd := strings.Join(argv, " ")
		if argv[0] == "uname" {
			return "Linux", nil
		}
		if strings.Contains(cmd, "os-release") {
			return "", errors.New("missing")
		}
		return "", errors.New("unexpected " + cmd)
	}
	p = &provisionCtx{node: Node{ID: "n"}}
	state, detail, err = s.stepReach(ctx, p)
	if err != nil || state != StepDone || detail != "Linux" {
		t.Fatalf("no os-release = %s %q %v", state, detail, err)
	}
}

func TestStepPrivileges(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	ctx := context.Background()

	state, _, err := s.stepPrivileges(ctx, &provisionCtx{skipInstall: true})
	if err != nil || state != StepSkipped {
		t.Fatalf("skip = %s %v", state, err)
	}

	p := &provisionCtx{node: Node{ID: "n"}}
	state, detail, err := s.stepPrivileges(ctx, p)
	if err != nil || state != StepDone || p.sudo != "" || !strings.Contains(detail, "root") {
		t.Fatalf("root = %s %q sudo=%q %v", state, detail, p.sudo, err)
	}

	fake.uid = "1000"
	p = &provisionCtx{node: Node{ID: "n"}}
	state, detail, err = s.stepPrivileges(ctx, p)
	if err != nil || p.sudo != "sudo -n" || !strings.Contains(detail, "sudo") {
		t.Fatalf("sudo = %s %q sudo=%q %v", state, detail, p.sudo, err)
	}

	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		cmd := strings.Join(argv, " ")
		if argv[0] == "id" {
			return "1000", nil
		}
		if strings.Contains(cmd, "sudo -n true") {
			return "", errors.New("password required")
		}
		return "", errors.New("unexpected " + cmd)
	}
	_, _, err = s.stepPrivileges(ctx, &provisionCtx{node: Node{ID: "n"}})
	if !errors.Is(err, ErrSudoRequired) {
		t.Fatalf("sudo required = %v", err)
	}
}

func TestStepTools(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	ctx := context.Background()

	state, _, err := s.stepTools(ctx, &provisionCtx{skipInstall: true})
	if err != nil || state != StepSkipped {
		t.Fatalf("skip = %s %v", state, err)
	}

	state, detail, err := s.stepTools(ctx, &provisionCtx{node: Node{ID: "n"}})
	if err != nil || state != StepSkipped || !strings.Contains(detail, "already") {
		t.Fatalf("curl present = %s %q %v", state, detail, err)
	}

	fake.hasCurl = false
	state, detail, err = s.stepTools(ctx, &provisionCtx{node: Node{ID: "n"}})
	if err != nil || state != StepDone || !strings.Contains(detail, "Installed") {
		t.Fatalf("curl install = %s %q %v", state, detail, err)
	}

	fake.hasCurl = false
	fake.curlErr = errors.New("pkg fail")
	_, _, err = s.stepTools(ctx, &provisionCtx{node: Node{ID: "n"}})
	if !errors.Is(err, ErrCurlMissing) {
		t.Fatalf("curl fail = %v", err)
	}
}

func TestStepDockerSkipped(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	state, detail, err := s.stepDocker(context.Background(), &provisionCtx{node: Node{ID: "n"}})
	if err != nil || state != StepSkipped || !strings.Contains(detail, "Docker version") {
		t.Fatalf("skip install = %s %q %v", state, detail, err)
	}
}

func TestStepDaemon(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	ctx := context.Background()

	state, _, err := s.stepDaemon(ctx, &provisionCtx{node: Node{ID: "n"}})
	if err != nil || state != StepSkipped {
		t.Fatalf("already up = %s %v", state, err)
	}

	fake.daemon = false
	p := &provisionCtx{node: Node{ID: "n"}, skipInstall: true}
	_, _, err = s.stepDaemon(ctx, p)
	if !errors.Is(err, ErrDockerNotRunning) {
		t.Fatalf("skipInstall down = %v", err)
	}

	p = &provisionCtx{node: Node{ID: "n"}}
	fake.hasCtl = false
	state, _, err = s.stepDaemon(ctx, p)
	if err != nil || state != StepWarned {
		t.Fatalf("no systemctl = %s %v", state, err)
	}

	fake.hasCtl = true
	state, _, err = s.stepDaemon(ctx, p)
	if err != nil || state != StepDone {
		t.Fatalf("started = %s %v", state, err)
	}

	fake.daemon = false
	s.rt.exec = func(_ context.Context, _ Node, _ time.Duration, argv []string, _ string) (string, error) {
		cmd := strings.Join(argv, " ")
		if strings.Contains(cmd, "ServerVersion") {
			return "", errors.New("Cannot connect to the Docker daemon")
		}
		if strings.Contains(cmd, "command -v systemctl") {
			return "/bin/systemctl", nil
		}
		if strings.Contains(cmd, "systemctl enable") {
			return "", errors.New("failed")
		}
		return "", errors.New("unexpected " + cmd)
	}
	_, _, err = s.stepDaemon(ctx, &provisionCtx{node: Node{ID: "n"}})
	if !errors.Is(err, ErrDockerNotRunning) {
		t.Fatalf("enable fail = %v", err)
	}
}

func TestStepAccess(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	ctx := context.Background()

	state, _, err := s.stepAccess(ctx, &provisionCtx{node: Node{Local: true}, sudo: "sudo -n"})
	if err != nil || state != StepSkipped {
		t.Fatalf("local skip = %s %v", state, err)
	}

	state, _, err = s.stepAccess(ctx, &provisionCtx{node: Node{Ssh: "no-user"}, sudo: "sudo -n"})
	if err != nil || state != StepSkipped {
		t.Fatalf("no user = %s %v", state, err)
	}

	p := &provisionCtx{node: Node{ID: "n", Ssh: "deploy@host"}, sudo: "sudo -n"}
	state, detail, err := s.stepAccess(ctx, p)
	if err != nil || state != StepSkipped || !strings.Contains(detail, "already") {
		t.Fatalf("in group = %s %q %v", state, detail, err)
	}

	fake.inGroup = false
	state, detail, err = s.stepAccess(ctx, p)
	if err != nil || state != StepDone || !strings.Contains(detail, "Added") {
		t.Fatalf("usermod = %s %q %v", state, detail, err)
	}

	fake.inGroup = false
	fake.usermodErr = errors.New("denied")
	state, _, err = s.stepAccess(ctx, p)
	if err != nil || state != StepWarned {
		t.Fatalf("usermod warn = %s %v", state, err)
	}
}

func TestStepPortsAndSwarm(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	worker := seedWorker(t, s, "w1", "edge")

	state, _, err := s.stepPorts(context.Background(), &provisionCtx{})
	if err != nil || state != StepSkipped {
		t.Fatalf("ports = %s %v", state, err)
	}

	state, _, err = s.stepSwarm(context.Background(), &provisionCtx{node: worker})
	if err != nil || state != StepDone {
		t.Fatalf("swarm = %s %v", state, err)
	}

	_, _, err = s.stepSwarm(context.Background(), &provisionCtx{node: Node{ID: "missing"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
}

func TestSnapshotCopiesSteps(t *testing.T) {
	s := testService(t)
	job := &ProvisionJob{NodeID: "n", State: ProvisionRunning, Steps: []StepResult{{Key: "reach", State: StepPending}}}
	clone := s.snapshot(job)
	clone.Steps[0].State = StepDone
	if job.Steps[0].State != StepPending {
		t.Fatal("snapshot aliased steps")
	}
}

func stepByKey(job ProvisionJob, key string) StepResult {
	for _, st := range job.Steps {
		if st.Key == key {
			return st
		}
	}
	return StepResult{}
}
