package database

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"stacker/internal/modules/auth"
	"stacker/internal/modules/node"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestOpenClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "test.db")
	log := silentLog()

	db, err := Open(path, log)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var on int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&on).Error; err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if on != 1 {
		t.Fatal("foreign keys are not on")
	}

	if err := Close(db); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOpenMkdirFail(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := Open(filepath.Join(blocked, "stacker.db"), silentLog())
	if err == nil {
		t.Fatal("Open succeeded, want mkdir error")
	}
}

func TestOpenRejectsADirectory(t *testing.T) {
	_, err := Open(t.TempDir(), silentLog())
	if err == nil {
		t.Fatal("Open succeeded against a directory")
	}
}

func TestMigrateCreatesEveryTable(t *testing.T) {
	db := testDB(t)
	log := silentLog()

	if err := Migrate(db, log); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	m := db.Migrator()
	for _, model := range models() {
		if !m.HasTable(model) {
			t.Errorf("missing table for %T", model)
		}
	}
}

func TestResetWipesRowsAndRebuilds(t *testing.T) {
	db := testDB(t)
	log := silentLog()
	if err := Migrate(db, log); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	user := auth.User{
		ID: "u1", Email: "a@b.c", Name: "Ada", Username: "ada", PasswordHash: "x",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := Reset(db, log); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	var n int64
	if err := db.Model(&auth.User{}).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("users = %d, want 0 after reset", n)
	}
	if !db.Migrator().HasTable(&auth.User{}) {
		t.Fatal("users table was not rebuilt")
	}
}

func TestRenameVpsTableMovesRows(t *testing.T) {
	db := testDB(t)

	if err := db.Exec("CREATE TABLE vps (id TEXT PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatalf("create vps: %v", err)
	}
	if err := db.Exec("INSERT INTO vps (id, name) VALUES ('old', 'box')").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := renameVpsTable(db, silentLog()); err != nil {
		t.Fatalf("renameVpsTable: %v", err)
	}

	m := db.Migrator()
	if m.HasTable("vps") {
		t.Fatal("vps table still exists")
	}
	if !m.HasTable("nodes") {
		t.Fatal("nodes table was not created")
	}

	var name string
	if err := db.Raw("SELECT name FROM nodes WHERE id = ?", "old").Scan(&name).Error; err != nil {
		t.Fatalf("read renamed row: %v", err)
	}
	if name != "box" {
		t.Errorf("name = %q, want box", name)
	}
}

func TestRenameVpsTableNoopsWhenNodesExists(t *testing.T) {
	db := testDB(t)
	if err := db.Exec("CREATE TABLE vps (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create vps: %v", err)
	}
	if err := db.Migrator().CreateTable(&node.Node{}); err != nil {
		t.Fatalf("create nodes: %v", err)
	}

	if err := renameVpsTable(db, silentLog()); err != nil {
		t.Fatalf("renameVpsTable: %v", err)
	}
	if !db.Migrator().HasTable("vps") {
		t.Fatal("vps was renamed even though nodes already existed")
	}
}

func TestRenameVpsTableNoopsWithoutVps(t *testing.T) {
	db := testDB(t)
	if err := renameVpsTable(db, silentLog()); err != nil {
		t.Fatalf("renameVpsTable: %v", err)
	}
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}
