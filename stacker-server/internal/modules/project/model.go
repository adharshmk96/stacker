package project

import "time"

// SourceKind is where a project's compose file comes from.
type SourceKind string

const (
	// SourceGit clones a repository and reads the compose file out of it.
	SourceGit SourceKind = "git"
	// SourceCompose uses YAML stored on the project itself.
	SourceCompose SourceKind = "compose"
)

// GitProvider is only a label today: the clone URL is what actually decides
// where the repo is fetched from, and GitHub is the one provider whose
// credentials stacker can supply (see the github module).
type GitProvider string

// TriggerKind is what starts a deployment.
type TriggerKind string

const (
	TriggerManual   TriggerKind = "manual"
	TriggerPush     TriggerKind = "push"
	TriggerTag      TriggerKind = "tag"
	TriggerSchedule TriggerKind = "schedule"
)

// DeployStrategy maps onto swarm's update order: `rolling` starts the new task
// before stopping the old one, `recreate` stops first.
type DeployStrategy string

const (
	StrategyRolling  DeployStrategy = "rolling"
	StrategyRecreate DeployStrategy = "recreate"
)

// TLSMode is how a domain is served. `auto` asks Traefik's ACME resolver for a
// certificate; `custom` expects one already loaded on the proxy; `none` serves
// plain http.
type TLSMode string

const (
	TLSAuto   TLSMode = "auto"
	TLSCustom TLSMode = "custom"
	TLSNone   TLSMode = "none"
)

// GitSource is the repository half of a project. It is stored as one JSON
// column because nothing queries inside it.
type GitSource struct {
	Provider GitProvider `json:"provider"`
	// Repo is either `owner/name` or a full clone URL; cloneURL normalises it.
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	// ComposePath is the compose file's path inside the repository.
	ComposePath string `json:"composePath"`
}

// EnvVar is one variable. Secrets use the same shape — the difference is that
// the secrets list is redacted on the way out to the browser.
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Domain is one hostname routed to one compose service.
type Domain struct {
	ID   string `json:"id"`
	Host string `json:"host"`
	// Service is the compose service name, not the swarm service name: the
	// stack prefix is added when the route is written.
	Service     string  `json:"service"`
	Port        int     `json:"port"`
	TLS         TLSMode `json:"tls"`
	RedirectWww bool    `json:"redirectWww"`
}

type DeployTrigger struct {
	Kind TriggerKind `json:"kind"`
	// Pattern is a tag glob for `tag` and a cron expression for `schedule`.
	Pattern string `json:"pattern"`
}

// DeploySettings is how an environment rolls out. Replicas and placement are
// only applied to services that do not pin their own in the compose file, so a
// database with `deploy.replicas: 1` is never scaled by accident.
type DeploySettings struct {
	Strategy       DeployStrategy `json:"strategy"`
	Replicas       int            `json:"replicas"`
	Placement      string         `json:"placement"`
	HealthGraceSec int            `json:"healthGraceSec"`
	AutoRollback   bool           `json:"autoRollback"`
	AlwaysPull     bool           `json:"alwaysPull"`
}

// Environment is one deployable target of a project. It owns everything that
// differs between staging and production; the source is shared.
//
// Each environment maps to exactly one swarm stack, named by StackName.
type Environment struct {
	ID        string `gorm:"primaryKey;size:32" json:"id"`
	ProjectID string `gorm:"size:32;not null;index" json:"-"`

	Name string `gorm:"size:60;not null" json:"name"`
	// Branch overrides the project branch for git sources; blank inherits it.
	Branch string `gorm:"size:200;not null;default:''" json:"branch"`

	Variables []EnvVar `gorm:"serializer:json" json:"variables"`
	Secrets   []EnvVar `gorm:"serializer:json" json:"secrets"`
	Domains   []Domain `gorm:"serializer:json" json:"domains"`

	Trigger DeployTrigger  `gorm:"serializer:json" json:"trigger"`
	Deploy  DeploySettings `gorm:"serializer:json" json:"deploy"`

	// Position keeps the order the user arranged the environments in, since
	// the ids are random and would otherwise sort arbitrarily.
	Position int `gorm:"not null;default:0" json:"-"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName keeps the generic name `environments` out of the schema — it would
// read like a global table rather than one owned by projects.
func (Environment) TableName() string { return "project_environments" }

// Project is one application stacker deploys. Its environments are always
// loaded with it: every page that shows a project shows them too, and a project
// without an environment has nothing to deploy.
type Project struct {
	ID          string     `gorm:"primaryKey;size:32" json:"id"`
	Name        string     `gorm:"uniqueIndex;size:120;not null" json:"name"`
	Description string     `gorm:"size:500;not null;default:''" json:"description"`
	SourceKind  SourceKind `gorm:"size:16;not null;default:git" json:"sourceKind"`

	Git GitSource `gorm:"serializer:json" json:"git"`
	// Compose is the raw YAML, used when SourceKind is SourceCompose.
	Compose string `gorm:"not null;default:''" json:"compose"`

	Environments []Environment `gorm:"foreignKey:ProjectID" json:"environments"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DeploymentStatus is a run's lifecycle. Only one run per environment is ever
// `queued` or `running`.
type DeploymentStatus string

const (
	StatusQueued    DeploymentStatus = "queued"
	StatusRunning   DeploymentStatus = "running"
	StatusSucceeded DeploymentStatus = "succeeded"
	StatusFailed    DeploymentStatus = "failed"
	StatusCancelled DeploymentStatus = "cancelled"
)

// Done reports whether the run has stopped moving, which is what decides
// whether its logs still come from the live buffer or from the row.
func (s DeploymentStatus) Done() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCancelled
}

// Deployment is one build-and-deploy run.
//
// The project and environment names are copied onto the row rather than joined:
// a run is a record of what happened, and renaming a project afterwards must
// not rewrite its history.
type Deployment struct {
	ID     string `gorm:"primaryKey;size:32" json:"id"`
	Number int    `gorm:"not null;index" json:"number"`

	ProjectID     string `gorm:"size:32;not null;index" json:"projectId"`
	ProjectName   string `gorm:"size:120;not null" json:"projectName"`
	EnvironmentID string `gorm:"size:32;not null;index" json:"environmentId"`
	// Environment is the name, which is what the UI filters and groups on.
	Environment string `gorm:"size:60;not null" json:"environment"`
	// Stack is the swarm stack this run deployed, kept so a run can explain
	// which stack it touched even after the environment is renamed.
	Stack string `gorm:"size:120;not null;default:''" json:"stack"`

	Status      DeploymentStatus `gorm:"size:16;not null;default:queued" json:"status"`
	TriggeredBy TriggerKind      `gorm:"size:16;not null;default:manual" json:"triggeredBy"`
	// Actor is who or what started the run — a user's email, or the trigger.
	Actor string `gorm:"size:200;not null;default:''" json:"actor"`
	// Revision is the commit sha that was built, or `compose` for an inline
	// source. It is `pending` until the clone has run.
	Revision string `gorm:"size:60;not null;default:''" json:"revision"`
	Message  string `gorm:"size:500;not null;default:''" json:"message"`
	// Error is the reason a failed run gives, short enough for a toast. The
	// full story is in the log.
	Error string `gorm:"size:1000;not null;default:''" json:"error,omitempty"`

	StartedAt   time.Time  `gorm:"not null" json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	DurationSec *int       `json:"durationSec,omitempty"`

	// Log is the whole run output, persisted when the run ends. While the run
	// is live the lines are served from the in-memory buffer instead, so the
	// database is not written to once per line.
	Log string `gorm:"not null;default:''" json:"-"`
}

/* ---- live status ---- */

// RuntimeState is what docker reports for an environment's stack right now.
type RuntimeState string

const (
	// RuntimeRunning means every service has all its tasks up.
	RuntimeRunning RuntimeState = "running"
	// RuntimeDegraded means the stack exists but some tasks are missing —
	// mid-rollout, or crash-looping.
	RuntimeDegraded RuntimeState = "degraded"
	// RuntimeStopped means no service of the stack exists.
	RuntimeStopped RuntimeState = "stopped"
	// RuntimeDeploying means a run is working on the stack.
	RuntimeDeploying RuntimeState = "deploying"
	// RuntimeUnknown means docker could not be asked.
	RuntimeUnknown RuntimeState = "unknown"
)

// ServiceState is one swarm service of an environment's stack.
type ServiceState struct {
	// Name is the compose service name, with the stack prefix removed.
	Name  string `json:"name"`
	Stack string `json:"stack"`
	Image string `json:"image"`
	Mode  string `json:"mode"`
	// Running and Desired come from docker's `2/3` replicas column.
	Running int `json:"running"`
	Desired int `json:"desired"`
}

// EnvironmentStatus is the live view of one environment: what docker is running
// and what the last run did.
type EnvironmentStatus struct {
	EnvironmentID string         `json:"environmentId"`
	Name          string         `json:"name"`
	Stack         string         `json:"stack"`
	State         RuntimeState   `json:"state"`
	Services      []ServiceState `json:"services"`
	Running       int            `json:"running"`
	Desired       int            `json:"desired"`
	// Message explains a non-running state — docker's own error, usually.
	Message string `json:"message,omitempty"`
	// Domains are the hostnames currently routed to this environment.
	Domains        []string    `json:"domains"`
	LastDeployment *Deployment `json:"lastDeployment,omitempty"`
}

// ProjectStatus is one project's live view, as the cards and the detail page
// poll it.
type ProjectStatus struct {
	ProjectID    string              `json:"projectId"`
	State        RuntimeState        `json:"state"`
	Environments []EnvironmentStatus `json:"environments"`
	// LastDeployment is the newest run across every environment.
	LastDeployment *Deployment `json:"lastDeployment,omitempty"`
	// CheckedAt lets the UI show how fresh the reading is.
	CheckedAt time.Time `json:"checkedAt"`
}

/* ---- requests ---- */

// DomainRequest is one domain as the browser sends it. The id is accepted so an
// edit keeps its row, and generated when it is blank.
type DomainRequest struct {
	ID          string  `json:"id"`
	Host        string  `json:"host"`
	Service     string  `json:"service"`
	Port        int     `json:"port"`
	TLS         TLSMode `json:"tls"`
	RedirectWww bool    `json:"redirectWww"`
}

// EnvironmentRequest is one environment of a project write. Environments are
// replaced wholesale on every save, so an id that is absent from the payload is
// a deletion.
type EnvironmentRequest struct {
	ID        string          `json:"id"`
	Name      string          `json:"name" binding:"required,min=1,max=60"`
	Branch    string          `json:"branch"`
	Variables []EnvVar        `json:"variables"`
	Secrets   []EnvVar        `json:"secrets"`
	Domains   []DomainRequest `json:"domains"`
	Trigger   DeployTrigger   `json:"trigger"`
	Deploy    DeploySettings  `json:"deploy"`
}

// WriteRequest is the payload for both POST /api/projects and PUT /api/projects/:id.
// One shape for both: the create form and the detail tabs submit the same
// project, so a second half-overlapping request type would only drift.
type WriteRequest struct {
	Name         string               `json:"name" binding:"required,min=1,max=120"`
	Description  string               `json:"description" binding:"max=500"`
	SourceKind   SourceKind           `json:"sourceKind" binding:"required"`
	Git          GitSource            `json:"git"`
	Compose      string               `json:"compose"`
	Environments []EnvironmentRequest `json:"environments" binding:"required,min=1,dive"`
}

// DeployRequest is the payload for a manual deploy. Both fields are optional —
// the button sends neither.
type DeployRequest struct {
	Message     string      `json:"message" binding:"max=500"`
	Actor       string      `json:"actor" binding:"max=200"`
	TriggeredBy TriggerKind `json:"-"`
	Revision    string      `json:"-"`
}

// LogChunk is a slice of a run's output, read with a cursor so the browser can
// poll for the tail without re-reading what it already has.
type LogChunk struct {
	DeploymentID string           `json:"deploymentId"`
	Status       DeploymentStatus `json:"status"`
	// Lines are the lines after the requested cursor.
	Lines []string `json:"lines"`
	// Next is the cursor to send on the following poll.
	Next int `json:"next"`
	// Done is true once no more lines will arrive.
	Done bool `json:"done"`
}
