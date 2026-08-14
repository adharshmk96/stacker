package database

import (
	"log/slog"

	"stacker/internal/modules/sshkey"
	"stacker/internal/modules/vps"

	"gorm.io/gorm"
)

// Migrate runs auto-migration for every module model. New modules register
// their model here — one list, so the full schema is visible in one place.
func Migrate(db *gorm.DB, log *slog.Logger) error {
	if err := db.AutoMigrate(
		&sshkey.SshKey{},
		&vps.Vps{},
	); err != nil {
		return err
	}

	log.Info("database migrated")
	return nil
}
