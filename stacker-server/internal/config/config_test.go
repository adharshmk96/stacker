package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "nested", "data")
	t.Setenv("STACKER_DATA_DIR", dataDir)
	t.Setenv("STACKER_ADDR", "")
	t.Setenv("STACKER_ENV", "")
	t.Setenv("STACKER_LOG_LEVEL", "")
	t.Setenv("STACKER_ADVERTISE_ADDR", "")
	t.Setenv("STACKER_TRAEFIK_DYNAMIC_PATH", "")
	t.Setenv("STACKER_STACK_NAME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.Env != "development" {
		t.Errorf("Env = %q, want development", cfg.Env)
	}
	if cfg.DataDir != dataDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dataDir)
	}
	if cfg.DBPath != filepath.Join(dataDir, "stacker.db") {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if cfg.KeyDir != filepath.Join(dataDir, "keys") {
		t.Errorf("KeyDir = %q", cfg.KeyDir)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.AdvertiseAddr != "" {
		t.Errorf("AdvertiseAddr = %q, want empty", cfg.AdvertiseAddr)
	}
	if cfg.TraefikDynamicPath != "/etc/stacker/traefik/dynamic/stacker.yml" {
		t.Errorf("TraefikDynamicPath = %q", cfg.TraefikDynamicPath)
	}
	if cfg.StackName != "stacker" {
		t.Errorf("StackName = %q, want stacker", cfg.StackName)
	}

	info, err := os.Stat(cfg.KeyDir)
	if err != nil {
		t.Fatalf("KeyDir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("KeyDir is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("KeyDir perm = %04o, want 0700", perm)
	}
}

func TestLoadOverrides(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("STACKER_DATA_DIR", dataDir)
	t.Setenv("STACKER_ADDR", ":9999")
	t.Setenv("STACKER_ENV", "production")
	t.Setenv("STACKER_LOG_LEVEL", "debug")
	t.Setenv("STACKER_ADVERTISE_ADDR", "10.0.0.1")
	t.Setenv("STACKER_TRAEFIK_DYNAMIC_PATH", "/tmp/dynamic.yml")
	t.Setenv("STACKER_STACK_NAME", "custom")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Addr != ":9999" {
		t.Errorf("Addr = %q, want :9999", cfg.Addr)
	}
	if cfg.Env != "production" {
		t.Errorf("Env = %q, want production", cfg.Env)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.AdvertiseAddr != "10.0.0.1" {
		t.Errorf("AdvertiseAddr = %q", cfg.AdvertiseAddr)
	}
	if cfg.TraefikDynamicPath != "/tmp/dynamic.yml" {
		t.Errorf("TraefikDynamicPath = %q", cfg.TraefikDynamicPath)
	}
	if cfg.StackName != "custom" {
		t.Errorf("StackName = %q, want custom", cfg.StackName)
	}
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
}

func TestLoadMkdirFail(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("STACKER_DATA_DIR", blocked)

	_, err := Load()
	if err == nil {
		t.Fatal("Load succeeded, want mkdir error")
	}
}

func TestIsProduction(t *testing.T) {
	if !(Config{Env: "production"}).IsProduction() {
		t.Error("production should report true")
	}
	if (Config{Env: "development"}).IsProduction() {
		t.Error("development should report false")
	}
	if (Config{Env: "Production"}).IsProduction() {
		t.Error("case-sensitive: Production should report false")
	}
}

func TestEnvBool(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		fallback bool
		want     bool
	}{
		{"empty uses fallback true", "", true, true},
		{"empty uses fallback false", "", false, false},
		{"true", "true", false, true},
		{"TRUE", "TRUE", false, true},
		{"1", "1", false, true},
		{"false", "false", true, false},
		{"0", "0", true, false},
		{"garbage uses fallback", "yes", true, true},
		{"garbage uses fallback false", "nope", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STACKER_TEST_BOOL", tc.value)
			if got := EnvBool("STACKER_TEST_BOOL", tc.fallback); got != tc.want {
				t.Errorf("EnvBool = %v, want %v", got, tc.want)
			}
		})
	}
}
