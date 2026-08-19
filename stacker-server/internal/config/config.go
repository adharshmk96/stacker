package config

import (
	"os"
	"path/filepath"
	"strconv"
)

// Config holds every runtime knob. Everything is resolved once at startup and
// passed down explicitly — no globals, no re-reading the environment later.
type Config struct {
	Addr string
	Env  string

	// DataDir is the per-user application directory (see spec.md).
	DataDir string
	// DBPath is the sqlite file inside DataDir.
	DBPath string
	// KeyDir is the folder holding generated SSH keypairs.
	KeyDir string
	// WorkRoot is where a deploy clones and builds.
	//
	// It is configured separately from DataDir because it is the one path that
	// has to mean the same thing inside this container and on the host. A
	// compose file's relative bind mount is resolved by swarm on the node that
	// runs the task, not by stacker, so a checkout living only on stacker's
	// side of a named volume gives every such service "bind source path does
	// not exist". Deployed installs bind-mount a host directory here at the
	// identical path; the default keeps a local `go run` self-contained.
	WorkRoot string

	LogLevel string
	// AdvertiseAddr is the host address selected by install.sh for swarm peers.
	AdvertiseAddr string
	// TraefikDynamicPath is the installed file-provider config shared with Traefik.
	TraefikDynamicPath string
	// StackName prefixes the two Swarm services managed by the Server settings page.
	StackName string
}

// Load builds the config from the environment, falling back to the
// platform-specific application data directory.
func Load() (Config, error) {
	dataDir := env("STACKER_DATA_DIR", "/data")

	cfg := Config{
		Addr:               env("STACKER_ADDR", ":8080"),
		Env:                env("STACKER_ENV", "development"),
		DataDir:            dataDir,
		DBPath:             filepath.Join(dataDir, "stacker.db"),
		KeyDir:             filepath.Join(dataDir, "keys"),
		WorkRoot:           env("STACKER_WORK_ROOT", filepath.Join(dataDir, "workspaces")),
		LogLevel:           env("STACKER_LOG_LEVEL", "info"),
		AdvertiseAddr:      env("STACKER_ADVERTISE_ADDR", ""),
		TraefikDynamicPath: env("STACKER_TRAEFIK_DYNAMIC_PATH", "/etc/stacker/traefik/dynamic/stacker.yml"),
		StackName:          env("STACKER_STACK_NAME", "stacker"),
	}

	// The key directory holds private keys — 0700 for both it and its parent.
	if err := os.MkdirAll(cfg.KeyDir, 0o700); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// IsProduction reports whether the server should hide internal error details.
func (c Config) IsProduction() bool {
	return c.Env == "production"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvBool reads a boolean flag from the environment.
func EnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
