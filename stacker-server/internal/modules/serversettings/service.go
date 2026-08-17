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
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

var (
	ErrInvalidDomain = errors.New("enter a valid hostname without a scheme or path")
	ErrConfigMissing = errors.New("traefik configuration is not available on this installation")
	ErrUnknownTarget = errors.New("restart target must be stacker or traefik")
	hostRule         = regexp.MustCompile(`Host\x28\x60([^\x60]+)\x60\x29`)
	stackerRuleLine  = regexp.MustCompile(`(?m)(^    stacker:\r?\n(?:      .*\r?\n)*?      rule:\s*["']?)Host\x28\x60([^\x60]+)\x60\x29(["']?\s*)$`)
	labelPattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

// These are filled by build flags. Development builds deliberately say development.
var Version = "development"
var BuiltAt = ""

type dockerInfo struct {
	ServerVersion   string
	OperatingSystem string
}

type dockerService struct {
	Spec struct {
		Name         string
		TaskTemplate struct {
			ContainerSpec struct{ Image string }
		}
	}
	UpdatedAt     time.Time
	ServiceStatus struct {
		RunningTasks int
		DesiredTasks int
	}
}

type dockerServiceListItem struct {
	Name     string
	Replicas string
}

type dynamicConfig struct {
	HTTP struct {
		Routers map[string]struct {
			Rule        string
			EntryPoints []string `yaml:"entryPoints"`
			Service     string
			TLS         *struct {
				CertResolver string `yaml:"certResolver"`
			}
		}
		Services map[string]struct {
			LoadBalancer struct {
				Servers []struct{ URL string }
			} `yaml:"loadBalancer"`
		}
	} `yaml:"http"`
}

type staticConfig struct {
	EntryPoints map[string]struct {
		Address string
		HTTP    struct {
			Redirections struct {
				EntryPoint struct {
					To     string
					Scheme string
				} `yaml:"entryPoint"`
			}
		}
	} `yaml:"entryPoints"`
}

type command func(context.Context, string, ...string) ([]byte, error)

type Service struct {
	configPath    string
	stackName     string
	advertiseAddr string
	startedAt     time.Time
	run           command
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

	traefik, err := s.readTraefik()
	if err != nil && !errors.Is(err, ErrConfigMissing) {
		return Settings{}, err
	}
	traefik.StackName = s.stackName
	traefik.StackerService = s.readService(ctx, "stacker")
	traefik.TraefikService = s.readService(ctx, "traefik")

	instance := Instance{Hostname: hostname, IP: s.advertiseAddr, Version: Version, BuiltAt: BuiltAt, StartedAt: s.startedAt}
	if output, err := s.run(ctx, "docker", "info", "--format", "{{json .}}"); err == nil {
		var info dockerInfo
		if json.Unmarshal(output, &info) == nil {
			instance.Docker = info.ServerVersion
			instance.OS = info.OperatingSystem
		}
	}
	return Settings{Instance: instance, Traefik: traefik}, nil
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
	if !stackerRuleLine.Match(content) {
		return "", fmt.Errorf("%w: host rule is missing", ErrConfigMissing)
	}

	updated := stackerRuleLine.ReplaceAll(content, []byte("${1}Host(`"+domain+"`)${3}"))
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

func (s *Service) readTraefik() (TraefikInfo, error) {
	content, err := os.ReadFile(s.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return TraefikInfo{}, ErrConfigMissing
	}
	if err != nil {
		return TraefikInfo{}, err
	}
	var dynamic dynamicConfig
	if err := yaml.Unmarshal(content, &dynamic); err != nil {
		return TraefikInfo{}, fmt.Errorf("read traefik dynamic config: %w", err)
	}
	router, ok := dynamic.HTTP.Routers["stacker"]
	if !ok {
		return TraefikInfo{}, ErrConfigMissing
	}
	match := hostRule.FindStringSubmatch(router.Rule)
	if len(match) != 2 {
		return TraefikInfo{}, ErrConfigMissing
	}

	info := TraefikInfo{
		Domain:              match[1],
		HTTPS:               router.TLS != nil && contains(router.EntryPoints, "websecure"),
		CertificateResolver: "",
		StackName:           s.stackName,
		PublishedPorts:      []string{},
	}
	if router.TLS != nil {
		info.CertificateResolver = router.TLS.CertResolver
	}
	if backend, ok := dynamic.HTTP.Services[router.Service]; ok && len(backend.LoadBalancer.Servers) > 0 {
		info.BackendTarget = backend.LoadBalancer.Servers[0].URL
	}

	staticPath := filepath.Join(filepath.Dir(filepath.Dir(s.configPath)), "traefik.yml")
	if staticContent, readErr := os.ReadFile(staticPath); readErr == nil {
		var static staticConfig
		if yaml.Unmarshal(staticContent, &static) == nil {
			web := static.EntryPoints["web"]
			info.HTTPRedirect = web.HTTP.Redirections.EntryPoint.To == "websecure" && web.HTTP.Redirections.EntryPoint.Scheme == "https"
			for _, name := range []string{"web", "websecure"} {
				if address := static.EntryPoints[name].Address; address != "" {
					info.PublishedPorts = append(info.PublishedPorts, address)
				}
			}
		}
	}
	return info, nil
}

func (s *Service) readService(ctx context.Context, target string) ServiceInfo {
	name := s.stackName + "_" + target
	info := ServiceInfo{Name: name, Status: "unavailable"}
	output, err := s.run(ctx, "docker", "service", "inspect", name, "--format", "{{json .}}")
	if err != nil {
		return info
	}
	var service dockerService
	if json.Unmarshal(output, &service) != nil {
		return info
	}
	info.Image = service.Spec.TaskTemplate.ContainerSpec.Image
	info.UpdatedAt = service.UpdatedAt
	info.Running, info.Desired = s.readReplicas(ctx, name)
	info.Status = "degraded"
	if info.Desired > 0 && info.Running == info.Desired {
		info.Status = "healthy"
	}
	if target == "traefik" {
		image := strings.Split(info.Image, "@")[0]
		if _, version, found := strings.Cut(image, ":"); found {
			info.Version = version
		}
	}
	return info
}

func (s *Service) readReplicas(ctx context.Context, name string) (int, int) {
	output, err := s.run(ctx, "docker", "service", "ls", "--filter", "name="+name, "--format", "{{json .}}")
	if err != nil {
		return 0, 0
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		var item dockerServiceListItem
		if json.Unmarshal([]byte(line), &item) != nil || item.Name != name {
			continue
		}
		parts := strings.SplitN(item.Replicas, "/", 2)
		if len(parts) != 2 {
			return 0, 0
		}
		running, runningErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		desired, desiredErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if runningErr != nil || desiredErr != nil {
			return 0, 0
		}
		return running, desired
	}
	return 0, 0
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
