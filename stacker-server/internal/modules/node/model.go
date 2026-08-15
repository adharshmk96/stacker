package node

import "time"

// KeyStatus tracks whether stacker's key authenticates against the host.
type KeyStatus string

const (
	KeyStatusUnknown KeyStatus = "unknown"
	KeyStatusOK      KeyStatus = "ok"
	KeyStatusFailed  KeyStatus = "failed"
)

// SwarmRole is a node's place in the docker swarm stacker deploys to.
type SwarmRole string

const (
	// SwarmRoleNone is a node that has not been configured into the swarm yet.
	SwarmRoleNone SwarmRole = "none"
	// SwarmRoleManager runs the swarm control plane. The local node becomes the
	// first manager; further managers are promoted workers.
	SwarmRoleManager SwarmRole = "manager"
	SwarmRoleWorker  SwarmRole = "worker"
)

// SwarmPort is the port managers listen on for join and control-plane traffic.
const SwarmPort = 2377

// LocalID is the fixed id of the node stacker itself runs on. It is a constant
// rather than a random id so the row can be looked up (and re-seeded) without a
// scan, and so the UI can recognise it.
const LocalID = "local"

// Node is a host stacker deploys to. The table is named `nodes` explicitly so
// the ssh key module can count references without importing this package.
type Node struct {
	ID   string `gorm:"primaryKey;size:32" json:"id"`
	Name string `gorm:"uniqueIndex;size:120;not null" json:"name"`

	// Ssh is the connection string in `user@host` form. Empty for the local
	// node, which is reached by running commands directly.
	Ssh  string `gorm:"size:255;not null" json:"ssh"`
	Port int    `gorm:"not null;default:22" json:"port"`

	// SshKeyID is required for remote nodes: the key is installed with
	// ssh-copy-id and used for every connection afterwards. Empty for the local
	// node, which needs no ssh at all.
	SshKeyID string `gorm:"size:32;not null;index" json:"sshKeyId"`

	// Local marks the machine stacker is installed on. Exactly one row carries
	// it; it is seeded at startup and cannot be created or deleted over the API.
	Local bool `gorm:"not null;default:false" json:"local"`

	KeyStatus    KeyStatus  `gorm:"size:16;not null;default:unknown" json:"keyStatus"`
	KeyCheckedAt *time.Time `json:"keyCheckedAt,omitempty"`

	// SwarmRole is what stacker last observed on the host, not what it wishes
	// were true: every write follows a docker command that succeeded.
	SwarmRole SwarmRole `gorm:"size:16;not null;default:none" json:"swarmRole"`

	// SwarmNodeID is docker's own id for the node, needed to promote, demote or
	// remove it from a manager. Empty until the node joins.
	SwarmNodeID string `gorm:"size:64;not null;default:''" json:"swarmNodeId"`

	// SwarmAddr is the address other nodes reach this one on. It is only
	// meaningful for managers, where it is the advertise address workers join
	// against; `<addr>:2377` is the join endpoint.
	SwarmAddr string `gorm:"size:255;not null;default:''" json:"swarmAddr"`

	// SwarmError is the last failure seen while configuring or refreshing, kept
	// so the row can explain itself without re-running anything.
	SwarmError    string     `gorm:"size:500;not null;default:''" json:"swarmError"`
	SwarmSyncedAt *time.Time `json:"swarmSyncedAt,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName pins the table name the sshkey module queries by hand.
func (Node) TableName() string { return "nodes" }

// CreateRequest is the payload for POST /api/nodes.
type CreateRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Ssh      string `json:"ssh" binding:"required"`
	Port     int    `json:"port" binding:"omitempty,min=1,max=65535"`
	SshKeyID string `json:"sshKeyId" binding:"required"`
}

// UpdateRequest is the payload for PUT /api/nodes/:id. Every field is replaced.
// The local node accepts a rename only, so its ssh fields may be omitted.
type UpdateRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Ssh      string `json:"ssh"`
	Port     int    `json:"port" binding:"omitempty,min=1,max=65535"`
	SshKeyID string `json:"sshKeyId"`
}

// KeyCheckResult is the outcome of a key check or an ssh-copy-id run.
type KeyCheckResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// InstallKeyRequest describes a one-off ssh-copy-id run. It takes the
// connection details directly rather than a Node id, because the UI installs the
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

// ConfigureRequest is the payload for POST /api/nodes/:id/swarm/configure.
type ConfigureRequest struct {
	// AdvertiseAddr overrides the address a manager advertises to the rest of
	// the swarm. It is only read when the node is being initialised as the
	// first manager, where a host with several interfaces would otherwise make
	// docker refuse to guess. Ignored for workers.
	AdvertiseAddr string `json:"advertiseAddr"`
}

// SwarmResult is what every swarm mutation answers with: the node as it now
// stands, plus a line describing what happened.
type SwarmResult struct {
	Node    Node   `json:"node"`
	Message string `json:"message"`
}
