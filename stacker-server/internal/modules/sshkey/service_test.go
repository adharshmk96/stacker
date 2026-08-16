package sshkey

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&SshKey{}); err != nil {
		t.Fatal(err)
	}
	return NewService(NewRepository(db), t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestDefaultKeyLifecycle(t *testing.T) {
	service := testService(t)

	key, err := service.EnsureDefault()
	if err != nil {
		t.Fatal(err)
	}
	if !key.IsDefault || key.Type != KeyTypeEd25519 {
		t.Fatalf("unexpected default key: %+v", key)
	}

	again, err := service.EnsureDefault()
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != key.ID {
		t.Fatal("EnsureDefault created a second key")
	}
	if err := service.Delete(key.ID); err != ErrDefaultKey {
		t.Fatalf("Delete error = %v, want %v", err, ErrDefaultKey)
	}

	rotated, err := service.Rotate(key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != key.ID || rotated.Fingerprint == key.Fingerprint {
		t.Fatal("Rotate did not retain the id and replace the keypair")
	}
}
