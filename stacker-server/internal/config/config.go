package config

import (
	"os"
	"path/filepath"
	"runtime"
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

	LogLevel string
}

// Load builds the config from the environment, falling back to the
// platform-specific application data directory.
func Load() (Config, error) {
	dataDir, err := DataDir()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:     env("STACKER_ADDR", ":8080"),
		Env:      env("STACKER_ENV", "development"),
		DataDir:  dataDir,
		DBPath:   filepath.Join(dataDir, "stacker.db"),
		KeyDir:   filepath.Join(dataDir, "keys"),
		LogLevel: env("STACKER_LOG_LEVEL", "info"),
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

// DataDir resolves the application data directory without creating anything,
// so callers that only need the path (like `stacker uninstall`) don't recreate
// a directory they are about to remove.
func DataDir() (string, error) {
	if dir := os.Getenv("STACKER_DATA_DIR"); dir != "" {
		return dir, nil
	}
	return defaultDataDir()
}

func defaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "stacker"), nil
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "stacker"), nil
		}
		return filepath.Join(home, "AppData", "Local", "stacker"), nil
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "stacker"), nil
		}
		return filepath.Join(home, ".local", "share", "stacker"), nil
	}
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
