package node

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

func (r *Repository) List() ([]Node, error) {
	var items []Node
	err := r.db.Order("created_at desc").Find(&items).Error
	return items, err
}

func (r *Repository) Get(id string) (Node, error) {
	var item Node
	err := r.db.First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Node{}, ErrNotFound
	}
	return item, err
}

// ExistsByName checks for a name clash, ignoring the entry being updated.
func (r *Repository) ExistsByName(name, excludeID string) (bool, error) {
	q := r.db.Model(&Node{}).Where("name = ?", name)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}

	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

func (r *Repository) Create(item *Node) error {
	return r.db.Create(item).Error
}

// Save writes every column of an existing row.
func (r *Repository) Save(item *Node) error {
	return r.db.Save(item).Error
}

func (r *Repository) Delete(id string) error {
	res := r.db.Delete(&Node{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
