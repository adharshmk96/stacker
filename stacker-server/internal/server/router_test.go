package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stacker/internal/config"
	"stacker/internal/database"
	"stacker/internal/modules/auth"
	"stacker/internal/modules/node"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testEngine(t *testing.T, cfg config.Config) *gin.Engine {
	t.Helper()

	log := silentLog()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := os.MkdirAll(cfg.KeyDir, 0o700); err != nil {
		t.Fatalf("keydir: %v", err)
	}

	if cfg.IsProduction() {
		t.Cleanup(func() { gin.SetMode(gin.TestMode) })
	}

	engine, err := newRouter(cfg, db, log)
	if err != nil {
		t.Fatalf("newRouter: %v", err)
	}
	return engine
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	data := t.TempDir()
	return config.Config{
		Addr:               ":0",
		Env:                "development",
		DataDir:            data,
		DBPath:             filepath.Join(data, "stacker.db"),
		KeyDir:             filepath.Join(data, "keys"),
		LogLevel:           "info",
		TraefikDynamicPath: filepath.Join(data, "stacker.yml"),
		StackName:          "stacker",
	}
}

func TestHealth(t *testing.T) {
	engine := testEngine(t, testConfig(t))

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestCORSAllowsOrigin(t *testing.T) {
	engine := testEngine(t, testConfig(t))
	origin := "http://localhost:3000"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", origin)
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Allow-Origin = %q, want %q", got, origin)
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Errorf("Vary = %q, want Origin", rec.Header().Get("Vary"))
	}
}

func TestCORSPreflight(t *testing.T) {
	engine := testEngine(t, testConfig(t))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Allow-Methods is empty")
	}
}

func TestCORSWithoutOrigin(t *testing.T) {
	engine := testEngine(t, testConfig(t))

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("Allow-Origin = %q, want empty", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestRequestLoggerRecordsErrors(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))

	r := gin.New()
	r.Use(requestLogger(log))
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	r.GET("/boom", func(c *gin.Context) {
		_ = c.Error(errors.New("explode"))
		c.Status(http.StatusInternalServerError)
	})

	okRec := httptest.NewRecorder()
	r.ServeHTTP(okRec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if !strings.Contains(buf.String(), "request") {
		t.Errorf("success log = %q, want a request line", buf.String())
	}

	buf.Reset()
	boomRec := httptest.NewRecorder()
	r.ServeHTTP(boomRec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if boomRec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", boomRec.Code)
	}
	if !strings.Contains(buf.String(), "request failed") || !strings.Contains(buf.String(), "explode") {
		t.Errorf("error log = %q, want request failed / explode", buf.String())
	}
}

func TestNewRouterProductionSetsReleaseMode(t *testing.T) {
	cfg := testConfig(t)
	cfg.Env = "production"
	_ = testEngine(t, cfg)

	if gin.Mode() != gin.ReleaseMode {
		t.Errorf("mode = %q, want %q", gin.Mode(), gin.ReleaseMode)
	}
}

func TestNewRouterFailsWhenKeyDirIsAFile(t *testing.T) {
	log := silentLog()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := testConfig(t)
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg.KeyDir = blocked

	_, err = newRouter(cfg, db, log)
	if err == nil {
		t.Fatal("newRouter succeeded, want a key-dir error")
	}
}

func TestNewRouterFailsWhenAuthCannotStart(t *testing.T) {
	log := silentLog()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Migrator().DropTable(&auth.Secret{}); err != nil {
		t.Fatalf("drop secrets: %v", err)
	}

	cfg := testConfig(t)
	if err := os.MkdirAll(cfg.KeyDir, 0o700); err != nil {
		t.Fatalf("keydir: %v", err)
	}

	_, err = newRouter(cfg, db, log)
	if err == nil {
		t.Fatal("newRouter succeeded, want an auth error")
	}
}

func TestNewRouterLogsRecoverFailure(t *testing.T) {
	log := silentLog()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := testConfig(t)
	if err := os.MkdirAll(cfg.KeyDir, 0o700); err != nil {
		t.Fatalf("keydir: %v", err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg.DataDir = blocked

	engine, err := newRouter(cfg, db, log)
	if err != nil {
		t.Fatalf("newRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — recover failure is not fatal", rec.Code)
	}
}

func TestNewRouterLogsLocalNodeFailure(t *testing.T) {
	log := silentLog()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"), log)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Migrator().DropTable(&node.Node{}); err != nil {
		t.Fatalf("drop nodes: %v", err)
	}

	cfg := testConfig(t)
	if err := os.MkdirAll(cfg.KeyDir, 0o700); err != nil {
		t.Fatalf("keydir: %v", err)
	}

	engine, err := newRouter(cfg, db, log)
	if err != nil {
		t.Fatalf("newRouter: %v", err)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — local-node failure is not fatal", rec.Code)
	}
}
