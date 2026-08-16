package sshkey

import "time"

// KeyType is the algorithm a keypair was generated with.
type KeyType string

const (
	KeyTypeEd25519 KeyType = "ed25519"
	KeyTypeRSA     KeyType = "rsa"
)

// SshKey is a keypair generated and owned by stacker. The private half stays on
// disk under config.KeyDir and is never returned by the API.
type SshKey struct {
	ID   string  `gorm:"primaryKey;size:32" json:"id"`
	Name string  `gorm:"uniqueIndex;size:120;not null" json:"name"`
	Type KeyType `gorm:"size:16;not null" json:"type"`

	// PublicKey is the OpenSSH public key line.
	PublicKey string `gorm:"not null" json:"publicKey"`
	// Fingerprint is the SHA256 form printed by `ssh-keygen -lf`.
	Fingerprint string `gorm:"size:120;not null" json:"fingerprint"`

	// PrivateKeyPath points at the file inside the key folder. Kept out of JSON.
	PrivateKeyPath string `gorm:"not null" json:"-"`
	IsDefault      bool   `gorm:"not null;default:false;index" json:"isDefault"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateRequest is the payload accepted by POST /api/ssh-keys. Keys are
// create-and-delete only — renaming would break every node already trusting it.
type CreateRequest struct {
	Name string  `json:"name" binding:"required,min=1,max=120"`
	Type KeyType `json:"type" binding:"required,oneof=ed25519 rsa"`
}
