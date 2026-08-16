package github

import "time"

// App is the single GitHub App owned by this Stacker installation.
type App struct {
	ID             string    `gorm:"primaryKey;size:32" json:"id"`
	Name           string    `gorm:"size:100;not null" json:"name"`
	AppID          int64     `gorm:"not null;default:0" json:"appId"`
	Slug           string    `gorm:"size:100;not null;default:''" json:"slug"`
	ClientID       string    `gorm:"size:100;not null;default:''" json:"-"`
	ClientSecret   string    `gorm:"not null;default:''" json:"-"`
	WebhookSecret  string    `gorm:"not null;default:''" json:"-"`
	PrivateKey     string    `gorm:"not null;default:''" json:"-"`
	CallbackSecret string    `gorm:"size:64;not null" json:"-"`
	InstallationID int64     `gorm:"not null;default:0" json:"installationId"`
	AccountLogin   string    `gorm:"size:200;not null;default:''" json:"account"`
	AccountType    string    `gorm:"size:40;not null;default:''" json:"accountType"`
	RepositoryMode string    `gorm:"size:20;not null;default:''" json:"repositorySelection"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type CreateRequest struct {
	Name         string `json:"name" binding:"required,min=1,max=100"`
	BaseURL      string `json:"baseUrl" binding:"required,url"`
	Organization string `json:"organization" binding:"omitempty,max=100"`
}

type ManifestStart struct {
	URL      string         `json:"url"`
	Manifest map[string]any `json:"manifest"`
}

type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"fullName"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"htmlUrl"`
}
