package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	cache  statusCache
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
	s.sweepWorkspaces()
	return s.repo.ResetRunning("the server restarted while this deployment was running")
}

// sweepWorkspaces clears what the work root should not be holding after a
// restart, and only that.
//
// An environment's workspace is not abandoned just because no run is in flight:
// it is the directory its deployed stack resolves relative bind mounts against,
// and swarm re-reads those paths on every task it schedules. Deleting them here
// would break every running stack that binds a file out of its own repository.
// What can go is staging directories, whose runs died with the old process, and
// workspaces belonging to environments that no longer exist.
func (s *Service) sweepWorkspaces() {
	if err := os.RemoveAll(filepath.Join(s.engine.workRoot, stagingName)); err != nil {
		s.log.Warn("could not clear the staging directory", "error", err)
	}

	entries, err := os.ReadDir(s.engine.workRoot)
	if err != nil {
		return
	}

	known, err := s.knownEnvironments()
	if err != nil {
		// Without the environment list there is no way to tell a live workspace
		// from a stale one, and keeping a stale directory costs disk while
		// deleting a live one breaks a running stack.
		s.log.Warn("could not list environments, leaving the workspaces alone", "error", err)
		return
	}

	for _, entry := range entries {
		name := entry.Name()
		envID, ok := strings.CutPrefix(name, "env-")
		if !ok || known[envID] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.engine.workRoot, name)); err != nil {
			s.log.Warn("could not remove an abandoned workspace", "name", name, "error", err)
		}
	}
}

// knownEnvironments is the id set of every environment stacker still has a
// record of.
func (s *Service) knownEnvironments() (map[string]bool, error) {
	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, item := range items {
		for _, env := range item.Environments {
			known[env.ID] = true
		}
	}
	return known, nil
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

// PreviewCompose reads a selected repository's Compose file in an ephemeral
// checkout. The browser needs its service names before it can configure a host,
// but no project or deployment should be created merely by opening that list.
func (s *Service) PreviewCompose(ctx context.Context, git GitSource) (string, error) {
	git.Repo = strings.TrimSpace(git.Repo)
	git.Branch = strings.TrimSpace(git.Branch)
	git.ComposePath = strings.TrimSpace(git.ComposePath)
	if git.Repo == "" {
		return "", ErrRepoRequired
	}
	if git.Branch == "" {
		return "", ErrBranchRequired
	}
	if git.ComposePath == "" {
		return "", ErrComposePath
	}
	if err := os.MkdirAll(s.engine.workRoot, 0o700); err != nil {
		return "", err
	}

	workspace, err := os.MkdirTemp(s.engine.workRoot, ".preview-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workspace) //nolint:errcheck // it is an isolated preview checkout

	item := Project{SourceKind: SourceGit, Git: git}
	_, _, compose, err := s.engine.source(ctx, &run{}, item, Environment{}, workspace, "")
	if err != nil {
		return "", err
	}
	if _, err := parseCompose(compose); err != nil {
		return "", err
	}
	return compose, nil
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
		env, err := s.buildEnvironment(item.ID, item.SourceKind, position, envReq, stored)
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
	source SourceKind,
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
	trigger, err := cleanTrigger(req.Trigger, source)
	if err != nil {
		return Environment{}, err
	}
	env.Trigger = trigger

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

	owners, err := s.repo.HostOwnerMap()
	if err != nil {
		return err
	}

	claimed := map[string]bool{}
	for _, env := range item.Environments {
		for _, domain := range env.Domains {
			if claimed[domain.Host] {
				return fmt.Errorf("%w: %s", ErrDomainTaken, domain.Host)
			}
			claimed[domain.Host] = true

			owner := owners[domain.Host]
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
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return nil, nil
	}

	return s.handleWebhook(repository, func(item Project, env Environment) (bool, string) {
		if env.Trigger.Kind != TriggerPush {
			return false, ""
		}
		return effectiveBranch(item, env) == branch, ""
	}, TriggerPush, actor, revision, message)
}

// HandleTag queues every tag-enabled environment of the repository whose
// pattern matches the tag that was pushed.
//
// Unlike a push, the branch is irrelevant: the tag names the exact commit to
// deploy, and it is passed to the run as the ref to clone.
func (s *Service) HandleTag(repository, tag, actor, revision, message string) ([]Deployment, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, nil
	}

	return s.handleWebhook(repository, func(_ Project, env Environment) (bool, string) {
		if env.Trigger.Kind != TriggerTag {
			return false, ""
		}
		pattern := env.Trigger.Pattern
		if pattern == "" {
			pattern = "*"
		}
		// The pattern was validated on save, so a bad one here can only be a
		// row that predates the rule; it simply matches nothing.
		matched, err := path.Match(pattern, tag)
		if err != nil || !matched {
			return false, ""
		}
		return true, "refs/tags/" + tag
	}, TriggerTag, actor, revision, message)
}

// handleWebhook is the half a push and a tag share: find the repository's
// projects, ask `wants` about each environment, and start the ones that say
// yes. `wants` returns the git ref the run should check out, blank for the
// environment's own branch.
func (s *Service) handleWebhook(
	repository string,
	wants func(Project, Environment) (bool, string),
	kind TriggerKind,
	actor, revision, message string,
) ([]Deployment, error) {
	repository = canonicalGitHubRepo(repository)
	if repository == "" {
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
			match, ref := wants(item, env)
			if !match {
				continue
			}

			deployment, deployErr := s.engine.Start(item, env, DeployRequest{
				Actor: actor, Message: message, TriggeredBy: kind, Revision: revision, Ref: ref,
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

// effectiveBranch is the environment's branch override, or the project's.
func effectiveBranch(item Project, env Environment) string {
	if branch := strings.TrimSpace(env.Branch); branch != "" {
		return branch
	}
	return strings.TrimSpace(item.Git.Branch)
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

// ServiceLogs reads the current tail of one compose service's container
// output, via `docker service logs` on the swarm service the stack runs it as.
func (s *Service) ServiceLogs(ctx context.Context, id, envID, service string, tail int) (ServiceLogChunk, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return ServiceLogChunk{}, err
	}
	env, err := findEnv(item, envID)
	if err != nil {
		return ServiceLogChunk{}, err
	}
	if !serviceName.MatchString(service) {
		return ServiceLogChunk{}, ErrServiceNotFound
	}
	if tail <= 0 || tail > 2000 {
		tail = 300
	}

	stack := StackName(item, env)
	swarmService := stack + "_" + service

	var lines []string
	cmd := Command{
		Name: "docker",
		Args: []string{"service", "logs", "--tail", strconv.Itoa(tail), "--timestamps", "--no-task-ids", swarmService},
		Env:  os.Environ(),
	}
	if err := execCommand(ctx, cmd, func(line string) { lines = append(lines, line) }); err != nil {
		if rows, statusErr := s.status.services(ctx, stack); statusErr == nil {
			found := false
			for _, row := range rows {
				if strings.TrimPrefix(row.Name, stack+"_") == service {
					found = true
					break
				}
			}
			if !found {
				return ServiceLogChunk{}, ErrServiceNotFound
			}
		}
		return ServiceLogChunk{}, err
	}

	return ServiceLogChunk{Service: service, Lines: lines, FetchedAt: timeNow()}, nil
}

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

	latest, err := s.repo.LatestByEnvironment(item.ID)
	if err != nil {
		return ProjectStatus{}, err
	}

	services, servicesErr := s.status.allServices(ctx)

	return s.statusOf(item, s.engine.Running(), latest, services, servicesErr, timeNow())
}

// StatusAll reads every project's live state, for the card grid.
//
// Results are cached for a few seconds and docker is queried once per poll,
// not once per environment.
func (s *Service) StatusAll(ctx context.Context) ([]ProjectStatus, error) {
	now := timeNow()
	if cached, ok := s.cache.get(now); ok {
		return cached, nil
	}

	items, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	projectIDs := make([]string, len(items))
	for i, item := range items {
		projectIDs[i] = item.ID
	}

	latestAll, err := s.repo.LatestForProjects(projectIDs)
	if err != nil {
		return nil, err
	}

	services, servicesErr := s.status.allServices(ctx)
	if servicesErr != nil && len(items) == 0 {
		return nil, servicesErr
	}

	running := s.engine.Running()
	out := make([]ProjectStatus, 0, len(items))
	for _, item := range items {
		status, err := s.statusOf(item, running, latestAll[item.ID], services, servicesErr, now)
		if err != nil {
			return nil, err
		}
		out = append(out, status)
	}

	s.cache.set(now, out)
	return out, nil
}

func (s *Service) statusOf(item Project, running map[string]bool, latest map[string]Deployment, services map[string][]dockerServiceRow, dockerErr error, checkedAt time.Time) (ProjectStatus, error) {
	status := ProjectStatus{
		ProjectID:    item.ID,
		Environments: make([]EnvironmentStatus, 0, len(item.Environments)),
		CheckedAt:    checkedAt,
	}

	for _, env := range item.Environments {
		stack := StackName(item, env)
		var envStatus EnvironmentStatus
		if dockerErr != nil {
			envStatus = EnvironmentStatus{
				EnvironmentID: env.ID,
				Name:          env.Name,
				Stack:         stack,
				Services:      []ServiceState{},
				Domains:       hosts(env.Domains),
				State:         RuntimeUnknown,
				Message:       dockerErr.Error(),
			}
		} else {
			envStatus = s.status.environmentFromRows(env, stack, services[stack], running[env.ID])
		}
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

// cleanTrigger validates the trigger against the source it will run from. The
// pattern is checked here rather than at deploy time so a schedule that can
// never fire is refused while the user is still looking at the form.
func cleanTrigger(trigger DeployTrigger, source SourceKind) (DeployTrigger, error) {
	trigger.Pattern = strings.TrimSpace(trigger.Pattern)

	switch trigger.Kind {
	case TriggerPush, TriggerTag:
		// Both are webhook-driven, and there is no webhook for a compose file
		// pasted into stacker.
		if source != SourceGit {
			return DeployTrigger{}, ErrTriggerSource
		}
	case TriggerSchedule:
	default:
		trigger.Kind = TriggerManual
	}

	switch trigger.Kind {
	case TriggerTag:
		if trigger.Pattern == "" {
			// Every tag, which is the useful default for a repository that
			// only tags releases.
			trigger.Pattern = "*"
		}
		if _, err := path.Match(trigger.Pattern, "v1.0.0"); err != nil {
			return DeployTrigger{}, fmt.Errorf("%w: %s", ErrTagPattern, trigger.Pattern)
		}
	case TriggerSchedule:
		if _, err := parseCron(trigger.Pattern); err != nil {
			return DeployTrigger{}, err
		}
	default:
		// A pattern means nothing to a manual or push trigger, and keeping a
		// stale one would resurface it if the kind were switched back.
		trigger.Pattern = ""
	}

	return trigger, nil
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

		// Anything that is not an explicit opt-out of TLS is served with an
		// issued certificate, which also folds the retired `custom` mode of
		// stored domains back onto `auto`.
		tls := TLSAuto
		if req.TLS == TLSNone {
			tls = TLSNone
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
		ErrTagPattern, ErrCronExpression, ErrTriggerSource,
		ErrComposeInvalid, ErrNoServices, ErrUnknownService,
	} {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}
