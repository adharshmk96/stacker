package sshkey

import (
	"errors"

	"gorm.io/gorm"
)

// Repository is the only place this module touches the database.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List() ([]SshKey, error) {
	var keys []SshKey
	err := r.db.Order("created_at desc").Find(&keys).Error
	return keys, err
}

func (r *Repository) Get(id string) (SshKey, error) {
	var key SshKey
	err := r.db.First(&key, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return SshKey{}, ErrNotFound
	}
	return key, err
}

func (r *Repository) ExistsByName(name string) (bool, error) {
	var count int64
	err := r.db.Model(&SshKey{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (r *Repository) Create(key *SshKey) error {
	return r.db.Create(key).Error
}

func (r *Repository) Delete(id string) error {
	res := r.db.Delete(&SshKey{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UsedByVpsCount reports how many VPS entries reference this key. Kept as a raw
// count so the module doesn't have to import the vps package.
func (r *Repository) UsedByVpsCount(id string) (int64, error) {
	var count int64
	err := r.db.Table("vps").Where("ssh_key_id = ?", id).Count(&count).Error
	return count, err
}
