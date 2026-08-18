package project

import "errors"

var (
	ErrNotFound        = errors.New("project not found")
	ErrEnvNotFound     = errors.New("environment not found")
	ErrDeployNotFound  = errors.New("deployment not found")
	ErrNameTaken       = errors.New("a project with that name already exists")
	ErrInvalidName     = errors.New("use letters, numbers, dashes and underscores")
	ErrInvalidSource   = errors.New("source must be git or compose")
	ErrRepoRequired    = errors.New("a repository is required for a git source")
	ErrBranchRequired  = errors.New("a branch is required for a git source")
	ErrComposePath     = errors.New("a path to the compose file is required")
	ErrComposeRequired = errors.New("paste a compose file")
	ErrEnvRequired     = errors.New("a project needs at least one environment")
	ErrEnvName         = errors.New("environment names must be unique, and use letters, numbers and dashes")
	ErrDomainHost      = errors.New("enter a valid hostname without a scheme or path")
	ErrDomainService   = errors.New("every domain needs the compose service it routes to")
	ErrDomainPort      = errors.New("a domain port must be between 1 and 65535")
	ErrDomainTaken     = errors.New("that hostname is already routed by another environment")
	ErrReplicas        = errors.New("replicas must be between 1 and 100")
	ErrTagPattern      = errors.New("enter a tag pattern, such as v* or v1.*")
	ErrCronExpression  = errors.New("enter a cron expression, such as 0 3 * * *")
	ErrTriggerSource   = errors.New("push and tag triggers need a git source")

	// ErrAlreadyDeploying guards the one-run-per-environment rule: two stack
	// deploys of the same stack would race each other's rollout.
	ErrAlreadyDeploying = errors.New("a deployment is already running for this environment")
	// ErrNotRunning is returned when a finished run is asked to cancel.
	ErrNotRunning = errors.New("this deployment has already finished")

	// ErrComposeInvalid wraps a compose file stacker could not read. The
	// wrapped text is the parser's own complaint.
	ErrComposeInvalid = errors.New("the compose file could not be read")
	// ErrNoServices is a compose file that parses but declares nothing to run.
	ErrNoServices = errors.New("the compose file declares no services")
	// ErrUnknownService is a domain pointing at a service the compose file does
	// not define. It is only reachable at deploy time, since the compose file is
	// not in hand when the project is saved.
	ErrUnknownService = errors.New("the compose file has no such service")
	// ErrTraefikMissing means the installed Traefik config directory is absent,
	// so no hostname can be published.
	ErrTraefikMissing = errors.New("traefik configuration is not available on this installation")
)
