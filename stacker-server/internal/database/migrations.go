package database

import (
	"log/slog"

	"stacker/internal/modules/node"
	"stacker/internal/modules/sshkey"

	"gorm.io/gorm"
)

// Migrate runs auto-migration for every module model. New modules register
// their model here — one list, so the full schema is visible in one place.
func Migrate(db *gorm.DB, log *slog.Logger) error {
	if err := renameVpsTable(db, log); err != nil {
		return err
	}

	if err := db.AutoMigrate(
		&sshkey.SshKey{},
		&node.Node{},
	); err != nil {
		return err
	}

	log.Info("database migrated")
	return nil
}

// renameVpsTable carries databases written before VPS was renamed to Node.
// AutoMigrate would otherwise create an empty `nodes` table and strand the old
// rows in `vps`.
func renameVpsTable(db *gorm.DB, log *slog.Logger) error {
	m := db.Migrator()
	if !m.HasTable("vps") || m.HasTable("nodes") {
		return nil
	}
	if err := m.RenameTable("vps", "nodes"); err != nil {
		return err
	}

	log.Info("renamed the vps table to nodes")
	return nil
}
