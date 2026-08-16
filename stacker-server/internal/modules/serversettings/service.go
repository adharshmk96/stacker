package serversettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidDomain = errors.New("enter a valid hostname without a scheme or path")
	ErrConfigMissing = errors.New("traefik configuration is not available on this installation")
	ErrUnknownTarget = errors.New("restart target must be stacker or traefik")
	hostRule         = regexp.MustCompile(`Host\x28\x60([^\x60]+)\x60\x29`)
	labelPattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

// These are filled by build flags. Development builds deliberately say development.
var Version = "development"
var BuiltAt = ""

type dockerInfo struct {
	ServerVersion   string
	OperatingSystem string
}

type command func(context.Context, string, ...string) ([]byte, error)

type Service struct {
	configPath string
	stackName  string
	startedAt  time.Time
	run        command
}

func NewService(configPath, stackName string) *Service {
	return &Service{
		configPath: configPath,
		stackName:  stackName,
		startedAt:  time.Now().UTC(),
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
	}
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}

	domain, err := s.readDomain()
	if err != nil && !errors.Is(err, ErrConfigMissing) {
		return Settings{}, err
	}

	instance := Instance{Hostname: hostname, Version: Version, BuiltAt: BuiltAt, StartedAt: s.startedAt}
	if output, err := s.run(ctx, "docker", "info", "--format", "{{json .}}"); err == nil {
		var info dockerInfo
		if json.Unmarshal(output, &info) == nil {
			instance.Docker = info.ServerVersion
			instance.OS = info.OperatingSystem
		}
	}
	return Settings{Instance: instance, Domain: domain}, nil
}

func (s *Service) UpdateDomain(domain string) (string, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !validDomain(domain) {
		return "", ErrInvalidDomain
	}

	content, err := os.ReadFile(s.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrConfigMissing
	}
	if err != nil {
		return "", err
	}
	if !hostRule.Match(content) {
		return "", fmt.Errorf("%w: host rule is missing", ErrConfigMissing)
	}

	updated := hostRule.ReplaceAll(content, []byte("Host(`"+domain+"`)"))
	tmp, err := os.CreateTemp(filepath.Dir(s.configPath), ".stacker-domain-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // renamed on success

	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(updated)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, s.configPath); err != nil {
		return "", err
	}
	return domain, nil
}

func (s *Service) Restart(ctx context.Context, target string) error {
	if target != "stacker" && target != "traefik" {
		return ErrUnknownTarget
	}
	service := s.stackName + "_" + target
	output, err := s.run(ctx, "docker", "service", "update", "--force", "--detach=true", service)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("could not restart %s: %s", target, message)
		}
		return fmt.Errorf("could not restart %s: %w", target, err)
	}
	return nil
}

func (s *Service) readDomain() (string, error) {
	content, err := os.ReadFile(s.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrConfigMissing
	}
	if err != nil {
		return "", err
	}
	match := hostRule.FindSubmatch(content)
	if len(match) != 2 {
		return "", ErrConfigMissing
	}
	return string(match[1]), nil
}

func validDomain(value string) bool {
	if len(value) == 0 || len(value) > 253 || strings.HasSuffix(value, ".") {
		return false
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !labelPattern.MatchString(label) {
			return false
		}
	}
	return true
}
