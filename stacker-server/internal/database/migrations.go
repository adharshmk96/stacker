package database

import (
	"log/slog"

	"stacker/internal/modules/auth"
	githubprovider "stacker/internal/modules/github"
	"stacker/internal/modules/node"
	"stacker/internal/modules/project"
	"stacker/internal/modules/smtp"
	"stacker/internal/modules/sshkey"

	"gorm.io/gorm"
)

// models is every module model, in dependency order. New modules add theirs
// here — one list, so the full schema is visible in one place, and so Reset
// knows what there is to erase.
func models() []any {
	return []any{
		&auth.User{},
		&auth.Session{},
		&auth.PasswordReset{},
		&auth.Secret{},
		&githubprovider.App{},
		&sshkey.SshKey{},
		&node.Node{},
		&project.Project{},
		&project.Environment{},
		&project.Deployment{},
		&smtp.Settings{},
	}
}

// Migrate runs auto-migration for every module model.
func Migrate(db *gorm.DB, log *slog.Logger) error {
	if err := renameVpsTable(db, log); err != nil {
		return err
	}

	if err := db.AutoMigrate(models()...); err != nil {
		return err
	}

	log.Info("database migrated")
	return nil
}

// Reset erases every table and rebuilds the empty schema, leaving the install
// exactly as it was before first run — no account, no nodes, no keys. It backs
// the "reset all data" button in Settings.
//
// Tables are dropped in reverse order so a child never outlives its parent, and
// foreign keys are switched off for the duration because sqlite checks them per
// statement rather than at the end of the transaction.
func Reset(db *gorm.DB, log *slog.Logger) error {
	log.Warn("erasing all data")

	if err := db.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
		return err
	}
	defer db.Exec("PRAGMA foreign_keys = ON") //nolint:errcheck // best effort

	list := models()
	migrator := db.Migrator()
	for i := len(list) - 1; i >= 0; i-- {
		if err := migrator.DropTable(list[i]); err != nil {
			return err
		}
	}

	return Migrate(db, log)
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
