package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
)

// Service holds every decision this module makes. The handlers do HTTP, the
// repository does SQL, the engine does docker; validation, the secret-merge rule
// and the ordering between the three live here.
type Service struct {
	repo   *Repository
	engine *engine
	status *statusReader
	routes *router
	log    *slog.Logger
}

// Options is what the module needs from the installation to be able to deploy.
type Options struct {
	// WorkRoot is the parent directory runs clone and build under.
	WorkRoot string
	// TraefikDynamicPath is the installed dynamic-config file; its directory is
	// where generated routes are written.
	TraefikDynamicPath string
	// Network is the attachable overlay Traefik shares with deployed services.
	Network string
	// Token supplies a git credential for private repositories. Optional.
	Token tokenFunc
}

func NewService(repo *Repository, opts Options, log *slog.Logger) *Service {
	routes := newRouter(opts.TraefikDynamicPath)

	return &Service{
		repo:   repo,
		engine: newEngine(repo, routes, opts.WorkRoot, opts.Network, opts.Token, log),
		status: &statusReader{exec: execCommand},
		routes: routes,
		log:    log,
	}
}

// Recover is called once at startup. A run only exists inside the process that
// started it, so a row still marked running after a restart describes work
// nobody is doing — it is closed out rather than left to look live forever.
func (s *Service) Recover() error {
	if err := os.MkdirAll(s.engine.workRoot, 0o700); err != nil {
		return err
	}
	// Workspaces are per run and the runs are gone, so whatever is left under
	// the work root is abandoned build context.
	entries, err := os.ReadDir(s.engine.workRoot)
	if err == nil {
		for _, entry := range entries {
			if err := os.RemoveAll(s.engine.workRoot + "/" + entry.Name()); err != nil {
				s.log.Warn("could not remove an abandoned workspace", "name", entry.Name(), "error", err)
			}
		}
	}
	return s.repo.ResetRunning("the server restarted while this deployment was running")
}

/* ---- reads ---- */

// List returns every project, secrets redacted.
func (s *Service) List() ([]Project, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		s.hideSecrets(&items[i])
	}
	return items, nil
}

func (s *Service) Get(id string) (Project, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return Project{}, err
	}
	s.hideSecrets(&item)
	return item, nil
}

// hideSecrets blanks stored secret values on the way out. The browser needs the
// keys — it renders them — but never the values, and a response is one more
// place a value could be read from.
func (s *Service) hideSecrets(item *Project) {
	for i := range item.Environments {
		item.Environments[i].Secrets = redact(item.Environments[i].Secrets)
	}
}

/* ---- writes ---- */

// Create validates and stores a project.
func (s *Service) Create(req WriteRequest) (Project, error) {
	item, err := s.build(Project{ID: newID()}, req, nil)
	if err != nil {
		return Project{}, err
	}

	taken, err := s.repo.ExistsByName(item.Name, "")
	if err != nil {
		return Project{}, err
	}
	if taken {
		return Project{}, ErrNameTaken
	}
	if err := s.checkHosts(item); err != nil {
		return Project{}, err
	}

	if err := s.repo.Create(&item); err != nil {
		return Project{}, err
	}
	s.hideSecrets(&item)
	return item, nil
}

// Update replaces a project. Environments are matched by id: one that is absent
// from the payload is deleted, and its stack and routes are torn down with it —
// leaving a stack running for an environment the user just removed would be a
// service nothing in the UI can reach any more.
func (s *Service) Update(ctx context.Context, id string, req WriteRequest) (Project, error) {
	existing, err := s.repo.Get(id)
	if err != nil {
		return Project{}, err
	}

	item, err := s.build(existing, req, existing.Environments)
	if err != nil {
		return Project{}, err
	}

	taken, err := s.repo.ExistsByName(item.Name, id)
	if err != nil {
		return Project{}, err
	}
	if taken {
		return Project{}, ErrNameTaken
	}
	if err := s.checkHosts(item); err != nil {
		return Project{}, err
	}

	kept := map[string]bool{}
	for _, env := range item.Environments {
		kept[env.ID] = true
	}
	var removed []string
	for _, env := range existing.Environments {
		if !kept[env.ID] {
			removed = append(removed, env.ID)
			if err := s.engine.Teardown(ctx, env.ID, StackName(existing, env)); err != nil {
				s.log.Error("could not tear down a removed environment",
					"project", existing.Name, "environment", env.Name, "error", err)
			}
		}
	}

	if err := s.repo.Save(&item, removed); err != nil {
		return Project{}, err
	}

	// A rename changes the stack name, so the old stack would otherwise keep
	// running under a name nothing points at any more.
	if slug(existing.Name) != slug(item.Name) {
		for _, env := range existing.Environments {
			if !kept[env.ID] {
				continue
			}
			if err := s.engine.Teardown(ctx, env.ID, StackName(existing, env)); err != nil {
				s.log.Error("could not remove the stack left by a rename",
					"project", existing.Name, "environment", env.Name, "error", err)
			}
		}
	}

	s.hideSecrets(&item)
	return item, nil
}

// Delete removes a project and everything it is running.
func (s *Service) Delete(ctx context.Context, id string) error {
	item, err := s.repo.Get(id)
	if err != nil {
		return err
	}

	for _, env := range item.Environments {
		if err := s.engine.Teardown(ctx, env.ID, StackName(item, env)); err != nil {
			// Reported, not fatal: the record has to go even when docker is
			// unreachable, or the project becomes impossible to delete.
			s.log.Error("could not tear down an environment of a deleted project",
				"project", item.Name, "environment", env.Name, "error", err)
		}
	}
	return s.repo.Delete(id)
}

// build turns a write request into a Project, validating as it goes.
//
// `previous` is the stored environment set on an update, and nil on a create. It
// is what makes the secret-merge rule work: values are redacted on the way out,
// so a save round-trips blank values for secrets the user did not touch, and a
// blank one has to mean "keep what is stored" rather than "erase it".
func (s *Service) build(base Project, req WriteRequest, previous []Environment) (Project, error) {
	name := strings.TrimSpace(req.Name)
	if !namePattern.MatchString(name) {
		return Project{}, ErrInvalidName
	}

	item := base
	item.Name = name
	item.Description = strings.TrimSpace(req.Description)
	item.SourceKind = req.SourceKind

	switch req.SourceKind {
	case SourceGit:
		item.Git = GitSource{
			Provider:    req.Git.Provider,
			Repo:        strings.TrimSpace(req.Git.Repo),
			Branch:      strings.TrimSpace(req.Git.Branch),
			ComposePath: strings.TrimSpace(req.Git.ComposePath),
		}
		if item.Git.Repo == "" {
			return Project{}, ErrRepoRequired
		}
		if item.Git.Branch == "" {
			return Project{}, ErrBranchRequired
		}
		if item.Git.ComposePath == "" {
			return Project{}, ErrComposePath
		}
		// The compose file is kept: switching a project to `compose` and back
		// should not silently discard what was pasted before.
		item.Compose = req.Compose

	case SourceCompose:
		item.Compose = req.Compose
		if strings.TrimSpace(item.Compose) == "" {
			return Project{}, ErrComposeRequired
		}
		// Read now rather than at deploy time: a pasted file that cannot parse
		// is a form error, and the form is where it can be fixed.
		if _, err := parseCompose(item.Compose); err != nil {
			return Project{}, err
		}
		item.Git = req.Git

	default:
		return Project{}, ErrInvalidSource
	}

	if len(req.Environments) == 0 {
		return Project{}, ErrEnvRequired
	}

	stored := map[string]Environment{}
	for _, env := range previous {
		stored[env.ID] = env
	}

	seen := map[string]bool{}
	item.Environments = make([]Environment, 0, len(req.Environments))
	for position, envReq := range req.Environments {
		env, err := s.buildEnvironment(item.ID, position, envReq, stored)
		if err != nil {
			return Project{}, err
		}
		key := strings.ToLower(env.Name)
		if seen[key] {
			return Project{}, ErrEnvName
		}
		seen[key] = true
		item.Environments = append(item.Environments, env)
	}
	return item, nil
}

func (s *Service) buildEnvironment(
	projectID string,
	position int,
	req EnvironmentRequest,
	stored map[string]Environment,
) (Environment, error) {
	name := strings.TrimSpace(req.Name)
	if !envNamePattern.MatchString(name) {
		return Environment{}, ErrEnvName
	}

	// An id the payload carries is only honoured when it is one of this
	// project's own: accepting an arbitrary id would let a save reassign
	// another project's environment.
	env, existed := stored[req.ID]
	if !existed {
		env = Environment{ID: newID()}
	}

	env.ProjectID = projectID
	env.Name = name
	env.Branch = strings.TrimSpace(req.Branch)
	env.Position = position
	env.Variables = cleanVars(req.Variables)
	env.Secrets = mergeSecrets(cleanVars(req.Secrets), env.Secrets)
	env.Trigger = cleanTrigger(req.Trigger)

	deploy, err := cleanDeploy(req.Deploy)
	if err != nil {
		return Environment{}, err
	}
	env.Deploy = deploy

	domains, err := cleanDomains(req.Domains)
	if err != nil {
		return Environment{}, err
	}
	env.Domains = domains

	return env, nil
}

// checkHosts rejects a hostname another project's environment already routes.
// Routes share one Traefik directory, so two claims on a host would resolve by
// whichever file was written last — a coin flip is not an acceptable answer to
// "where does my domain point".
func (s *Service) checkHosts(item Project) error {
	mine := map[string]bool{}
	for _, env := range item.Environments {
		mine[env.ID] = true
	}

	claimed := map[string]bool{}
	for _, env := range item.Environments {
		for _, domain := range env.Domains {
			if claimed[domain.Host] {
				return fmt.Errorf("%w: %s", ErrDomainTaken, domain.Host)
			}
			claimed[domain.Host] = true

			owner, err := s.repo.HostOwner(domain.Host)
			if err != nil {
				return err
			}
			if owner != "" && !mine[owner] {
				return fmt.Errorf("%w: %s", ErrDomainTaken, domain.Host)
			}
		}
	}
	return nil
}

/* ---- deploying ---- */

// Deploy starts a run for one environment.
func (s *Service) Deploy(id, envID string, req DeployRequest) (Deployment, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return Deployment{}, err
	}
	env, err := findEnv(item, envID)
	if err != nil {
		return Deployment{}, err
	}
	return s.engine.Start(item, env, req)
}

// HandlePush queues every push-enabled environment whose GitHub repository and
// effective branch match the verified webhook event.
func (s *Service) HandlePush(repository, branch, actor, revision, message string) ([]Deployment, error) {
	repository = canonicalGitHubRepo(repository)
	branch = strings.TrimSpace(branch)
	if repository == "" || branch == "" {
		return nil, nil
	}

	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	queued := make([]Deployment, 0)
	for _, item := range items {
		if item.SourceKind != SourceGit || canonicalGitHubRepo(item.Git.Repo) != repository {
			continue
		}
		for _, env := range item.Environments {
			effectiveBranch := strings.TrimSpace(env.Branch)
			if effectiveBranch == "" {
				effectiveBranch = strings.TrimSpace(item.Git.Branch)
			}
			if env.Trigger.Kind != TriggerPush || effectiveBranch != branch {
				continue
			}

			deployment, deployErr := s.engine.Start(item, env, DeployRequest{
				Actor: actor, Message: message, TriggeredBy: TriggerPush, Revision: revision,
			})
			if errors.Is(deployErr, ErrAlreadyDeploying) {
				continue
			}
			if deployErr != nil {
				return queued, deployErr
			}
			queued = append(queued, deployment)
		}
	}
	return queued, nil
}

func canonicalGitHubRepo(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "git@github.com:") {
		value = strings.TrimPrefix(value, "git@github.com:")
	} else if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
			return ""
		}
		value = parsed.Path
	}
	value = strings.TrimSuffix(strings.Trim(value, "/"), ".git")
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return strings.ToLower(parts[0] + "/" + parts[1])
}

// Stop removes an environment's stack and its routes. The project keeps its
// configuration, so the same environment can be deployed again unchanged.
func (s *Service) Stop(ctx context.Context, id, envID string) error {
	item, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	env, err := findEnv(item, envID)
	if err != nil {
		return err
	}
	return s.engine.Teardown(ctx, env.ID, StackName(item, env))
}

// Cancel stops a live run.
func (s *Service) Cancel(id string) error { return s.engine.Cancel(id) }

// Logs reads a run's output after a cursor.
func (s *Service) Logs(id string, after int) (LogChunk, error) { return s.engine.Logs(id, after) }

// Deployments lists runs newest first, optionally filtered to one project.
func (s *Service) Deployments(projectID string, limit int) ([]Deployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.repo.ListDeployments(projectID, limit)
}

func (s *Service) Deployment(id string) (Deployment, error) { return s.repo.GetDeployment(id) }

/* ---- live status ---- */

// Status reads one project's live state.
func (s *Service) Status(ctx context.Context, id string) (ProjectStatus, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return ProjectStatus{}, err
	}
	return s.statusOf(ctx, item, s.engine.Running())
}

// StatusAll reads every project's live state, for the card grid.
//
// One docker call per environment: that is a handful of local calls on the
// installations this serves, and the alternative — one call for every service on
// the host, then grouping — reads the state of stacks nobody asked about.
func (s *Service) StatusAll(ctx context.Context) ([]ProjectStatus, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	running := s.engine.Running()
	out := make([]ProjectStatus, 0, len(items))
	for _, item := range items {
		status, err := s.statusOf(ctx, item, running)
		if err != nil {
			return nil, err
		}
		out = append(out, status)
	}
	return out, nil
}

func (s *Service) statusOf(ctx context.Context, item Project, running map[string]bool) (ProjectStatus, error) {
	latest, err := s.repo.LatestByEnvironment(item.ID)
	if err != nil {
		return ProjectStatus{}, err
	}

	status := ProjectStatus{
		ProjectID:    item.ID,
		Environments: make([]EnvironmentStatus, 0, len(item.Environments)),
		CheckedAt:    timeNow(),
	}

	for _, env := range item.Environments {
		envStatus := s.status.Environment(ctx, env, StackName(item, env), running[env.ID])
		if deployment, ok := latest[env.ID]; ok {
			envStatus.LastDeployment = &deployment
			if status.LastDeployment == nil || deployment.StartedAt.After(status.LastDeployment.StartedAt) {
				status.LastDeployment = &deployment
			}
		}
		status.Environments = append(status.Environments, envStatus)
	}

	status.State = rollUp(status.Environments)
	return status, nil
}

/* ---- normalisation ---- */

// cleanVars drops entries with no key and trims the rest. A blank row is what an
// unused input in the UI's key/value editor sends.
func cleanVars(list []EnvVar) []EnvVar {
	out := make([]EnvVar, 0, len(list))
	for _, item := range list {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		out = append(out, EnvVar{Key: key, Value: item.Value})
	}
	return out
}

// mergeSecrets applies the keep-what-is-stored rule: a secret whose value comes
// back blank keeps the value already on record, because that is what a redacted
// response round-tripped. Clearing a secret is done by removing the row, which is
// unambiguous.
func mergeSecrets(incoming, stored []EnvVar) []EnvVar {
	previous := make(map[string]string, len(stored))
	for _, item := range stored {
		previous[item.Key] = item.Value
	}

	out := make([]EnvVar, 0, len(incoming))
	for _, item := range incoming {
		if item.Value == "" {
			item.Value = previous[item.Key]
		}
		out = append(out, item)
	}
	return out
}

func cleanTrigger(trigger DeployTrigger) DeployTrigger {
	switch trigger.Kind {
	case TriggerPush, TriggerTag, TriggerSchedule:
	default:
		trigger.Kind = TriggerManual
	}
	trigger.Pattern = strings.TrimSpace(trigger.Pattern)
	return trigger
}

// cleanDeploy fills in the defaults a payload may leave at zero, so a request
// from a client that omits the block still deploys sensibly.
func cleanDeploy(settings DeploySettings) (DeploySettings, error) {
	if settings.Strategy != StrategyRecreate {
		settings.Strategy = StrategyRolling
	}
	if settings.Replicas == 0 {
		settings.Replicas = 1
	}
	if settings.Replicas < 1 || settings.Replicas > 100 {
		return DeploySettings{}, ErrReplicas
	}
	// Zero is treated as unset rather than as "watch for no time at all": a
	// payload that omits the block still gets swarm a window to judge a new task
	// in, which is what auto-rollback depends on.
	if settings.HealthGraceSec <= 0 || settings.HealthGraceSec > 3600 {
		settings.HealthGraceSec = 30
	}
	settings.Placement = strings.TrimSpace(settings.Placement)
	return settings, nil
}

func cleanDomains(list []DomainRequest) ([]Domain, error) {
	out := make([]Domain, 0, len(list))
	for _, req := range list {
		host := strings.ToLower(strings.TrimSpace(req.Host))
		service := strings.TrimSpace(req.Service)

		// A wholly blank row is the UI's empty domain editor, not an error.
		if host == "" && service == "" {
			continue
		}
		if !validHost(host) {
			return nil, fmt.Errorf("%w: %s", ErrDomainHost, req.Host)
		}
		if !serviceName.MatchString(service) {
			return nil, fmt.Errorf("%w: %s", ErrDomainService, host)
		}

		port := req.Port
		if port == 0 {
			port = 80
		}
		if port < 1 || port > 65535 {
			return nil, ErrDomainPort
		}

		tls := req.TLS
		switch tls {
		case TLSAuto, TLSCustom, TLSNone:
		default:
			tls = TLSAuto
		}

		id := strings.TrimSpace(req.ID)
		if id == "" {
			id = newID()
		}

		out = append(out, Domain{
			ID:          id,
			Host:        host,
			Service:     service,
			Port:        port,
			TLS:         tls,
			RedirectWww: req.RedirectWww,
		})
	}
	return out, nil
}

func findEnv(item Project, envID string) (Environment, error) {
	for _, env := range item.Environments {
		if env.ID == envID {
			return env, nil
		}
	}
	return Environment{}, ErrEnvNotFound
}

// errIsValidation reports whether an error is the user's to fix, which is what
// separates a 400 from a 500 in the handlers.
func errIsValidation(err error) bool {
	for _, candidate := range []error{
		ErrInvalidName, ErrInvalidSource, ErrRepoRequired, ErrBranchRequired,
		ErrComposePath, ErrComposeRequired, ErrEnvRequired, ErrEnvName,
		ErrDomainHost, ErrDomainService, ErrDomainPort, ErrReplicas,
		ErrComposeInvalid, ErrNoServices, ErrUnknownService,
	} {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}
