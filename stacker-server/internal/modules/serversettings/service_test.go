package serversettings

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetReadsInstalledDomainAndDockerInfo(t *testing.T) {
	path := writeConfig(t, "stacker.203.0.113.10.sslip.io")
	service := NewService(path, "stacker")
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

func writeConfig(t *testing.T, domain string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stacker.yml")
	content := "http:\n  routers:\n    stacker:\n      rule: \"Host(`" + domain + "`)\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
