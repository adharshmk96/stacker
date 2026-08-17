package project

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

/* ---- projects ---- */

// List returns every project with its environments. Environments are always
// preloaded: there is no screen that shows a project without them.
func (r *Repository) List() ([]Project, error) {
	var items []Project
	err := r.db.
		Preload("Environments", orderEnvironments).
		Order("created_at desc").
		Find(&items).Error
	return items, err
}

func (r *Repository) Get(id string) (Project, error) {
	var item Project
	err := r.db.
		Preload("Environments", orderEnvironments).
		First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Project{}, ErrNotFound
	}
	return item, err
}

// ExistsByName checks for a name clash, ignoring the project being updated.
func (r *Repository) ExistsByName(name, excludeID string) (bool, error) {
	q := r.db.Model(&Project{}).Where("name = ?", name)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}

	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// HostOwner returns the id of the environment already routing a hostname, or an
// empty string when the host is free. Routes live in one Traefik directory, so
// two environments claiming the same host would silently fight over it.
func (r *Repository) HostOwner(host string) (string, error) {
	var items []Environment
	// Domains are a JSON column, so the match cannot be pushed into SQL. The
	// list is small — one row per environment across the whole installation.
	if err := r.db.Find(&items).Error; err != nil {
		return "", err
	}
	for _, env := range items {
		for _, domain := range env.Domains {
			if domain.Host == host {
				return env.ID, nil
			}
		}
	}
	return "", nil
}

// Create writes a project and its environments in one transaction, so a failure
// halfway cannot leave a project with no way to deploy.
func (r *Repository) Create(item *Project) error {
	return r.db.Create(item).Error
}

// Save replaces a project and its environment set. Environments absent from the
// new list are deleted, which is what makes the detail page's "remove
// environment" stick.
func (r *Repository) Save(item *Project, removedEnvIDs []string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if len(removedEnvIDs) > 0 {
			if err := tx.Delete(&Environment{}, "id IN ?", removedEnvIDs).Error; err != nil {
				return err
			}
		}
		// Omit the association and write the children by hand: gorm's full
		// save would re-insert rows it has already seen and lose Position.
		if err := tx.Omit("Environments").Save(item).Error; err != nil {
			return err
		}
		for i := range item.Environments {
			if err := tx.Save(&item.Environments[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete removes a project, its environments and its deployment history.
//
// Children go first: AutoMigrate gives project_environments a foreign key onto
// projects, and sqlite checks constraints per statement rather than at the end of
// the transaction, so deleting the parent first fails outright.
func (r *Repository) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&Environment{}, "project_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&Deployment{}, "project_id = ?", id).Error; err != nil {
			return err
		}

		res := tx.Delete(&Project{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

/* ---- deployments ---- */

// ListDeployments returns runs newest first, optionally for one project.
func (r *Repository) ListDeployments(projectID string, limit int) ([]Deployment, error) {
	q := r.db.Order("started_at desc").Limit(limit)
	if projectID != "" {
		q = q.Where("project_id = ?", projectID)
	}

	var items []Deployment
	err := q.Find(&items).Error
	return items, err
}

func (r *Repository) GetDeployment(id string) (Deployment, error) {
	var item Deployment
	err := r.db.First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Deployment{}, ErrDeployNotFound
	}
	return item, err
}

// LatestByEnvironment returns the newest run of each environment of a project,
// keyed by environment id — one query for the whole status view rather than one
// per environment.
func (r *Repository) LatestByEnvironment(projectID string) (map[string]Deployment, error) {
	var items []Deployment
	err := r.db.
		Where("project_id = ?", projectID).
		Order("started_at desc").
		Find(&items).Error
	if err != nil {
		return nil, err
	}

	latest := make(map[string]Deployment, len(items))
	for _, item := range items {
		if _, seen := latest[item.EnvironmentID]; !seen {
			latest[item.EnvironmentID] = item
		}
	}
	return latest, nil
}

// ActiveForEnvironment returns the run currently queued or working on an
// environment, if there is one. It is the check behind ErrAlreadyDeploying.
func (r *Repository) ActiveForEnvironment(envID string) (Deployment, bool, error) {
	var item Deployment
	err := r.db.
		Where("environment_id = ? AND status IN ?", envID, []DeploymentStatus{StatusQueued, StatusRunning}).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Deployment{}, false, nil
	}
	if err != nil {
		return Deployment{}, false, err
	}
	return item, true, nil
}

// NextNumber is the run number shown as `#42`. It counts per project, so each
// project's history reads from one, and it is taken inside the same transaction
// as the insert to keep two concurrent deploys from sharing a number.
func (r *Repository) CreateDeployment(item *Deployment) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var max struct{ Number int }
		err := tx.Model(&Deployment{}).
			Select("COALESCE(MAX(number), 0) AS number").
			Where("project_id = ?", item.ProjectID).
			Scan(&max).Error
		if err != nil {
			return err
		}
		item.Number = max.Number + 1
		return tx.Create(item).Error
	})
}

func (r *Repository) SaveDeployment(item *Deployment) error {
	return r.db.Save(item).Error
}

// ResetRunning marks every run that was in flight as failed. It runs at startup:
// a run only lives inside the process that started it, so a row still marked
// running after a restart is a run nobody is watching any more.
func (r *Repository) ResetRunning(reason string) error {
	now := timeNow()
	return r.db.Model(&Deployment{}).
		Where("status IN ?", []DeploymentStatus{StatusQueued, StatusRunning}).
		Updates(map[string]any{
			"status":      StatusFailed,
			"error":       reason,
			"finished_at": now,
		}).Error
}

// orderEnvironments is the preload scope that keeps environments in the order
// the user arranged them.
func orderEnvironments(db *gorm.DB) *gorm.DB {
	return db.Order("project_environments.position asc, project_environments.created_at asc")
}
