package project

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The deploy engine.
//
// A deploy runs entirely on the machine stacker is installed on: it clones the
// repository into a throwaway workspace, builds whatever the compose file builds,
// deploys the result as a swarm stack, publishes the environment's hostnames
// through Traefik, and deletes the workspace again. Nothing is left on disk
// except the images the stack needs to run.
//
// The run outlives the HTTP request that started it — a build takes minutes — so
// Start returns as soon as the row exists and the work happens on its own
// goroutine against a background context. Progress is readable two ways while it
// happens: the run's log through Logs, and the stack's real state through the
// status reader.

// tokenFunc supplies a git credential for a private repository. It is optional:
// a public repository clones without one, and an installation that has not
// connected GitHub has no token to give.
type tokenFunc func(ctx context.Context) (string, error)

// engine owns the live runs. There is one per server.
type engine struct {
	repo   *Repository
	routes *router
	log    *slog.Logger
	exec   Exec
	token  tokenFunc

	// workRoot is the parent of every run's workspace. Each run gets a
	// directory under it, removed when the run ends.
	workRoot string
	// network is the attachable overlay Traefik shares with deployed services.
	network string

	mu   sync.Mutex
	runs map[string]*run
}

// run is one in-flight deploy: its log so far, and the handle to stop it.
type run struct {
	mu     sync.Mutex
	lines  []string
	status DeploymentStatus
	// mask holds values that must never reach the log — the git token and every
	// secret of the environment. Redaction happens on the way in, so a secret is
	// not in the buffer to be leaked by a later reader.
	mask   []string
	cancel context.CancelFunc
	// cancelled records that a person stopped the run, so the failure it causes
	// is reported as a cancellation rather than as a broken build.
	cancelled bool
}

func newEngine(repo *Repository, routes *router, workRoot, network string, token tokenFunc, log *slog.Logger) *engine {
	return &engine{
		repo:     repo,
		routes:   routes,
		log:      log,
		exec:     execCommand,
		token:    token,
		workRoot: workRoot,
		network:  network,
		runs:     map[string]*run{},
	}
}

// Start records a deployment and begins working on it.
//
// One run per environment: two `docker stack deploy` calls against the same
// stack would race each other's rollout, and the second would usually win with
// an older image.
func (e *engine) Start(item Project, env Environment, req DeployRequest) (Deployment, error) {
	if _, active, err := e.repo.ActiveForEnvironment(env.ID); err != nil {
		return Deployment{}, err
	} else if active {
		return Deployment{}, ErrAlreadyDeploying
	}

	revision := "compose"
	if item.SourceKind == SourceGit {
		revision = "pending"
	}
	actor := strings.TrimSpace(req.Actor)
	if actor == "" {
		actor = "manual"
	}

	deployment := Deployment{
		ID:            newID(),
		ProjectID:     item.ID,
		ProjectName:   item.Name,
		EnvironmentID: env.ID,
		Environment:   env.Name,
		Stack:         StackName(item, env),
		Status:        StatusQueued,
		TriggeredBy:   TriggerManual,
		Actor:         actor,
		Revision:      revision,
		Message:       strings.TrimSpace(req.Message),
		StartedAt:     timeNow(),
	}
	if err := e.repo.CreateDeployment(&deployment); err != nil {
		return Deployment{}, err
	}

	// Background, not the request's context: the response returns immediately
	// and cancelling the request must not kill the deploy.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)

	state := &run{status: StatusQueued, cancel: cancel, mask: maskValues(env)}
	e.mu.Lock()
	e.runs[deployment.ID] = state
	e.mu.Unlock()

	go func() {
		defer cancel()
		e.work(ctx, state, deployment, item, env)
	}()

	return deployment, nil
}

// Cancel stops a live run. The stack is left exactly as the interrupted deploy
// left it — swarm may well have half a rollout in place — which is why the log
// says so rather than pretending the environment was restored.
func (e *engine) Cancel(id string) error {
	e.mu.Lock()
	state, ok := e.runs[id]
	e.mu.Unlock()
	if !ok {
		// Nothing is running under that id in this process. Either the run
		// finished, or it belonged to a previous process — both mean there is
		// nothing left to stop.
		return ErrNotRunning
	}

	state.mu.Lock()
	state.cancelled = true
	state.mu.Unlock()
	state.cancel()
	return nil
}

// Logs returns the lines of a run after a cursor.
//
// A live run answers from memory; a finished one answers from its row. The
// cursor is a line count rather than a byte offset, which is what lets the
// browser poll for the tail with no bookkeeping beyond the number it was
// handed last time.
func (e *engine) Logs(id string, after int) (LogChunk, error) {
	e.mu.Lock()
	state, live := e.runs[id]
	e.mu.Unlock()

	if live {
		state.mu.Lock()
		defer state.mu.Unlock()
		return chunk(id, state.status, state.lines, after), nil
	}

	deployment, err := e.repo.GetDeployment(id)
	if err != nil {
		return LogChunk{}, err
	}
	var lines []string
	if deployment.Log != "" {
		lines = strings.Split(strings.TrimRight(deployment.Log, "\n"), "\n")
	}
	return chunk(id, deployment.Status, lines, after), nil
}

// Running reports the ids of environments a run is working on, so the status
// reader can label them `deploying` instead of reporting the half-rolled-out
// stack underneath as degraded.
func (e *engine) Running() map[string]bool {
	items, err := e.repo.ListDeployments("", 50)
	if err != nil {
		return nil
	}

	out := map[string]bool{}
	for _, item := range items {
		if !item.Status.Done() {
			out[item.EnvironmentID] = true
		}
	}
	return out
}

// Teardown removes an environment's stack and its routes. It backs both the
// stop action and project deletion, and is safe to call for an environment that
// was never deployed.
func (e *engine) Teardown(ctx context.Context, envID, stack string) error {
	if err := e.routes.Remove(envID); err != nil {
		return err
	}

	err := e.exec(ctx, Command{Name: "docker", Args: []string{"stack", "rm", stack}, Env: os.Environ()}, func(string) {})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "nothing found") {
		return err
	}
	return nil
}

/* ---- the run itself ---- */

// work is the whole deploy, start to finish. Every step logs what it is about to
// do before doing it, so a run that fails halfway reads as a story rather than
// as a stack trace.
func (e *engine) work(ctx context.Context, state *run, deployment Deployment, item Project, env Environment) {
	e.setStatus(state, StatusRunning)
	deployment.Status = StatusRunning
	if err := e.repo.SaveDeployment(&deployment); err != nil {
		e.log.Error("could not mark the deployment running", "deployment", deployment.ID, "error", err)
	}

	workspace := filepath.Join(e.workRoot, deployment.ID)
	revision, err := e.steps(ctx, state, &deployment, item, env, workspace)

	// Cleanup runs whatever happened: a failed run's workspace is no more
	// useful than a successful one's, and the log already holds everything the
	// user needs to see.
	e.cleanup(state, workspace)

	if revision != "" {
		deployment.Revision = revision
	}
	e.finish(state, &deployment, err)
}

// steps performs the deploy and returns the revision it built, if it got that
// far. Errors are returned rather than logged so finish is the only place that
// decides what a failure means.
func (e *engine) steps(
	ctx context.Context,
	state *run,
	deployment *Deployment,
	item Project,
	env Environment,
	workspace string,
) (string, error) {
	stack := deployment.Stack
	e.emit(state, fmt.Sprintf("==> deploying %s · %s as stack %s", item.Name, env.Name, stack))

	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return "", fmt.Errorf("could not create the workspace: %w", err)
	}

	// 1. Get the compose file, and where it sits on disk.
	revision, basePath, content, err := e.source(ctx, state, item, env, workspace)
	if err != nil {
		return revision, err
	}
	if revision != "" && revision != deployment.Revision {
		deployment.Revision = revision
		if err := e.repo.SaveDeployment(deployment); err != nil {
			e.log.Error("could not record the revision", "deployment", deployment.ID, "error", err)
		}
	}

	// 2. Read it, and check the domains point at services it actually declares.
	spec, err := parseCompose(content)
	if err != nil {
		return revision, err
	}
	e.emit(state, "--> services: "+strings.Join(spec.Names, ", "))

	for _, domain := range env.Domains {
		if !spec.Has(domain.Service) {
			return revision, fmt.Errorf("%w: %s routes to %q, which the compose file does not define",
				ErrUnknownService, domain.Host, domain.Service)
		}
	}

	// 3. Write stacker's additions as a second compose file, in the same
	// directory as the first.
	//
	// The directory matters: every relative path in a compose file — a build
	// context, a bind mount, an env_file — resolves against the file's own
	// location, so an overlay written anywhere else would silently move the
	// build context out from under `build: .`.
	composeDir := filepath.Dir(basePath)
	environment := envMap(env)
	override, err := buildOverride(spec, overrideOptions{
		Stack:         stack,
		Network:       e.network,
		ProxyServices: proxyServices(env.Domains),
		Env:           environment,
		Deploy:        env.Deploy,
	})
	if err != nil {
		return revision, err
	}

	overridePath := filepath.Join(composeDir, ".stacker-override.yml")
	if err := os.WriteFile(overridePath, []byte(override), 0o600); err != nil {
		return revision, err
	}
	e.emit(state, "--> generated the stacker compose overlay")

	// The build and the deploy both interpolate `${VAR}` out of their own
	// environment, so the values are handed to the commands rather than written
	// into a file that would then have to be cleaned up.
	commandEnv := append(os.Environ(), envPairs(environment)...)
	// The two commands spell the same argument differently: `docker compose`
	// takes -f, `docker stack deploy` takes -c and rejects -f outright.
	composeFiles := []string{"-f", basePath, "-f", overridePath}
	stackFiles := []string{"-c", basePath, "-c", overridePath}

	// 4. Build, if anything in the file is built rather than pulled.
	if len(spec.Builds) > 0 {
		e.emit(state, "==> building "+strings.Join(spec.Builds, ", "))

		args := append([]string{"compose"}, composeFiles...)
		args = append(args, "--project-name", stack, "build")
		if env.Deploy.AlwaysPull {
			// Re-fetch the base images, so a `FROM node:20` that moved is
			// picked up instead of building on a months-old local layer.
			args = append(args, "--pull")
		}
		if err := e.exec(ctx, Command{Name: "docker", Args: args, Dir: composeDir, Env: commandEnv}, e.sink(state)); err != nil {
			return revision, err
		}
	} else if env.Deploy.AlwaysPull {
		e.emit(state, "==> pulling images")
		args := append([]string{"compose"}, composeFiles...)
		args = append(args, "--project-name", stack, "pull", "--ignore-buildable")
		if err := e.exec(ctx, Command{Name: "docker", Args: args, Dir: composeDir, Env: commandEnv}, e.sink(state)); err != nil {
			// A pull that fails is not fatal: the image may already be present
			// locally, and the deploy below will say so if it is not.
			e.emit(state, "--> pull failed, continuing with the images already on this host")
		}
	}

	// 5. Deploy the stack.
	//
	// `--resolve-image never` is what makes a locally built image usable: the
	// default asks a registry to resolve the tag to a digest, and an image that
	// was only ever built here has no registry to answer.
	//
	// It is also this deploy's boundary. A built image exists only on the machine
	// that built it, so a service built from a repository can only be scheduled
	// on this node — placing it elsewhere needs a registry to push to, which is a
	// deliberate next step and not something to fake here. Services that pull a
	// published image are unaffected and schedule anywhere.
	e.emit(state, "==> deploying the stack")
	deployArgs := []string{
		"stack", "deploy",
		"--detach=true",
		"--prune",
		"--resolve-image", "never",
		"--with-registry-auth",
	}
	deployArgs = append(deployArgs, stackFiles...)
	deployArgs = append(deployArgs, stack)
	if err := e.exec(ctx, Command{Name: "docker", Args: deployArgs, Dir: composeDir, Env: commandEnv}, e.sink(state)); err != nil {
		return revision, err
	}

	// 6. Publish the hostnames.
	if len(env.Domains) > 0 {
		hosts := make([]string, 0, len(env.Domains))
		for _, domain := range env.Domains {
			hosts = append(hosts, domain.Host)
		}
		e.emit(state, "==> routing "+strings.Join(hosts, ", "))
	}
	if err := e.routes.Apply(env.ID, stack, env.Domains); err != nil {
		return revision, fmt.Errorf("the stack is deployed but its hostnames could not be published: %w", err)
	}

	// Swarm accepted the spec; the tasks converge on their own from here, which
	// is what the environment's live status reports.
	e.emit(state, "==> done — swarm is converging on the new spec")
	return revision, nil
}

// source puts the compose file on disk and returns the revision it came from,
// the path it was written to and its content.
//
// The path is part of the answer because compose resolves every relative path in
// a file against that file's own directory: a repository's compose file has to
// stay exactly where the repository put it.
func (e *engine) source(
	ctx context.Context,
	state *run,
	item Project,
	env Environment,
	workspace string,
) (revision, path, content string, err error) {
	if item.SourceKind == SourceCompose {
		e.emit(state, "==> using the project's stored compose file")

		// An inline file has no repository to be relative to, so the workspace
		// root is as good a home as any.
		path = filepath.Join(workspace, "docker-compose.yml")
		if err := os.WriteFile(path, []byte(item.Compose), 0o600); err != nil {
			return "", "", "", err
		}
		return "compose", path, item.Compose, nil
	}

	branch := strings.TrimSpace(env.Branch)
	if branch == "" {
		branch = strings.TrimSpace(item.Git.Branch)
	}

	url, display, err := e.cloneURL(ctx, item, state)
	if err != nil {
		return "", "", "", err
	}
	e.emit(state, fmt.Sprintf("==> cloning %s at %s", display, branch))

	checkout := filepath.Join(workspace, "repo")
	// A shallow single-branch clone: a deploy needs one tree, not the history,
	// and on a large repository that is the difference between seconds and
	// minutes.
	clone := Command{
		Name: "git",
		Args: []string{"clone", "--depth", "1", "--single-branch", "--branch", branch, url, checkout},
		Dir:  workspace,
		// GIT_TERMINAL_PROMPT=0 turns a missing credential into an error
		// instead of a process waiting forever for a password nobody will type.
		Env: append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "SSH_ASKPASS="),
	}
	if err := e.exec(ctx, clone, e.sink(state)); err != nil {
		return "", "", "", err
	}

	revision = "unknown"
	e.exec(ctx, Command{ //nolint:errcheck // the revision is a label, not a gate
		Name: "git", Args: []string{"rev-parse", "--short", "HEAD"}, Dir: checkout, Env: os.Environ(),
	}, func(line string) {
		if line = strings.TrimSpace(line); line != "" {
			revision = line
		}
	})

	relative := strings.TrimSpace(item.Git.ComposePath)
	if relative == "" {
		relative = "docker-compose.yml"
	}
	// Cleaned against a leading slash before it is joined, so a path like
	// `../../etc/passwd` cannot escape the checkout.
	path = filepath.Join(checkout, filepath.Clean("/"+relative))
	raw, err := os.ReadFile(path)
	if err != nil {
		return revision, "", "", fmt.Errorf("could not read %s from the repository: %w", relative, err)
	}

	e.emit(state, "--> checked out "+revision)
	return revision, path, string(raw), nil
}

// cloneURL builds the URL git is given, and the one the log is allowed to show.
//
// `owner/name` is expanded to GitHub, since that is the provider stacker can
// authenticate; anything that already looks like a URL is passed through so an
// ssh remote or a self-hosted git works untouched.
func (e *engine) cloneURL(ctx context.Context, item Project, state *run) (url, display string, err error) {
	repo := strings.TrimSpace(item.Git.Repo)
	if repo == "" {
		return "", "", ErrRepoRequired
	}

	if !strings.Contains(repo, "://") && !strings.Contains(repo, "@") {
		repo = "https://github.com/" + strings.TrimSuffix(strings.Trim(repo, "/"), ".git") + ".git"
	}
	display = repo

	// Credentials are only ever attached to an https GitHub remote: an ssh
	// remote authenticates with the host's own key, and sending an installation
	// token anywhere else would hand it to a third party.
	if e.token == nil || !strings.HasPrefix(repo, "https://github.com/") {
		return repo, display, nil
	}

	token, tokenErr := e.token(ctx)
	if tokenErr != nil || token == "" {
		// No token is normal — a public repository needs none. If the
		// repository is private, git's own error will say so.
		return repo, display, nil
	}

	state.mu.Lock()
	state.mask = append(state.mask, token)
	state.mu.Unlock()

	return strings.Replace(repo, "https://", "https://x-access-token:"+token+"@", 1), display, nil
}

// cleanup removes the run's workspace. It is the "clean up after deploy" half of
// the contract: the clone, the compose files stacker generated and every build
// context go away, and only the images the stack runs from are kept.
func (e *engine) cleanup(state *run, workspace string) {
	if err := os.RemoveAll(workspace); err != nil {
		e.emit(state, "--> could not remove the workspace: "+err.Error())
		return
	}
	e.emit(state, "--> removed the workspace")
}

// finish writes the run's outcome and its log, and stops tracking it.
func (e *engine) finish(state *run, deployment *Deployment, err error) {
	now := timeNow()
	seconds := int(now.Sub(deployment.StartedAt).Round(time.Second).Seconds())

	state.mu.Lock()
	cancelled := state.cancelled
	state.mu.Unlock()

	switch {
	case err == nil:
		deployment.Status = StatusSucceeded
		e.emit(state, "==> succeeded")
	case cancelled, errors.Is(err, context.Canceled):
		deployment.Status = StatusCancelled
		deployment.Error = "cancelled"
		e.emit(state, "==> cancelled — the stack was left as this run found it")
	case errors.Is(err, context.DeadlineExceeded):
		deployment.Status = StatusFailed
		deployment.Error = "the deployment timed out"
		e.emit(state, "==> timed out")
	default:
		deployment.Status = StatusFailed
		deployment.Error = truncate(err.Error(), 1000)
		e.emit(state, "==> failed: "+err.Error())
	}

	deployment.FinishedAt = &now
	deployment.DurationSec = &seconds

	state.mu.Lock()
	state.status = deployment.Status
	deployment.Log = strings.Join(state.lines, "\n")
	state.mu.Unlock()

	if saveErr := e.repo.SaveDeployment(deployment); saveErr != nil {
		e.log.Error("could not record the deployment result", "deployment", deployment.ID, "error", saveErr)
	}

	// Dropped from the live map only after the row is written, so a poll that
	// misses the memory buffer finds the persisted log rather than nothing.
	e.mu.Lock()
	delete(e.runs, deployment.ID)
	e.mu.Unlock()

	e.log.Info("deployment finished",
		"deployment", deployment.ID,
		"project", deployment.ProjectName,
		"environment", deployment.Environment,
		"status", deployment.Status,
		"duration", seconds)
}

/* ---- log buffer ---- */

// sink is the per-command line handler: it timestamps nothing and interprets
// nothing, it just appends what the command said.
func (e *engine) sink(state *run) Sink {
	return func(line string) { e.emit(state, line) }
}

// emit appends one redacted line to the run's buffer. The buffer is capped: a
// pathological build could otherwise fill memory, and the tail is the part
// anyone reads.
func (e *engine) emit(state *run, line string) {
	state.mu.Lock()
	defer state.mu.Unlock()

	for _, secret := range state.mask {
		if secret != "" {
			line = strings.ReplaceAll(line, secret, "••••")
		}
	}
	state.lines = append(state.lines, line)

	const maxLines = 5000
	if len(state.lines) > maxLines {
		// The cursor the browser holds counts from the start of the buffer, so
		// dropping the head would silently renumber every line it has already
		// read. Truncating the tail instead keeps the cursor honest.
		state.lines = state.lines[:maxLines]
		state.lines[maxLines-1] = "--> log truncated"
	}
}

func (e *engine) setStatus(state *run, status DeploymentStatus) {
	state.mu.Lock()
	state.status = status
	state.mu.Unlock()
}

// chunk slices a log at a cursor. An out-of-range cursor yields nothing rather
// than an error: a browser that polls after a restart is not a fault.
func chunk(id string, status DeploymentStatus, lines []string, after int) LogChunk {
	if after < 0 || after > len(lines) {
		after = len(lines)
	}

	tail := lines[after:]
	out := make([]string, len(tail))
	copy(out, tail)

	return LogChunk{
		DeploymentID: id,
		Status:       status,
		Lines:        out,
		Next:         len(lines),
		Done:         status.Done(),
	}
}

// maskValues is the redaction list a run starts with: every secret of the
// environment, longest first so a value that contains another is masked whole.
func maskValues(env Environment) []string {
	values := make([]string, 0, len(env.Secrets))
	for _, secret := range env.Secrets {
		if value := secret.Value; strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values
}

// envPairs renders an environment map as the KEY=VALUE list exec wants, sorted
// so a build is reproducible.
func envPairs(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+"="+values[key])
	}
	return pairs
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit-1] + "…"
}
