package serversettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
)

var (
	ErrInvalidDomain        = errors.New("enter a valid hostname without a scheme or path")
	ErrConfigMissing        = errors.New("traefik configuration is not available on this installation")
	ErrUnknownTarget        = errors.New("restart target must be stacker or traefik")
	ErrInvalidUpdateChannel = errors.New("update channel must be stable or edge")
	ErrNoUpdate             = errors.New("the selected update is not available")
	ErrUpdateInProgress     = errors.New("a server update is already in progress")
	ErrUpdatesUnavailable   = errors.New("could not check for server updates")
	hostRule                = regexp.MustCompile(`Host\x28\x60([^\x60]+)\x60\x29`)
	stackerRuleLine         = regexp.MustCompile(`(?m)(^    stacker:\r?\n(?:      .*\r?\n)*?      rule:\s*["']?)Host\x28\x60([^\x60]+)\x60\x29(["']?\s*)$`)
	labelPattern            = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

// These are filled by build flags. Development builds deliberately say development.
var Version = "development"
var BuiltAt = ""
var Revision = ""
var RepositoryURL = "https://github.com/adharshmk96/stacker.git"

const githubAPI = "https://api.github.com"

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

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

type githubCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

type Service struct {
	configPath    string
	stackName     string
	advertiseAddr string
	startedAt     time.Time
	run           command
	client        *http.Client
	updateMu      sync.Mutex
	updating      bool
	updateErr     string
}

func NewService(configPath, stackName string) *Service {
	return &Service{
		configPath: configPath,
		stackName:  stackName,
		startedAt:  time.Now().UTC(),
		client:     &http.Client{Timeout: 15 * time.Second},
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

	instance := Instance{Hostname: hostname, IP: s.advertiseAddr, Version: Version, BuiltAt: BuiltAt, StartedAt: s.startedAt, Revision: Revision, Repository: RepositoryURL}
	if output, err := s.run(ctx, "docker", "info", "--format", "{{json .}}"); err == nil {
		var info dockerInfo
		if json.Unmarshal(output, &info) == nil {
			instance.Docker = info.ServerVersion
			instance.OS = info.OperatingSystem
		}
	}
	return Settings{Instance: instance, Traefik: traefik}, nil
}

// Updates checks the configured GitHub repository for the latest stable tag
// and main branch commit. It does not mutate the installation.
func (s *Service) Updates(ctx context.Context) (Updates, error) {
	repository, err := githubRepository(RepositoryURL)
	if err != nil {
		return Updates{}, fmt.Errorf("%w: %v", ErrUpdatesUnavailable, err)
	}
	stable, err := s.latestStable(ctx, repository)
	if err != nil {
		return Updates{}, err
	}
	edge, err := s.latestEdge(ctx, repository)
	if err != nil {
		return Updates{}, err
	}
	s.updateMu.Lock()
	updating := s.updating
	updateErr := s.updateErr
	s.updateMu.Unlock()
	return Updates{Stable: stable, Edge: edge, Updating: updating, Error: updateErr}, nil
}

func (s *Service) latestStable(ctx context.Context, repository string) (UpdateCandidate, error) {
	var releases []githubRelease
	if err := s.githubGet(ctx, "/repos/"+repository+"/releases?per_page=100", &releases); err != nil {
		return UpdateCandidate{}, err
	}
	for _, release := range releases {
		if release.Draft || release.Prerelease || strings.TrimSpace(release.TagName) == "" {
			continue
		}
		candidate := UpdateCandidate{Channel: "stable", Version: release.TagName, PublishedAt: release.PublishedAt}
		// A different channel is intentionally treated as a switch, which the UI
		// spells out in its confirmation dialog.
		candidate.Available = Version != release.TagName
		return candidate, nil
	}
	return UpdateCandidate{}, fmt.Errorf("%w: no published stable release", ErrUpdatesUnavailable)
}

func (s *Service) latestEdge(ctx context.Context, repository string) (UpdateCandidate, error) {
	var commit githubCommit
	if err := s.githubGet(ctx, "/repos/"+repository+"/commits/main", &commit); err != nil {
		return UpdateCandidate{}, err
	}
	if strings.TrimSpace(commit.SHA) == "" {
		return UpdateCandidate{}, fmt.Errorf("%w: main has no revision", ErrUpdatesUnavailable)
	}
	candidate := UpdateCandidate{Channel: "edge", Version: "main", Revision: commit.SHA, PublishedAt: commit.Commit.Committer.Date}
	if Revision != "" && Revision != "unknown" {
		candidate.Available = !strings.EqualFold(Revision, commit.SHA)
		return candidate, nil
	}
	// Builds made before revision metadata existed can still make a safe best
	// effort comparison: a commit authored after the build cannot be included.
	if builtAt, err := time.Parse(time.RFC3339, BuiltAt); err == nil {
		candidate.Available = commit.Commit.Committer.Date.After(builtAt)
	}
	return candidate, nil
}

func (s *Service) githubGet(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPI+path, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpdatesUnavailable, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpdatesUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: GitHub returned %s", ErrUpdatesUnavailable, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("%w: invalid GitHub response", ErrUpdatesUnavailable)
	}
	return nil
}

// StartUpdate re-checks availability before the background rebuild begins, so
// callers cannot submit a stale or arbitrary Git ref.
func (s *Service) StartUpdate(ctx context.Context, channel string) error {
	if channel != "stable" && channel != "edge" {
		return ErrInvalidUpdateChannel
	}
	updates, err := s.Updates(ctx)
	if err != nil {
		return err
	}
	candidate := updates.Stable
	if channel == "edge" {
		candidate = updates.Edge
	}
	if !candidate.Available {
		return ErrNoUpdate
	}

	s.updateMu.Lock()
	if s.updating {
		s.updateMu.Unlock()
		return ErrUpdateInProgress
	}
	s.updating = true
	s.updateErr = ""
	s.updateMu.Unlock()

	go func() {
		defer func() {
			s.updateMu.Lock()
			s.updating = false
			s.updateMu.Unlock()
		}()
		// The service is intentionally replaced near the end of this work. If it
		// succeeds, the new process owns subsequent status requests.
		if err := s.runUpdate(context.Background(), candidate); err != nil {
			s.updateMu.Lock()
			s.updateErr = err.Error()
			s.updateMu.Unlock()
		}
	}()
	return nil
}

func (s *Service) runUpdate(ctx context.Context, candidate UpdateCandidate) error {
	workdir, err := os.MkdirTemp("", "stacker-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workdir) //nolint:errcheck // best-effort after redeploy

	repoDir := filepath.Join(workdir, "source")
	if _, err := s.run(ctx, "git", "clone", "--quiet", "--depth", "1", "--branch", candidate.Version, RepositoryURL, repoDir); err != nil {
		return fmt.Errorf("clone update source: %w", err)
	}
	output, err := s.run(ctx, "git", "-C", repoDir, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return errors.New("could not resolve the downloaded update revision")
	}
	resolvedRevision := strings.TrimSpace(string(output))
	if candidate.Channel == "edge" {
		if !strings.EqualFold(resolvedRevision, candidate.Revision) {
			return errors.New("main changed while preparing the update; check again")
		}
	}
	candidate.Revision = resolvedRevision

	builtAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.run(ctx, "docker", "build", "--pull",
		"--build-arg", "STACKER_VERSION="+candidate.Version,
		"--build-arg", "STACKER_BUILT_AT="+builtAt,
		"--build-arg", "STACKER_REVISION="+candidate.Revision,
		"--build-arg", "STACKER_REPOSITORY_URL="+RepositoryURL,
		"-t", "stacker:local", repoDir); err != nil {
		return fmt.Errorf("build server update: %w", err)
	}

	stack, err := os.ReadFile(filepath.Join(repoDir, "deploy", "stack.yml"))
	if err != nil {
		return fmt.Errorf("read update stack manifest: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return errors.New("could not determine server hostname")
	}
	rendered := strings.NewReplacer(
		"__STACKER_IMAGE__", "stacker:local",
		"__STACKER_NODE_NAME__", hostname,
		"__STACKER_ADVERTISE_ADDR__", s.advertiseAddr,
		"__STACKER_STACK_NAME__", s.stackName,
	).Replace(string(stack))
	stackPath := filepath.Join(workdir, "stack.yml")
	if err := os.WriteFile(stackPath, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write update stack manifest: %w", err)
	}
	if _, err := s.run(ctx, "docker", "stack", "deploy", "--detach=true", "--resolve-image", "never", "-c", stackPath, s.stackName); err != nil {
		return fmt.Errorf("deploy server update: %w", err)
	}
	// Keep the installer behaviour: local tags do not change the Swarm service
	// spec by themselves, so force both managed services to pick up the build.
	if _, err := s.run(ctx, "docker", "service", "update", "--force", "--detach=true", s.stackName+"_traefik"); err != nil {
		return fmt.Errorf("restart Traefik after update: %w", err)
	}
	_, err = s.run(ctx, "docker", "service", "update", "--force", "--detach=true", s.stackName+"_stacker")
	return err
}

func githubRepository(raw string) (string, error) {
	value := strings.TrimSuffix(strings.TrimSpace(raw), ".git")
	if strings.HasPrefix(value, "git@github.com:") {
		value = "https://github.com/" + strings.TrimPrefix(value, "git@github.com:")
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", errors.New("updates require a github.com repository")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("repository URL must name owner/repository")
	}
	return parts[0] + "/" + parts[1], nil
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
