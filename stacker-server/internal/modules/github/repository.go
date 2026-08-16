package github

import (
	"errors"

	"gorm.io/gorm"
)

type RepositoryStore struct{ db *gorm.DB }

func NewRepositoryStore(db *gorm.DB) *RepositoryStore { return &RepositoryStore{db: db} }

func (r *RepositoryStore) Current() (App, error) {
	var app App
	err := r.db.Order("created_at desc").First(&app).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return App{}, ErrNotFound
	}
	return app, err
}

func (r *RepositoryStore) Get(id string) (App, error) {
	var app App
	err := r.db.First(&app, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return App{}, ErrNotFound
	}
	return app, err
}

func (r *RepositoryStore) Replace(app *App) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&App{}).Error; err != nil {
			return err
		}
		return tx.Create(app).Error
	})
}

func (r *RepositoryStore) Save(app *App) error { return r.db.Save(app).Error }

func (r *RepositoryStore) Delete() error { return r.db.Where("1 = 1").Delete(&App{}).Error }
