package swarm

// Resource is one docker resource list the Swarm menu shows. The value is also
// the URL segment (`/api/swarm/services`), so route, tab and dataset all share
// the one identifier.
type Resource string

const (
	Stacks     Resource = "stacks"
	Services   Resource = "services"
	Tasks      Resource = "tasks"
	Containers Resource = "containers"
	Images     Resource = "images"
	Volumes    Resource = "volumes"
	Networks   Resource = "networks"
	Secrets    Resource = "secrets"
	Configs    Resource = "configs"
)

// Scope says which daemon answers for a resource.
//
// A swarm-scoped resource is held by the manager and asked for once. A
// node-scoped one exists separately on every node, so the list is the
// concatenation of what each node reports and every row carries its node.
type Scope string

const (
	ScopeSwarm Scope = "swarm"
	ScopeNode  Scope = "node"
)

// Row is one entry of a resource list. Keys are the column keys the UI renders,
// and every value is a string: docker's own `{{json .}}` output is all strings,
// and normalising further would only invent precision the CLI did not give us.
type Row map[string]string

// NodeRef is a node as the resource lists refer to it — enough for the node
// filter and the badge in a row, without the connection details.
type NodeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Role is the docker swarm role: manager or worker.
	Role string `json:"role"`
	// Online is the last reachability reading stacker took.
	Online bool `json:"online"`
}

// NodeError is one node that could not be read. A single unreachable worker
// must not turn the whole list into an error page, so failures travel beside
// the rows instead of replacing them.
type NodeError struct {
	Node    string `json:"node"`
	Message string `json:"message"`
}

// ListResult is the answer to GET /api/swarm/:resource.
type ListResult struct {
	Resource Resource    `json:"resource"`
	Scope    Scope       `json:"scope"`
	Rows     []Row       `json:"rows"`
	Nodes    []NodeRef   `json:"nodes"`
	Errors   []NodeError `json:"errors"`
}

// ActionRequest is the payload for POST /api/swarm/:resource/action.
//
// One endpoint serves every row action because the UI's action menu is itself
// table-driven: it knows an action key, the row's id and (for node-scoped
// resources) which node the row was read from. Whether that maps to
// `docker service rm` or `docker container restart` is this module's business.
type ActionRequest struct {
	Action string `json:"action" binding:"required"`
	// ID is the row's docker identifier — a name for stacks, services, volumes,
	// networks, secrets and configs; an id for tasks, containers and images.
	ID string `json:"id" binding:"required"`
	// Node is the node the row was read from. Required for node-scoped
	// resources, ignored for swarm-scoped ones.
	Node string `json:"node"`
	// Replicas is the target count for `scale`.
	Replicas *int `json:"replicas"`
}

// CreateRequest is the payload for POST /api/swarm/:resource. Only the fields
// that resource uses are read; the rest are ignored.
type CreateRequest struct {
	Name string `json:"name"`
	// Node is which node to create on, for node-scoped resources.
	Node string `json:"node"`

	// Image and Replicas create a service; Image alone pulls an image.
	Image    string `json:"image"`
	Replicas *int   `json:"replicas"`

	// Driver is the network or volume driver. Empty means docker's default,
	// which is `overlay` for a swarm network and `local` for a volume.
	Driver string `json:"driver"`

	// Content is a compose file (stacks) or the value of a secret or config.
	Content string `json:"content"`
}

// ActionResult is what a mutation answers with: a line for the toast, plus any
// output worth showing (logs, an inspect dump, a config's content).
type ActionResult struct {
	Message string `json:"message"`
	// Output is filled only by the actions that read something back.
	Output string `json:"output,omitempty"`
}
