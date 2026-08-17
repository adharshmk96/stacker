package github

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&App{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func testFileService(t *testing.T) *Service {
	t.Helper()
	return NewService(NewRepositoryStore(testDB(t)), silentLog())
}

func testModule(t *testing.T) *Module {
	t.Helper()
	return New(testDB(t), silentLog())
}

func testRouter(mod *Module) *gin.Engine {
	engine := gin.New()
	api := engine.Group("/api")
	mod.RegisterPublicRoutes(api)
	mod.RegisterRoutes(api)
	return engine
}

func attachAPI(s *Service, srv *httptest.Server) {
	s.apiURL = srv.URL
	s.client = srv.Client()
}

func githubServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func seedApp(t *testing.T, s *Service, app App) App {
	t.Helper()
	if app.ID == "" {
		app.ID = "app-" + randomHex(4)
	}
	if app.Name == "" {
		app.Name = "Stacker"
	}
	if app.CallbackSecret == "" {
		app.CallbackSecret = "callback-secret"
	}
	if err := s.repo.Replace(&app); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return app
}

var (
	rsaOnce sync.Once
	rsaPriv *rsa.PrivateKey
)

func testRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	rsaOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		rsaPriv = key
	})
	if rsaPriv == nil {
		t.Fatal("rsa key missing")
	}
	return rsaPriv
}

func pkcs8PEM(t *testing.T) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(testRSA(t))
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func pkcs1PEM(t *testing.T) string {
	t.Helper()
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(testRSA(t)),
	}))
}

func ecPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func doJSON(t *testing.T, engine http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
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

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
}
