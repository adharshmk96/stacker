package smtp

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Settings{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func testService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(NewRepository(testDB(t)), t.TempDir(), silentLog())
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := loadOrCreateKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := encrypt(key, "s3cret-app-password")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "s3cret-app-password" {
		t.Fatal("password was not encrypted")
	}

	plaintext, err := decrypt(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "s3cret-app-password" {
		t.Fatalf("plaintext = %q, want %q", plaintext, "s3cret-app-password")
	}
}

func TestLoadOrCreateKeyPersists(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("key was regenerated instead of reused")
	}
}

func TestUpdateStoresPasswordEncrypted(t *testing.T) {
	service := testService(t)

	updated, err := service.Update(UpdateRequest{
		Enabled:   true,
		Host:      "smtp.example.com",
		Port:      587,
		Username:  "user@example.com",
		Password:  "s3cret-app-password",
		FromEmail: "noreply@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasPassword {
		t.Fatal("expected HasPassword to be true")
	}

	stored, err := service.repo.Get()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Password == "s3cret-app-password" {
		t.Fatal("password was stored in plaintext")
	}
	if stored.Password == "" {
		t.Fatal("password was not stored")
	}

	decrypted, err := decrypt(service.key, stored.Password)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "s3cret-app-password" {
		t.Fatalf("decrypted password = %q, want %q", decrypted, "s3cret-app-password")
	}
}

func TestUpdateKeepsExistingPasswordWhenBlank(t *testing.T) {
	service := testService(t)

	if _, err := service.Update(UpdateRequest{
		Host:      "smtp.example.com",
		Port:      587,
		Password:  "s3cret-app-password",
		FromEmail: "noreply@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	before, err := service.repo.Get()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Update(UpdateRequest{
		Host:      "smtp.example.com",
		Port:      587,
		FromEmail: "noreply@example.com",
		FromName:  "Stacker",
	}); err != nil {
		t.Fatal(err)
	}

	after, err := service.repo.Get()
	if err != nil {
		t.Fatal(err)
	}
	if after.Password != before.Password {
		t.Fatal("password ciphertext changed even though no new password was submitted")
	}
}
