package serversettings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestGetReadsInstalledDomainAndDockerInfo(t *testing.T) {
	path := writeConfig(t, "stacker.203.0.113.10.sslip.io")
	service := NewService(path, "stacker")
	service.advertiseAddr = "203.0.113.10"
	service.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"ServerVersion":"27.5.1","OperatingSystem":"Ubuntu 24.04 LTS"}`), nil
	}

	result, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Traefik.Domain != "stacker.203.0.113.10.sslip.io" {
		t.Fatalf("domain = %q", result.Traefik.Domain)
	}
	if result.Instance.IP != "203.0.113.10" {
		t.Fatalf("ip = %q", result.Instance.IP)
	}
	if result.Instance.Docker != "27.5.1" || result.Instance.OS != "Ubuntu 24.04 LTS" {
		t.Fatalf("instance = %#v", result.Instance)
	}
}

func TestGetReadsOnlyStackerRoutingAndServiceState(t *testing.T) {
	root := t.TempDir()
	dynamicDir := filepath.Join(root, "dynamic")
	if err := os.Mkdir(dynamicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dynamic := `http:
  routers:
    other:
      rule: "Host(` + "`other.example.com`" + `)"
    stacker:
      rule: "Host(` + "`stacker.203.0.113.10.sslip.io`" + `)"
      entryPoints: [websecure]
      service: stacker
      tls:
        certResolver: letsencrypt
  services:
    stacker:
      loadBalancer:
        servers:
          - url: http://stacker:8080
`
	path := filepath.Join(dynamicDir, "stacker.yml")
	if err := os.WriteFile(path, []byte(dynamic), 0o644); err != nil {
		t.Fatal(err)
	}
	static := "entryPoints:\n  web:\n    address: ':80'\n    http:\n      redirections:\n        entryPoint:\n          to: websecure\n          scheme: https\n  websecure:\n    address: ':443'\n"
	if err := os.WriteFile(filepath.Join(root, "traefik.yml"), []byte(static), 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService(path, "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "info" {
			return []byte(`{"ServerVersion":"27.5.1","OperatingSystem":"Ubuntu"}`), nil
		}
		if args[1] == "ls" {
			name := args[3][len("name="):]
			return []byte(`{"Name":"` + name + `","Replicas":"1/1"}`), nil
		}
		name := args[2]
		image := "stacker:local"
		if name == "stacker_traefik" {
			image = "traefik:v3.6"
		}
		return []byte(`{"Spec":{"Name":"` + name + `","TaskTemplate":{"ContainerSpec":{"Image":"` + image + `"}}},"UpdatedAt":"2026-08-17T00:00:00Z","ServiceStatus":{"RunningTasks":1,"DesiredTasks":1}}`), nil
	}

	result, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	info := result.Traefik
	if info.Domain != "stacker.203.0.113.10.sslip.io" || !info.HTTPS || !info.HTTPRedirect {
		t.Fatalf("traefik = %#v", info)
	}
	if info.CertificateResolver != "letsencrypt" || info.BackendTarget != "http://stacker:8080" {
		t.Fatalf("traefik = %#v", info)
	}
	if info.TraefikService.Version != "v3.6" || info.TraefikService.Status != "healthy" {
		t.Fatalf("service = %#v", info.TraefikService)
	}
}

func TestReadServiceGetsReplicasFromServiceList(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[1] == "inspect" {
			return []byte(`{"Spec":{"Name":"stacker_stacker","TaskTemplate":{"ContainerSpec":{"Image":"stacker:local"}}}}`), nil
		}
		return []byte("{\"Name\":\"another_service\",\"Replicas\":\"3/3\"}\n{\"Name\":\"stacker_stacker\",\"Replicas\":\"1/1\"}"), nil
	}

	info := service.readService(context.Background(), "stacker")
	if info.Running != 1 || info.Desired != 1 || info.Status != "healthy" {
		t.Fatalf("service = %#v", info)
	}
}

func TestReadServiceDegradesWhenReplicaStateIsInvalid(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[1] == "inspect" {
			return []byte(`{"Spec":{"Name":"stacker_stacker"}}`), nil
		}
		return []byte(`{"Name":"stacker_stacker","Replicas":"pending"}`), nil
	}

	info := service.readService(context.Background(), "stacker")
	if info.Running != 0 || info.Desired != 0 || info.Status != "degraded" {
		t.Fatalf("service = %#v", info)
	}
}

func TestUpdateDomainRewritesOnlyHostRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stacker.yml")
	original := "http:\n  routers:\n    other:\n      rule: \"Host(`other.example.com`)\"\n    stacker:\n      rule: \"Host(`stacker.203.0.113.10.sslip.io`)\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(path, "stacker")

	domain, err := service.UpdateDomain(" Dashboard.Example.COM ")
	if err != nil {
		t.Fatal(err)
	}
	if domain != "dashboard.example.com" {
		t.Fatalf("domain = %q", domain)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "http:\n  routers:\n    other:\n      rule: \"Host(`other.example.com`)\"\n    stacker:\n      rule: \"Host(`dashboard.example.com`)\"\n"
	if string(content) != want {
		t.Fatalf("config = %q", content)
	}
}

func TestUpdateDomainRejectsUnsafeValues(t *testing.T) {
	service := NewService(writeConfig(t, "stacker.example.com"), "stacker")
	for _, value := range []string{"https://example.com", "*.example.com", "localhost", "bad_domain.com", "example.com/path"} {
		if _, err := service.UpdateDomain(value); !errors.Is(err, ErrInvalidDomain) {
			t.Errorf("UpdateDomain(%q) error = %v", value, err)
		}
	}
}

func TestRestartTargetsOnlyManagedServices(t *testing.T) {
	service := NewService(writeConfig(t, "stacker.example.com"), "custom")
	var calls [][]string
	service.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}

	if err := service.Restart(context.Background(), "traefik"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"docker", "service", "update", "--force", "--detach=true", "custom_traefik"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
	if err := service.Restart(context.Background(), "database"); !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("error = %v", err)
	}
}

func TestGetMissingYAMLLeavesDomainEmpty(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "missing.yml"), "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "info" {
			return []byte(`{"ServerVersion":"27.5.1","OperatingSystem":"Ubuntu"}`), nil
		}
		return nil, errors.New("no service")
	}

	result, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Traefik.Domain != "" {
		t.Fatalf("domain = %q, want empty", result.Traefik.Domain)
	}
}

func TestGetSucceedsWhenDockerInfoFails(t *testing.T) {
	service := NewService(writeConfig(t, "stacker.example.com"), "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("Cannot connect to the Docker daemon"), errors.New("exit status 1")
	}

	result, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Traefik.Domain != "stacker.example.com" {
		t.Fatalf("domain = %q", result.Traefik.Domain)
	}
	if result.Instance.Docker != "" || result.Instance.OS != "" {
		t.Fatalf("instance = %#v", result.Instance)
	}
}

func TestGetIgnoresInvalidDockerInfoJSON(t *testing.T) {
	service := NewService(writeConfig(t, "stacker.example.com"), "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "info" {
			return []byte("not-json"), nil
		}
		return nil, errors.New("unused")
	}

	result, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Instance.Docker != "" {
		t.Fatalf("docker = %q", result.Instance.Docker)
	}
}

func TestGetRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stacker.yml")
	if err := os.WriteFile(path, []byte(":::not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(path, "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	}

	if _, err := service.Get(context.Background()); err == nil {
		t.Fatal("expected yaml error")
	}
}

func TestGetMissingStackerRouter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stacker.yml")
	if err := os.WriteFile(path, []byte("http:\n  routers:\n    other:\n      rule: \"Host(`other.example.com`)\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(path, "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("skip docker")
	}

	result, err := service.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Traefik.Domain != "" {
		t.Fatalf("domain = %q, want empty", result.Traefik.Domain)
	}
}

func TestUpdateDomainMissingFile(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "missing.yml"), "stacker")
	_, err := service.UpdateDomain("dashboard.example.com")
	if !errors.Is(err, ErrConfigMissing) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateDomainMissingHostRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stacker.yml")
	if err := os.WriteFile(path, []byte("http:\n  routers: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(path, "stacker")
	_, err := service.UpdateDomain("dashboard.example.com")
	if !errors.Is(err, ErrConfigMissing) {
		t.Fatalf("error = %v", err)
	}
}

func TestRestartWrapsDockerError(t *testing.T) {
	service := NewService("unused", "stacker")

	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("no such service\n"), errors.New("exit status 1")
	}
	err := service.Restart(context.Background(), "stacker")
	if err == nil || err.Error() != "could not restart stacker: no such service" {
		t.Fatalf("error = %v", err)
	}

	cause := errors.New("connection refused")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, cause
	}
	err = service.Restart(context.Background(), "traefik")
	if err == nil || !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "could not restart traefik") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidDomain(t *testing.T) {
	longLabel := strings.Repeat("a", 63) + ".com"
	tooLongLabel := strings.Repeat("a", 64) + ".com"
	tooLongHost := strings.Repeat("a", 254)

	tests := []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"a.b", true},
		{"my-app.example.com", true},
		{longLabel, true},
		{"", false},
		{"example.com.", false},
		{"localhost", false},
		{"-bad.example.com", false},
		{"bad-.example.com", false},
		{"BAD.example.com", false},
		{"example..com", false},
		{tooLongLabel, false},
		{tooLongHost, false},
	}
	for _, tc := range tests {
		if got := validDomain(tc.in); got != tc.want {
			t.Errorf("validDomain(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestReadServiceInspectFailure(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("not found"), errors.New("exit status 1")
	}

	info := service.readService(context.Background(), "stacker")
	if info.Status != "unavailable" || info.Name != "stacker_stacker" {
		t.Fatalf("service = %#v", info)
	}
}

func TestReadServiceIgnoresInvalidInspectJSON(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "inspect" {
			return []byte("not-json"), nil
		}
		return nil, errors.New("unused")
	}

	info := service.readService(context.Background(), "stacker")
	if info.Status != "unavailable" {
		t.Fatalf("service = %#v", info)
	}
}

func TestReadServiceParsesTraefikDigestTag(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "inspect" {
			return []byte(`{"Spec":{"Name":"stacker_traefik","TaskTemplate":{"ContainerSpec":{"Image":"traefik:v3.6@sha256:abc"}}}}`), nil
		}
		return []byte(`{"Name":"stacker_traefik","Replicas":"1/1"}`), nil
	}

	info := service.readService(context.Background(), "traefik")
	if info.Version != "v3.6" || info.Status != "healthy" {
		t.Fatalf("service = %#v", info)
	}
}

func TestReadReplicasReturnsZeroOnListFailure(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}

	running, desired := service.readReplicas(context.Background(), "stacker_stacker")
	if running != 0 || desired != 0 {
		t.Fatalf("replicas = %d/%d", running, desired)
	}
}

func TestReadReplicasSkipsUnknownNamesAndGarbage(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 1 && args[1] == "inspect" {
			return []byte(`{"Spec":{"Name":"stacker_stacker"}}`), nil
		}
		return []byte("not-json\n{\"Name\":\"other\",\"Replicas\":\"2/2\"}\n{\"Name\":\"stacker_stacker\",\"Replicas\":\"1/x\"}"), nil
	}

	info := service.readService(context.Background(), "stacker")
	if info.Running != 0 || info.Desired != 0 || info.Status != "degraded" {
		t.Fatalf("service = %#v", info)
	}
}

func TestNewWiresAdvertiseAddr(t *testing.T) {
	mod := New("/tmp/stacker.yml", "stacker", "203.0.113.10")
	if mod == nil || mod.handler == nil || mod.handler.service == nil {
		t.Fatal("nil module")
	}
	if mod.handler.service.advertiseAddr != "203.0.113.10" {
		t.Fatalf("addr = %q", mod.handler.service.advertiseAddr)
	}
	if mod.handler.service.stackName != "stacker" {
		t.Fatalf("stack = %q", mod.handler.service.stackName)
	}
}

func TestHandlerGetOK(t *testing.T) {
	service := NewService(writeConfig(t, "stacker.example.com"), "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("docker down")
	}

	rec := doJSON(t, testEngine(service), http.MethodGet, "/api/server", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	var payload struct {
		Data Settings `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Traefik.Domain != "stacker.example.com" {
		t.Fatalf("domain = %q", payload.Data.Traefik.Domain)
	}
}

func TestHandlerUpdateDomainValidation(t *testing.T) {
	service := NewService(writeConfig(t, "stacker.example.com"), "stacker")
	engine := testEngine(service)

	rec := doJSON(t, engine, http.MethodPut, "/api/server/domain", DomainRequest{Domain: "not a host"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}

	rec = doJSON(t, engine, http.MethodPut, "/api/server/domain", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bind status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
}

func TestHandlerUpdateDomainMissingConfig(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "missing.yml"), "stacker")
	rec := doJSON(t, testEngine(service), http.MethodPut, "/api/server/domain", DomainRequest{Domain: "dashboard.example.com"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
}

func TestHandlerRestartAccepted(t *testing.T) {
	service := NewService("unused", "stacker")
	service.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	}

	rec := doJSON(t, testEngine(service), http.MethodPost, "/api/server/restart", RestartRequest{Target: "traefik"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
	if !strings.Contains(rec.Body.String(), `"restarting":"traefik"`) {
		t.Fatalf("body = %s", rec.Body.Bytes())
	}
}

func TestUpdatesFiltersPrereleasesAndComparesCurrentRevision(t *testing.T) {
	previousVersion, previousRevision := Version, Revision
	Version, Revision = "v0.0.1", "abc123"
	t.Cleanup(func() { Version, Revision = previousVersion, previousRevision })

	service := NewService("unused", "stacker")
	service.client = githubTestClient(t, func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/repos/adharshmk96/stacker/releases":
			return jsonResponse(http.StatusOK, `[
				{"tag_name":"v0.0.3","draft":true},
				{"tag_name":"v0.0.2-rc.1","prerelease":true},
				{"tag_name":"v0.0.2","published_at":"2026-09-04T00:00:00Z"}
			]`)
		case "/repos/adharshmk96/stacker/commits/main":
			return jsonResponse(http.StatusOK, `{"sha":"def456","commit":{"committer":{"date":"2026-09-04T00:00:00Z"}}}`)
		default:
			t.Fatalf("unexpected GitHub path %s", req.URL.Path)
			return jsonResponse(http.StatusNotFound, `{}`)
		}
	})

	updates, err := service.Updates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updates.Stable.Version != "v0.0.2" || !updates.Stable.Available {
		t.Fatalf("stable = %#v", updates.Stable)
	}
	if updates.Edge.Revision != "def456" || !updates.Edge.Available {
		t.Fatalf("edge = %#v", updates.Edge)
	}
}

func TestUpdatesReturnsUnavailableWhenGitHubFails(t *testing.T) {
	service := NewService("unused", "stacker")
	service.client = githubTestClient(t, func(*http.Request) *http.Response {
		return jsonResponse(http.StatusBadGateway, `{}`)
	})
	if _, err := service.Updates(context.Background()); !errors.Is(err, ErrUpdatesUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdatesAllowsEdgeWhenNoStableReleaseExists(t *testing.T) {
	previousRevision := Revision
	Revision = "abc123"
	t.Cleanup(func() { Revision = previousRevision })

	service := NewService("unused", "stacker")
	service.client = githubTestClient(t, func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/repos/adharshmk96/stacker/releases":
			return jsonResponse(http.StatusOK, `[]`)
		case "/repos/adharshmk96/stacker/commits/main":
			return jsonResponse(http.StatusOK, `{"sha":"def456","commit":{"committer":{"date":"2026-09-04T00:00:00Z"}}}`)
		default:
			t.Fatalf("unexpected GitHub path %s", req.URL.Path)
			return jsonResponse(http.StatusNotFound, `{}`)
		}
	})

	updates, err := service.Updates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if updates.Stable.Channel != "stable" || updates.Stable.Available || updates.Stable.Version != "" {
		t.Fatalf("stable = %#v", updates.Stable)
	}
	if updates.Edge.Revision != "def456" || !updates.Edge.Available {
		t.Fatalf("edge = %#v", updates.Edge)
	}
}

func TestHandlerUpdatesReturnsCandidates(t *testing.T) {
	previousVersion, previousRevision := Version, Revision
	Version, Revision = "v0.0.1", "abc123"
	t.Cleanup(func() { Version, Revision = previousVersion, previousRevision })
	service := NewService("unused", "stacker")
	service.client = githubTestClient(t, func(req *http.Request) *http.Response {
		if strings.HasSuffix(req.URL.Path, "/releases") {
			return jsonResponse(http.StatusOK, `[{"tag_name":"v0.0.2","published_at":"2026-09-04T00:00:00Z"}]`)
		}
		return jsonResponse(http.StatusOK, `{"sha":"def456","commit":{"committer":{"date":"2026-09-04T00:00:00Z"}}}`)
	})
	rec := doJSON(t, testEngine(service), http.MethodGet, "/api/server/updates", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"channel":"stable"`) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.Bytes())
	}
}

func TestRunUpdateBuildsResolvedRevisionAndDeploysManagedStack(t *testing.T) {
	service := NewService("unused", "custom")
	service.advertiseAddr = "10.0.0.5"
	var calls [][]string
	service.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if name == "git" && len(args) > 0 && args[0] == "clone" {
			repoDir := args[len(args)-1]
			if err := os.MkdirAll(filepath.Join(repoDir, "deploy"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repoDir, "deploy", "stack.yml"), []byte("image: __STACKER_IMAGE__\nname: __STACKER_STACK_NAME__\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if name == "git" && len(args) > 0 && args[len(args)-1] == "HEAD" {
			return []byte("feedface\n"), nil
		}
		return nil, nil
	}

	if err := service.runUpdate(context.Background(), UpdateCandidate{Channel: "stable", Version: "v0.0.2"}); err != nil {
		t.Fatal(err)
	}
	joined := make([]string, 0, len(calls))
	for _, call := range calls {
		joined = append(joined, strings.Join(call, " "))
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{
		"docker build --pull --build-arg STACKER_VERSION=v0.0.2",
		"STACKER_REVISION=feedface",
		"docker stack deploy --detach=true --resolve-image never",
		"docker service update --force --detach=true custom_traefik",
		"docker service update --force --detach=true custom_stacker",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("commands missing %q:\n%s", want, all)
		}
	}
}

func TestRespondError(t *testing.T) {
	tests := []struct {
		err  error
		code int
		msg  string
	}{
		{ErrInvalidDomain, http.StatusBadRequest, ErrInvalidDomain.Error()},
		{ErrUnknownTarget, http.StatusBadRequest, ErrUnknownTarget.Error()},
		{ErrConfigMissing, http.StatusPreconditionFailed, ErrConfigMissing.Error()},
		{fmt.Errorf("%w: host rule is missing", ErrConfigMissing), http.StatusPreconditionFailed, "host rule is missing"},
		{errors.New("boom"), http.StatusInternalServerError, "internal server error"},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		(&Handler{}).respondError(ctx, tc.err)
		if rec.Code != tc.code {
			t.Errorf("%v status = %d, want %d", tc.err, rec.Code, tc.code)
		}
		if !strings.Contains(rec.Body.String(), tc.msg) {
			t.Errorf("%v body = %s", tc.err, rec.Body.Bytes())
		}
	}
}

func writeConfig(t *testing.T, domain string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stacker.yml")
	content := "http:\n  routers:\n    stacker:\n      rule: \"Host(`" + domain + "`)\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func testEngine(service *Service) *gin.Engine {
	engine := gin.New()
	mod := &Module{handler: &Handler{service: service}}
	mod.RegisterRoutes(engine.Group("/api"))
	return engine
}

func doJSON(t *testing.T, engine http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func githubTestClient(t *testing.T, respond func(*http.Request) *http.Response) *http.Client {
	t.Helper()
	return &http.Client{Transport: testRoundTripper(func(req *http.Request) (*http.Response, error) {
		return respond(req), nil
	})}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d test", status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
