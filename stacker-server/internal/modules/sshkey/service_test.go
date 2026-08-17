package sshkey

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// nodeRef is enough of the nodes table for UsedByNodeCount. sshkey must not
// import the node package — that import would cycle.
type nodeRef struct {
	ID       string `gorm:"primaryKey;size:32"`
	SshKeyID string
}

func (nodeRef) TableName() string { return "nodes" }

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&SshKey{}, &nodeRef{}); err != nil {
		t.Fatal(err)
	}
	return db, t.TempDir()
}

func testService(t *testing.T) *Service {
	t.Helper()
	db, keyDir := testDB(t)
	return NewService(NewRepository(db), keyDir, silentLog())
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

func TestEnsureDefaultNameClashFallsBack(t *testing.T) {
	service := testService(t)

	taken, err := service.Create(CreateRequest{Name: defaultKeyName, Type: KeyTypeEd25519})
	if err != nil {
		t.Fatal(err)
	}
	if taken.IsDefault {
		t.Fatal("pre-existing name should not be the default")
	}

	key, err := service.EnsureDefault()
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != defaultKeyName+"-key" || !key.IsDefault {
		t.Fatalf("fallback key = %+v", key)
	}

	stored, err := service.Get(key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.IsDefault {
		t.Fatal("fallback key was not persisted as default")
	}
}

func TestEnsureDefaultBothNamesTaken(t *testing.T) {
	service := testService(t)
	for _, name := range []string{defaultKeyName, defaultKeyName + "-key"} {
		if _, err := service.Create(CreateRequest{Name: name, Type: KeyTypeEd25519}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.EnsureDefault(); err != ErrNameTaken {
		t.Fatalf("EnsureDefault error = %v, want %v", err, ErrNameTaken)
	}
}

func TestCreateEd25519AndRSA(t *testing.T) {
	service := testService(t)

	ed, err := service.Create(CreateRequest{Name: "  deploy  ", Type: KeyTypeEd25519})
	if err != nil {
		t.Fatal(err)
	}
	if ed.Name != "deploy" || ed.Type != KeyTypeEd25519 || ed.IsDefault {
		t.Fatalf("ed25519 key = %+v", ed)
	}
	if !strings.HasPrefix(ed.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("ed25519 public key = %q", ed.PublicKey)
	}
	if !strings.HasPrefix(ed.Fingerprint, "SHA256:") {
		t.Fatalf("fingerprint = %q", ed.Fingerprint)
	}
	if _, err := os.Stat(ed.PrivateKeyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ed.PrivateKeyPath + ".pub"); err != nil {
		t.Fatal(err)
	}

	rsaKey, err := service.Create(CreateRequest{Name: "legacy", Type: KeyTypeRSA})
	if err != nil {
		t.Fatal(err)
	}
	if rsaKey.Type != KeyTypeRSA || !strings.HasPrefix(rsaKey.PublicKey, "ssh-rsa ") {
		t.Fatalf("rsa key = %+v", rsaKey)
	}

	keys, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("List len = %d, want 2", len(keys))
	}
}

func TestCreateRejectsInvalidNameTakenAndUnknownType(t *testing.T) {
	service := testService(t)

	for _, name := range []string{"", "  ", "has space", "bad/name", "foo@bar"} {
		if _, err := service.Create(CreateRequest{Name: name, Type: KeyTypeEd25519}); err != ErrInvalidName {
			t.Fatalf("name %q: error = %v, want %v", name, err, ErrInvalidName)
		}
	}

	if _, err := service.Create(CreateRequest{Name: "ok", Type: "dsa"}); err != ErrUnknownType {
		t.Fatalf("unknown type error = %v, want %v", err, ErrUnknownType)
	}

	if _, err := service.Create(CreateRequest{Name: "ok", Type: KeyTypeEd25519}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(CreateRequest{Name: "ok", Type: KeyTypeEd25519}); err != ErrNameTaken {
		t.Fatalf("duplicate error = %v, want %v", err, ErrNameTaken)
	}
}

func TestPrivateKeyPath(t *testing.T) {
	service := testService(t)

	if _, err := service.PrivateKeyPath("missing"); err != ErrNotFound {
		t.Fatalf("missing id error = %v, want %v", err, ErrNotFound)
	}

	key, err := service.Create(CreateRequest{Name: "path-check", Type: KeyTypeEd25519})
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.PrivateKeyPath(key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != key.PrivateKeyPath {
		t.Fatalf("path = %q, want %q", got, key.PrivateKeyPath)
	}

	if err := os.Remove(key.PrivateKeyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PrivateKeyPath(key.ID); err == nil {
		t.Fatal("expected missing-file error")
	}
}

func TestDeleteInUse(t *testing.T) {
	db, keyDir := testDB(t)
	service := NewService(NewRepository(db), keyDir, silentLog())

	key, err := service.Create(CreateRequest{Name: "in-use", Type: KeyTypeEd25519})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&nodeRef{ID: "n1", SshKeyID: key.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(key.ID); err != ErrKeyInUse {
		t.Fatalf("Delete error = %v, want %v", err, ErrKeyInUse)
	}
}

func TestDeleteRemovesUnusedKey(t *testing.T) {
	service := testService(t)

	if err := service.Delete("missing"); err != ErrNotFound {
		t.Fatalf("Delete missing = %v, want %v", err, ErrNotFound)
	}

	key, err := service.Create(CreateRequest{Name: "temp", Type: KeyTypeEd25519})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(key.ID); err != ErrNotFound {
		t.Fatalf("Get after delete = %v, want %v", err, ErrNotFound)
	}
	if _, err := os.Stat(key.PrivateKeyPath); !os.IsNotExist(err) {
		t.Fatalf("private key still on disk: %v", err)
	}
	if _, err := os.Stat(key.PrivateKeyPath + ".pub"); !os.IsNotExist(err) {
		t.Fatalf("public key still on disk: %v", err)
	}
}

func TestRotateNonDefault(t *testing.T) {
	service := testService(t)

	if _, err := service.Rotate("missing"); err != ErrNotFound {
		t.Fatalf("Rotate missing = %v, want %v", err, ErrNotFound)
	}

	key, err := service.Create(CreateRequest{Name: "other", Type: KeyTypeEd25519})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Rotate(key.ID); err != ErrDefaultKey {
		t.Fatalf("Rotate non-default = %v, want %v", err, ErrDefaultKey)
	}
}

func TestCreateFailsWhenKeyDirIsAFile(t *testing.T) {
	db, _ := testDB(t)
	badDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(badDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(NewRepository(db), badDir, silentLog())
	if _, err := service.Create(CreateRequest{Name: "fail", Type: KeyTypeEd25519}); err == nil {
		t.Fatal("expected write error")
	}
}

func TestValidateNameAcceptsSafeFilenames(t *testing.T) {
	for _, name := range []string{"a", "A.B-c_1", "stacker-default"} {
		if err := validateName(name); err != nil {
			t.Fatalf("validateName(%q) = %v", name, err)
		}
	}
}
