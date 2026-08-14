package vps

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	sshConnectTimeout = 10 * time.Second
	sshCommandTimeout = 30 * time.Second
)

// keys is the slice of the ssh key module this one depends on: resolving a key
// id to the private key file ssh should use. Declared here, on the consumer
// side, so the dependency stays one-way and explicit.
type keys interface {
	PrivateKeyPath(id string) (string, error)
}

// Service owns VPS records and the ssh calls made against them.
type Service struct {
	repo *Repository
	keys keys
	log  *slog.Logger
}

func NewService(repo *Repository, keys keys, log *slog.Logger) *Service {
	return &Service{repo: repo, keys: keys, log: log}
}

func (s *Service) List() ([]Vps, error) {
	return s.repo.List()
}

func (s *Service) Get(id string) (Vps, error) {
	return s.repo.Get(id)
}

func (s *Service) Create(req CreateRequest) (Vps, error) {
	if err := validateSsh(req.Ssh); err != nil {
		return Vps{}, err
	}
	if _, err := s.keys.PrivateKeyPath(req.SshKeyID); err != nil {
		return Vps{}, ErrSshKeyMissing
	}

	name := strings.TrimSpace(req.Name)
	taken, err := s.repo.ExistsByName(name, "")
	if err != nil {
		return Vps{}, err
	}
	if taken {
		return Vps{}, ErrNameTaken
	}

	item := Vps{
		ID:        newID(),
		Name:      name,
		Ssh:       strings.TrimSpace(req.Ssh),
		Port:      portOrDefault(req.Port),
		SshKeyID:  req.SshKeyID,
		KeyStatus: KeyStatusUnknown,
	}

	if err := s.repo.Create(&item); err != nil {
		return Vps{}, err
	}

	s.log.Info("vps created", "id", item.ID, "name", item.Name)
	return item, nil
}

func (s *Service) Update(id string, req UpdateRequest) (Vps, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return Vps{}, err
	}
	if err := validateSsh(req.Ssh); err != nil {
		return Vps{}, err
	}
	if _, err := s.keys.PrivateKeyPath(req.SshKeyID); err != nil {
		return Vps{}, ErrSshKeyMissing
	}

	name := strings.TrimSpace(req.Name)
	taken, err := s.repo.ExistsByName(name, id)
	if err != nil {
		return Vps{}, err
	}
	if taken {
		return Vps{}, ErrNameTaken
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
		return Vps{}, err
	}

	s.log.Info("vps updated", "id", item.ID, "name", item.Name)
	return item, nil
}

func (s *Service) Delete(id string) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.log.Info("vps deleted", "id", id)
	return nil
}

// CheckKey runs a no-op ssh command with the configured key and records whether
// the host accepted it.
func (s *Service) CheckKey(ctx context.Context, id string) (KeyCheckResult, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return KeyCheckResult{}, err
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

	s.log.Info("vps key checked", "id", item.ID, "ok", result.OK)
	return result, nil
}

// InstallKey runs ssh-copy-id once with the supplied password, then verifies the
// result with a key-only connection. It works on bare connection details, not a
// stored record, so the UI can install before saving. The password is never
// persisted.
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

	// ssh-copy-id prompts on a tty, so sshpass feeds it the password instead.
	if _, err := exec.LookPath("sshpass"); err != nil {
		return KeyCheckResult{}, ErrCopyIDMissing
	}
	if _, err := exec.LookPath("ssh-copy-id"); err != nil {
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
		return KeyCheckResult{OK: false, Message: firstLine(string(out), err)}, nil
	}

	// Installing is only meaningful if the key now works on its own.
	result := probeKey(ctx, keyPath, target, port)
	s.log.Info("ssh key installed", "ssh", target, "ok", result.OK)
	return result, nil
}

// probeKey runs a no-op command over ssh with key auth only — no prompts, no
// agent — so the result reflects the stored key and nothing else.
func probeKey(ctx context.Context, keyPath, target string, port int) KeyCheckResult {
	ctx, cancel := context.WithTimeout(ctx, sshCommandTimeout)
	defer cancel()

	args := []string{
		"-i", keyPath,
		"-p", strconv.Itoa(port),
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(sshConnectTimeout.Seconds())),
		target,
		"true",
	}

	out, err := exec.CommandContext(ctx, "ssh", args...).CombinedOutput()
	if err != nil {
		return KeyCheckResult{OK: false, Message: firstLine(string(out), err)}
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
