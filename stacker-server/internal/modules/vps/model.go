package vps

import "time"

// KeyStatus tracks whether stacker's key authenticates against the host.
type KeyStatus string

const (
	KeyStatusUnknown KeyStatus = "unknown"
	KeyStatusOK      KeyStatus = "ok"
	KeyStatusFailed  KeyStatus = "failed"
)

// Vps is a remote host stacker deploys to. The table is named `vps` explicitly
// so the ssh key module can count references without importing this package.
type Vps struct {
	ID   string `gorm:"primaryKey;size:32" json:"id"`
	Name string `gorm:"uniqueIndex;size:120;not null" json:"name"`

	// Ssh is the connection string in `user@host` form.
	Ssh  string `gorm:"size:255;not null" json:"ssh"`
	Port int    `gorm:"not null;default:22" json:"port"`

	// SshKeyID is required: the key is installed with ssh-copy-id and used for
	// every connection afterwards.
	SshKeyID string `gorm:"size:32;not null;index" json:"sshKeyId"`

	KeyStatus    KeyStatus  `gorm:"size:16;not null;default:unknown" json:"keyStatus"`
	KeyCheckedAt *time.Time `json:"keyCheckedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName pins the table name the sshkey module queries by hand.
func (Vps) TableName() string { return "vps" }

// CreateRequest is the payload for POST /api/vps.
type CreateRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Ssh      string `json:"ssh" binding:"required"`
	Port     int    `json:"port" binding:"omitempty,min=1,max=65535"`
	SshKeyID string `json:"sshKeyId" binding:"required"`
}

// UpdateRequest is the payload for PUT /api/vps/:id. Every field is replaced.
type UpdateRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Ssh      string `json:"ssh" binding:"required"`
	Port     int    `json:"port" binding:"omitempty,min=1,max=65535"`
	SshKeyID string `json:"sshKeyId" binding:"required"`
}

// KeyCheckResult is the outcome of a key check or an ssh-copy-id run.
type KeyCheckResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// InstallKeyRequest describes a one-off ssh-copy-id run. It takes the
// connection details directly rather than a VPS id, because the UI installs the
// key while the host is still being filled in — before any record exists.
//
// Password is never stored: it goes straight to the ssh-copy-id process and is
// dropped when the request ends.
type InstallKeyRequest struct {
	Ssh      string `json:"ssh" binding:"required"`
	Port     int    `json:"port" binding:"omitempty,min=1,max=65535"`
	SshKeyID string `json:"sshKeyId" binding:"required"`
	Password string `json:"password" binding:"required"`
}
