package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Session{}, &PasswordReset{}, &Secret{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testModule(t *testing.T, reset ResetDataFunc) (*Module, *gorm.DB) {
	t.Helper()

	db := testDB(t)
	mod, err := New(db, nil, reset, silentLog())
	if err != nil {
		t.Fatalf("new auth module: %v", err)
	}
	return mod, db
}

func testRouter(mod *Module) *gin.Engine {
	engine := gin.New()
	mod.RegisterRoutes(engine.Group("/api"))
	return engine
}

func sampleRegister() RegisterRequest {
	return RegisterRequest{
		Name:     "Ada Lovelace",
		Username: "ada",
		Email:    "Ada@Example.com",
		Password: "password1",
	}
}

func mustRegister(t *testing.T, svc *Service) LoginResult {
	t.Helper()
	result, err := svc.Register(sampleRegister(), "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return result
}

func wipeAuth(db *gorm.DB) error {
	models := []any{&Session{}, &PasswordReset{}, &User{}, &Secret{}}
	for _, model := range models {
		if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(model).Error; err != nil {
			return err
		}
	}
	return nil
}

func doJSON(t *testing.T, engine http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func doRaw(t *testing.T, engine http.Handler, method, path, token, raw string, header http.Header) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(raw)))
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func decodeData[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	return envelope.Data
}

func insertUser(t *testing.T, db *gorm.DB, user User) {
	t.Helper()
	if user.ID == "" {
		user.ID = newID()
	}
	if user.PasswordHash == "" {
		user.PasswordHash = "not-a-real-hash"
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
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
