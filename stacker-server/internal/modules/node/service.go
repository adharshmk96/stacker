package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sshConnectTimeout = 10 * time.Second
	sshCommandTimeout = 30 * time.Second
)

// lookPath and sshProbe are the production binaries; tests assign stubs so they
// never hang on a real ssh timeout.
var lookPath = exec.LookPath

var sshProbe = defaultSshProbe

func defaultSshProbe(ctx context.Context, keyPath, target string, port int, connectTimeout time.Duration) (string, error) {
	args := []string{
		"-i", keyPath,
		"-p", strconv.Itoa(port),
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(connectTimeout.Seconds())),
		target,
		"true",
	}
	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	return string(out), err
}

// keys is the slice of the ssh key module this one depends on: resolving a key
// id to the private key file ssh should use. Declared here, on the consumer
// side, so the dependency stays one-way and explicit.
type keys interface {
	PrivateKeyPath(id string) (string, error)
}

// Service owns Node records and the ssh calls made against them.
type Service struct {
	repo *Repository
	keys keys
	log  *slog.Logger

	// rt runs docker commands on a node, locally or over ssh.
	rt runner

	// busy holds the ids with a swarm operation in flight, guarded by mu, so
	// two clients cannot join or promote the same node at once.
	mu   sync.Mutex
	busy map[string]bool

	// jobs holds the most recent provisioning run per node, so the UI can poll
	// a checklist that outlives the request which started it. In memory only:
	// every step re-checks before it acts, so a run lost to a restart is simply
	// run again.
	jobs map[string]*ProvisionJob

	// health is the last reachability reading per node, kept in memory so the
	// list can answer "is this host up?" without probing on every request.
	health *healthCache
}

func NewService(repo *Repository, keys keys, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		keys: keys,
		log:  log,
		rt:   runner{keys: keys},
		busy: map[string]bool{},
		jobs: map[string]*ProvisionJob{},

		health: newHealthCache(),
	}
}

func (s *Service) List() ([]Node, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		s.decorate(&items[i])
	}
	return items, nil
}

func (s *Service) Get(id string) (Node, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return Node{}, err
	}
	s.decorate(&item)
	return item, nil
}

func (s *Service) Create(req CreateRequest) (Node, error) {
	if err := validateSsh(req.Ssh); err != nil {
		return Node{}, err
	}
	if _, err := s.keys.PrivateKeyPath(req.SshKeyID); err != nil {
		return Node{}, ErrSshKeyMissing
	}

	name := strings.TrimSpace(req.Name)
	taken, err := s.repo.ExistsByName(name, "")
	if err != nil {
		return Node{}, err
	}
	if taken {
		return Node{}, ErrNameTaken
	}

	item := Node{
		ID:        newID(),
		Name:      name,
		Ssh:       strings.TrimSpace(req.Ssh),
		Port:      portOrDefault(req.Port),
		SshKeyID:  req.SshKeyID,
		KeyStatus: KeyStatusUnknown,
	}

	if err := s.repo.Create(&item); err != nil {
		return Node{}, err
	}

	s.log.Info("node created", "id", item.ID, "name", item.Name)
	return item, nil
}

// EnsureLocal seeds (or refreshes) the row for the machine stacker is running
// on, so installing stacker anywhere makes that machine show up as a node
// straight away. It is idempotent: the row is keyed by LocalID, and a user who
// renamed it keeps their name.
func (s *Service) EnsureLocal() (Node, error) {
	item, err := s.repo.Get(LocalID)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Node{}, err
	}

	name := localName()
	// A remote node may already own the hostname; suffix rather than fail,
	// since the local row must exist either way.
	for i := 2; ; i++ {
		taken, err := s.repo.ExistsByName(name, LocalID)
		if err != nil {
			return Node{}, err
		}
		if !taken {
			break
		}
		name = fmt.Sprintf("%s (%d)", localName(), i)
	}

	now := time.Now()
	item = Node{
		ID:    LocalID,
		Name:  name,
		Local: true,
		Port:  0,
		// The local machine needs no ssh hop, so there is no key to verify.
		KeyStatus:    KeyStatusOK,
		KeyCheckedAt: &now,
	}
	if err := s.repo.Create(&item); err != nil {
		return Node{}, err
	}

	s.log.Info("local node registered", "name", item.Name)
	return item, nil
}

func (s *Service) Update(id string, req UpdateRequest) (Node, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return Node{}, err
	}

	// The local node has no connection details to edit — only its name.
	if item.Local {
		return s.rename(item, req.Name)
	}

	if err := validateSsh(req.Ssh); err != nil {
		return Node{}, err
	}
	if _, err := s.keys.PrivateKeyPath(req.SshKeyID); err != nil {
		return Node{}, ErrSshKeyMissing
	}

	name := strings.TrimSpace(req.Name)
	taken, err := s.repo.ExistsByName(name, id)
	if err != nil {
		return Node{}, err
	}
	if taken {
		return Node{}, ErrNameTaken
	}

	// Changing where or how we connect invalidates the last check.
	if item.Ssh != strings.TrimSpace(req.Ssh) || item.Port != portOrDefault(req.Port) || item.SshKeyID != req.SshKeyID {
		item.KeyStatus = KeyStatusUnknown
		item.KeyCheckedAt = nil
	}

	item.Name = name
	item.Ssh = strings.TrimSpace(req.Ssh)
	item.Port = portOrDefault(req.Port)
	item.SshKeyID = req.SshKeyID

	if err := s.repo.Save(&item); err != nil {
		return Node{}, err
	}

	s.log.Info("node updated", "id", item.ID, "name", item.Name)
	return item, nil
}

// rename is the only edit the local node accepts.
func (s *Service) rename(item Node, rawName string) (Node, error) {
	name := strings.TrimSpace(rawName)
	taken, err := s.repo.ExistsByName(name, item.ID)
	if err != nil {
		return Node{}, err
	}
	if taken {
		return Node{}, ErrNameTaken
	}

	item.Name = name
	if err := s.repo.Save(&item); err != nil {
		return Node{}, err
	}
	return item, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	item, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	// Deleting it would be pointless: the next start seeds it again.
	if item.Local {
		return ErrLocalNode
	}

	// Forgetting a node stacker had joined would leave it running swarm tasks
	// with nothing left to administer it, so it has to leave first. Only *our*
	// swarm counts: a node sitting in someone else's swarm is nothing stacker
	// administers, and refusing to forget it would strand the row forever.
	if s.inOurSwarm(ctx, item) {
		return ErrNodeInSwarm
	}

	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.health.drop(id)
	s.log.Info("node deleted", "id", id)
	return nil
}

// CheckKey runs a no-op ssh command with the configured key and records whether
// the host accepted it.
func (s *Service) CheckKey(ctx context.Context, id string) (KeyCheckResult, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return KeyCheckResult{}, err
	}

	// Nothing to probe on the machine we are already running on.
	if item.Local {
		return KeyCheckResult{OK: true, Message: "This is the machine stacker runs on"}, nil
	}

	keyPath, err := s.keys.PrivateKeyPath(item.SshKeyID)
	if err != nil {
		return KeyCheckResult{}, ErrSshKeyMissing
	}

	result := probeKey(ctx, keyPath, item.Ssh, item.Port)

	now := time.Now()
	item.KeyCheckedAt = &now
	if result.OK {
		item.KeyStatus = KeyStatusOK
	} else {
		item.KeyStatus = KeyStatusFailed
	}
	if err := s.repo.Save(&item); err != nil {
		return KeyCheckResult{}, err
	}

	s.log.Info("node key checked", "id", item.ID, "ok", result.OK)
	return result, nil
}

// InstallKey installs the key with ssh-copy-id, then verifies the result with a
// key-only connection. It works on bare connection details, not a stored record,
// so the UI can install before saving. The password is never persisted.
//
// The host is probed first, so an install that has already happened costs one
// ssh round trip and changes nothing. That is what makes the call safe to
// repeat: a re-run needs no password (the UI's Install button stays useful
// after the host has disabled password auth, which is the usual thing to do
// once a key works), and it never has to reason about ssh-copy-id's own
// deduplication.
func (s *Service) InstallKey(ctx context.Context, req InstallKeyRequest) (KeyCheckResult, error) {
	if err := validateSsh(req.Ssh); err != nil {
		return KeyCheckResult{}, err
	}

	keyPath, err := s.keys.PrivateKeyPath(req.SshKeyID)
	if err != nil {
		return KeyCheckResult{}, ErrSshKeyMissing
	}
	target := strings.TrimSpace(req.Ssh)
	port := portOrDefault(req.Port)

	// Already installed: nothing to do, and nothing to ask the user for.
	if result := probeKey(ctx, keyPath, target, port); result.OK {
		s.log.Info("ssh key already installed", "ssh", target)
		return KeyCheckResult{OK: true, Message: "The key is already installed on this host"}, nil
	}

	// Only a genuine install needs the password, so it is checked here rather
	// than at the request binding, where it would block the no-op re-run above.
	if strings.TrimSpace(req.Password) == "" {
		return KeyCheckResult{}, ErrPasswordRequired
	}

	// ssh-copy-id prompts on a tty, so sshpass feeds it the password instead.
	if _, err := lookPath("sshpass"); err != nil {
		return KeyCheckResult{}, ErrCopyIDMissing
	}
	if _, err := lookPath("ssh-copy-id"); err != nil {
		return KeyCheckResult{}, ErrCopyIDMissing
	}

	ctx, cancel := context.WithTimeout(ctx, sshCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sshpass", "-e", "ssh-copy-id",
		"-i", keyPath+".pub",
		"-p", strconv.Itoa(port),
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(sshConnectTimeout.Seconds())),
		target,
	)
	// `sshpass -e` reads the password from SSHPASS so it never lands in argv,
	// where any local process could read it off the process list.
	cmd.Env = append(cmd.Environ(), "SSHPASS="+req.Password)

	if out, err := cmd.CombinedOutput(); err != nil {
		s.log.Warn("ssh-copy-id failed", "ssh", target)
		return KeyCheckResult{OK: false, Message: copyIDError(string(out), err)}, nil
	}

	// Installing is only meaningful if the key now works on its own.
	result := probeKey(ctx, keyPath, target, port)
	s.log.Info("ssh key installed", "ssh", target, "ok", result.OK)
	return result, nil
}

// probeKey runs a no-op command over ssh with key auth only — no prompts, no
// agent — so the result reflects the stored key and nothing else.
func probeKey(ctx context.Context, keyPath, target string, port int) KeyCheckResult {
	return probeWith(ctx, keyPath, target, port, sshConnectTimeout)
}

// probeHost is the same round trip with a shorter patience, used by the
// reachability sweep: the table only asks whether the host answers, and a slow
// host holds up the whole column.
func probeHost(ctx context.Context, keyPath, target string, port int) KeyCheckResult {
	return probeWith(ctx, keyPath, target, port, pingConnectTimeout)
}

func probeWith(ctx context.Context, keyPath, target string, port int, connectTimeout time.Duration) KeyCheckResult {
	ctx, cancel := context.WithTimeout(ctx, sshCommandTimeout)
	defer cancel()

	out, err := sshProbe(ctx, keyPath, target, port, connectTimeout)
	if err != nil {
		return KeyCheckResult{OK: false, Message: firstLine(out, err)}
	}
	return KeyCheckResult{OK: true, Message: "Key authentication works"}
}

func portOrDefault(port int) int {
	if port == 0 {
		return 22
	}
	return port
}

// firstLine keeps API messages to the single most useful line of ssh output.
func firstLine(output string, fallback error) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return fallback.Error()
}

// copyIDError picks the line of an ssh-copy-id failure that says what went
// wrong. Plain firstLine cannot: ssh-copy-id always opens with an
// `INFO: Source of key(s) to be installed:` banner on stderr, so every failure
// looked identical and told the user nothing. The real cause — a refused
// password, a changed host key — comes later in the output.
func copyIDError(output string, fallback error) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// A changed host key aborts before authentication and prints a banner the
	// user cannot act on without being told what it means.
	for _, line := range lines {
		if strings.Contains(line, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
			return "the host key has changed since this host was last seen — if that was expected " +
				"(the box was rebuilt), remove the old entry with `ssh-keygen -R` and try again"
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.Contains(line, "INFO:"), strings.HasPrefix(line, "@"):
			continue
		case strings.Contains(line, "Permission denied"):
			return "the host refused the password"
		}
		return line
	}
	return fallback.Error()
}
