package node

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&Node{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

type stubKeys struct {
	path string
	err  error
}

func (k stubKeys) PrivateKeyPath(string) (string, error) {
	if k.err != nil {
		return "", k.err
	}
	if k.path == "" {
		return "/tmp/stacker-test-key", nil
	}
	return k.path, nil
}

func testService(t *testing.T) *Service {
	t.Helper()
	return NewService(NewRepository(testDB(t)), stubKeys{}, silentLog())
}

func testRouter(t *testing.T, s *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	m := &Module{Service: s, handler: NewHandler(s)}
	m.RegisterRoutes(r.Group("/api"))
	return r
}

func seedManager(t *testing.T, s *Service) Node {
	t.Helper()
	now := time.Now()
	item := Node{
		ID:           LocalID,
		Name:         "local",
		Local:        true,
		KeyStatus:    KeyStatusOK,
		KeyCheckedAt: &now,
		SwarmRole:    SwarmRoleManager,
		SwarmNodeID:  "mgr1",
		SwarmAddr:    "192.168.1.10",
	}
	if err := s.repo.Create(&item); err != nil {
		t.Fatalf("seed manager: %v", err)
	}
	return item
}

func seedWorker(t *testing.T, s *Service, id, name string) Node {
	t.Helper()
	item := Node{
		ID:        id,
		Name:      name,
		Ssh:       "deploy@10.0.0.2",
		Port:      22,
		SshKeyID:  "key1",
		KeyStatus: KeyStatusOK,
	}
	if err := s.repo.Create(&item); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	return item
}

func waitProvision(t *testing.T, s *Service, id string) ProvisionJob {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		job, err := s.ProvisionStatus(id)
		if err == nil && job.State != ProvisionRunning {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("provision did not finish: err=%v job=%+v", err, job)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func stubProbe(t *testing.T, ok bool, msg string) {
	t.Helper()
	orig := sshProbe
	t.Cleanup(func() { sshProbe = orig })
	sshProbe = func(context.Context, string, string, int, time.Duration) (string, error) {
		if ok {
			return "", nil
		}
		if msg == "" {
			msg = "Permission denied"
		}
		return msg, fmt.Errorf("exit status 255")
	}
}

func stubLookPath(t *testing.T, present map[string]bool) {
	t.Helper()
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(file string) (string, error) {
		if present[file] {
			return "/usr/bin/" + file, nil
		}
		return "", fmt.Errorf("executable file not found in $PATH")
	}
}

type cmdFail struct {
	contains string
	out      string
	err      error
}

// rtFake is the fake runner.exec used by swarm and provision tests.
type rtFake struct {
	mu sync.Mutex

	uname      string
	distro     string
	uid        string
	hasCurl    bool
	hasDocker  bool
	hasCtl     bool
	daemon     bool
	inGroup    bool
	usermodErr error
	curlErr    error

	token     string
	states    map[string]swarmState
	useRoster bool
	roster    []string

	onCmd func(item Node, argv []string, stdin string) (string, error, bool)
	fails []cmdFail

	lastStdin string
}

func newRTFake() *rtFake {
	return &rtFake{
		uname:     "Linux",
		distro:    "Debian GNU/Linux 12 (bookworm)",
		uid:       "0",
		hasCurl:   true,
		hasDocker: true,
		hasCtl:    true,
		daemon:    true,
		inGroup:   true,
		token:     "SWMTKN-1-test",
		states:    map[string]swarmState{},
	}
}

func (f *rtFake) attach(s *Service) {
	s.rt.exec = f.exec
}

func (f *rtFake) setRole(id string, role SwarmRole, dockerID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[id] = roleState(role, dockerID)
}

func roleState(role SwarmRole, dockerID string) swarmState {
	switch role {
	case SwarmRoleManager:
		return swarmState{LocalNodeState: "active", NodeID: dockerID, ControlAvailable: true}
	case SwarmRoleWorker:
		return swarmState{LocalNodeState: "active", NodeID: dockerID, ControlAvailable: false}
	default:
		return swarmState{LocalNodeState: "inactive"}
	}
}

func (f *rtFake) exec(_ context.Context, item Node, _ time.Duration, argv []string, stdin string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastStdin = stdin
	cmd := strings.Join(argv, " ")

	if f.onCmd != nil {
		if out, err, handled := f.onCmd(item, argv, stdin); handled {
			return out, err
		}
	}
	for _, fail := range f.fails {
		if strings.Contains(cmd, fail.contains) {
			return fail.out, fail.err
		}
	}

	switch {
	case len(argv) > 0 && argv[0] == "uname":
		if f.uname == "" {
			return "Linux", nil
		}
		return f.uname, nil
	case strings.Contains(cmd, "os-release"):
		return f.distro, nil
	case len(argv) >= 2 && argv[0] == "id" && argv[1] == "-u":
		return f.uid, nil
	case strings.Contains(cmd, "sudo -n true"):
		return "", nil
	case strings.Contains(cmd, "command -v curl"):
		if f.hasCurl {
			return "/usr/bin/curl", nil
		}
		return "", fmt.Errorf("curl: not found")
	case strings.Contains(cmd, "command -v systemctl"):
		if f.hasCtl {
			return "/usr/bin/systemctl", nil
		}
		return "", fmt.Errorf("systemctl: not found")
	case strings.Contains(cmd, "apt-get") || strings.Contains(cmd, "dnf install") || strings.Contains(cmd, "yum install"):
		if f.curlErr != nil {
			return "", f.curlErr
		}
		f.hasCurl = true
		return "installed", nil
	case len(argv) >= 2 && argv[0] == "docker" && argv[1] == "--version":
		if f.hasDocker {
			return "Docker version 27.0.3", nil
		}
		return "", fmt.Errorf("docker: command not found")
	case strings.Contains(cmd, "ServerVersion"):
		if f.daemon {
			return "27.0.3", nil
		}
		return "", fmt.Errorf("Cannot connect to the Docker daemon")
	case strings.Contains(cmd, "LocalNodeState"):
		st := f.states[item.ID]
		if st.LocalNodeState == "" {
			st = swarmState{LocalNodeState: "inactive"}
		}
		flag := "false"
		if st.ControlAvailable {
			flag = "true"
		}
		return st.LocalNodeState + "\t" + st.NodeID + "\t" + flag, nil
	case strings.Contains(cmd, "join-token"):
		return f.token, nil
	case strings.Contains(cmd, "swarm join"):
		id := "d-" + item.ID
		f.states[item.ID] = roleState(SwarmRoleWorker, id)
		return "This node joined a swarm as a worker.", nil
	case strings.Contains(cmd, "swarm leave"):
		f.states[item.ID] = roleState(SwarmRoleNone, "")
		return "", nil
	case strings.Contains(cmd, "node promote"):
		dockerID := argv[len(argv)-1]
		for k, st := range f.states {
			if st.NodeID == dockerID {
				f.states[k] = roleState(SwarmRoleManager, dockerID)
			}
		}
		return "", nil
	case strings.Contains(cmd, "node demote"):
		dockerID := argv[len(argv)-1]
		for k, st := range f.states {
			if st.NodeID == dockerID {
				f.states[k] = roleState(SwarmRoleWorker, dockerID)
			}
		}
		return "", nil
	case strings.Contains(cmd, "node ls"):
		if f.useRoster {
			return strings.Join(f.roster, "\n"), nil
		}
		var ids []string
		for _, st := range f.states {
			if st.NodeID != "" && st.LocalNodeState == "active" {
				ids = append(ids, st.NodeID)
			}
		}
		return strings.Join(ids, "\n"), nil
	case strings.Contains(cmd, "node rm"):
		return "", nil
	case strings.Contains(cmd, "id -nG"):
		if f.inGroup {
			return "docker", nil
		}
		return "", fmt.Errorf("exit status 1")
	case strings.Contains(cmd, "usermod"):
		if f.usermodErr != nil {
			return "", f.usermodErr
		}
		f.inGroup = true
		return "", nil
	case strings.Contains(cmd, "systemctl enable"):
		f.daemon = true
		return "", nil
	}

	return "", fmt.Errorf("unexpected command %q on node %s", cmd, item.ID)
}
