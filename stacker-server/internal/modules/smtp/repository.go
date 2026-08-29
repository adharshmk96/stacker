package smtp

import (
	"errors"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Get() (Settings, error) {
	var item Settings
	err := r.db.First(&item, "id = ?", settingsID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Settings{ID: settingsID, Port: 587, Encryption: "starttls"}, nil
	}
	return item, err
}

func (r *Repository) Save(item *Settings) error {
	item.ID = settingsID
	return r.db.Save(item).Error
}
