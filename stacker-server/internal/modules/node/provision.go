package node

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Provisioning timeouts. Probes answer in seconds; the docker install pulls
// packages over the network and legitimately takes minutes on a slow host.
// The install budget is deliberately generous: get.docker.com adds a package
// repository and pulls the engine, the CLI, containerd and buildx, which on a
// small VPS or a slow link runs well past ten minutes. Cutting it short leaves
// dpkg mid-transaction, which is worse than waiting.
const (
	probeTimeout        = 30 * time.Second
	installTimeout      = 25 * time.Minute
	serviceTimeout      = 2 * time.Minute
	provisionMaxRuntime = 40 * time.Minute
)

// dockerInstallScript is docker's official convenience installer. It is the
// documented one-command install and the one this feature is specified around;
// it is shown to the user before it runs, so what executes is what they saw.
const dockerInstallScript = "curl -fsSL https://get.docker.com | sh"

// recoverPackages runs before the installer on dpkg systems. A run that was
// interrupted — a timeout, a dropped ssh session — can leave the package
// database locked or a package half-configured, and the installer would then
// fail on something that has nothing to do with docker. This waits for any
// other package job to finish, then completes whatever was left dangling.
// Both commands are best effort: on a healthy host they do nothing, and on a
// non-dpkg host they are simply absent.
const recoverPackages = "command -v dpkg >/dev/null 2>&1 && { " +
	"for i in $(seq 1 60); do fuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 || break; sleep 5; done; " +
	"dpkg --configure -a >/dev/null 2>&1; }; "

// StepState is where one checklist step stands.
type StepState string

const (
	StepPending StepState = "pending"
	StepRunning StepState = "running"
	StepDone    StepState = "done"
	// StepSkipped is a step that was already satisfied — docker present, the
	// daemon already up. It is the normal outcome of re-running configure.
	StepSkipped StepState = "skipped"
	StepFailed  StepState = "failed"
	// StepWarned is a check that did not pass but is not fatal, so the run
	// continues and the user still gets told.
	StepWarned StepState = "warned"
)

// ProvisionState is the state of a whole run.
type ProvisionState string

const (
	ProvisionRunning   ProvisionState = "running"
	ProvisionSucceeded ProvisionState = "succeeded"
	ProvisionFailed    ProvisionState = "failed"
)

// StepResult is one line of the checklist as the UI renders it.
type StepResult struct {
	Key   string    `json:"key"`
	Title string    `json:"title"`
	State StepState `json:"state"`
	// Detail is what happened, in the step's own words — a version string, a
	// distro name, or the error that stopped it.
	Detail    string     `json:"detail"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// ProvisionJob is one configure run: the checklist plus its overall outcome.
// It is held in memory only — a run that a restart interrupts is simply re-run,
// since every step re-checks before it acts.
type ProvisionJob struct {
	NodeID     string         `json:"nodeId"`
	State      ProvisionState `json:"state"`
	Steps      []StepResult   `json:"steps"`
	Message    string         `json:"message"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

// provisionCtx carries what one step learns to the steps after it.
type provisionCtx struct {
	node Node
	// sudo is the prefix privileged commands need: empty for root, `sudo -n`
	// for a user with passwordless sudo.
	sudo string
	// distro is the pretty name from /etc/os-release, for the UI.
	distro string
	// skipInstall is set when the host is not Linux but already has a working
	// docker — a mac running Docker Desktop, typically. The install script
	// supports Linux only, so those steps stand down and the swarm step still
	// runs.
	skipInstall bool
}

// step is one entry in the checklist. run reports what it did; returning
// StepSkipped means the work was already done.
type step struct {
	key   string
	title string
	run   func(ctx context.Context, p *provisionCtx) (StepState, string, error)
}

// steps is the whole provisioning checklist, in order. Each one checks before
// it acts, so running configure twice is safe and the second run is all skips.
func (s *Service) steps() []step {
	return []step{
		{"reach", "Connect to the node", s.stepReach},
		{"privileges", "Check root or sudo access", s.stepPrivileges},
		{"tools", "Check curl", s.stepTools},
		{"docker", "Install docker", s.stepDocker},
		{"daemon", "Start the docker service", s.stepDaemon},
		{"access", "Grant the login user docker access", s.stepAccess},
		{"ports", "Check the swarm ports", s.stepPorts},
		{"swarm", "Join the swarm", s.stepSwarm},
	}
}

// Provision runs the checklist against a node in the background and returns the
// job immediately: installing docker takes minutes, far longer than a request
// should be held open. The UI polls ProvisionStatus for progress.
func (s *Service) Provision(id string) (ProvisionJob, error) {
	item, err := s.repo.Get(id)
	if err != nil {
		return ProvisionJob{}, err
	}
	if item.Local {
		return ProvisionJob{}, ErrLocalSetupManaged
	}

	// A node stacker cannot log into cannot be provisioned, and every step
	// after the first would fail in the same confusing way.
	if !item.Local && item.KeyStatus != KeyStatusOK {
		return ProvisionJob{}, ErrKeyNotVerified
	}

	// The same per-node lock the direct swarm calls take, so a configure and a
	// promote cannot interleave on one host.
	unlock, err := s.lock(id)
	if err != nil {
		return ProvisionJob{}, ErrSwarmBusy
	}

	job := &ProvisionJob{
		NodeID:    id,
		State:     ProvisionRunning,
		StartedAt: time.Now(),
		Steps:     make([]StepResult, 0, len(s.steps())),
	}
	for _, st := range s.steps() {
		job.Steps = append(job.Steps, StepResult{Key: st.key, Title: st.title, State: StepPending})
	}

	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()

	go func() {
		defer unlock()

		// The job outlives the request that started it, so it gets its own
		// context with a ceiling rather than borrowing the caller's.
		ctx, cancel := context.WithTimeout(context.Background(), provisionMaxRuntime)
		defer cancel()

		s.runSteps(ctx, item, job)
	}()

	return *s.snapshot(job), nil
}

// ProvisionStatus returns the latest run for a node, if there has been one.
func (s *Service) ProvisionStatus(id string) (ProvisionJob, error) {
	s.mu.Lock()
	job := s.jobs[id]
	s.mu.Unlock()

	if job == nil {
		return ProvisionJob{}, ErrNoProvisionRun
	}
	return *s.snapshot(job), nil
}

// runSteps walks the checklist, stopping at the first hard failure.
func (s *Service) runSteps(ctx context.Context, item Node, job *ProvisionJob) {
	p := &provisionCtx{node: item}

	for i, st := range s.steps() {
		s.setStep(job, i, StepRunning, "", true)

		state, detail, err := st.run(ctx, p)
		if err != nil {
			s.setStep(job, i, StepFailed, err.Error(), false)
			s.finish(job, ProvisionFailed, fmt.Sprintf("%s failed: %s", st.title, err))
			s.log.Warn("provisioning failed", "node", p.node.Name, "step", st.key, "error", err)
			return
		}
		s.setStep(job, i, state, detail, false)
	}

	s.finish(job, ProvisionSucceeded, "The node is configured and part of the swarm")
	s.log.Info("node provisioned", "node", p.node.Name)
}

/* ---- steps ---- */

// stepReach proves stacker can run a command on the node at all, and records
// which distribution it is, so later failures are not mistaken for ssh trouble.
func (s *Service) stepReach(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	out, err := s.rt.runFor(ctx, p.node, probeTimeout, "uname", "-s")
	if err != nil {
		return StepFailed, "", fmt.Errorf("%w: %s", ErrSwarmUnreachable, err)
	}

	kernel := lastLine(out)

	// The install script supports Linux only. A non-Linux host is still usable
	// when docker is already there — a mac running Docker Desktop is the
	// obvious case, and it is often the local node — so the install steps stand
	// down instead of the whole run refusing.
	if !strings.EqualFold(kernel, "Linux") {
		if _, err := s.rt.runFor(ctx, p.node, probeTimeout, "docker", "--version"); err != nil {
			if p.node.Local {
				return StepFailed, "", ErrLocalNotLinux
			}
			return StepFailed, "", fmt.Errorf("%w: the node reports %s", ErrUnsupportedOS, kernel)
		}

		p.skipInstall = true
		p.distro = kernel
		return StepWarned, kernel + " — using the docker already installed, nothing will be installed", nil
	}

	// Best effort — a missing os-release only costs a nicer label.
	if name, err := s.rt.shell(ctx, p.node, probeTimeout,
		". /etc/os-release 2>/dev/null && printf '%s' \"$PRETTY_NAME\""); err == nil {
		p.distro = lastLine(name)
	}

	if p.distro == "" {
		p.distro = kernel
	}
	return StepDone, p.distro, nil
}

// stepPrivileges works out how privileged commands should be run. Installing
// docker needs root, and stacker only ever uses non-interactive sudo, so a host
// that would prompt for a password is rejected up front rather than hanging.
func (s *Service) stepPrivileges(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	if p.skipInstall {
		return StepSkipped, "Nothing to install, so no privileges needed", nil
	}

	if out, err := s.rt.runFor(ctx, p.node, probeTimeout, "id", "-u"); err == nil && lastLine(out) == "0" {
		p.sudo = ""
		return StepDone, "Running as root", nil
	}

	if _, err := s.rt.runFor(ctx, p.node, probeTimeout, "sudo", "-n", "true"); err != nil {
		return StepFailed, "", ErrSudoRequired
	}

	p.sudo = "sudo -n"
	return StepDone, "Using passwordless sudo", nil
}

// stepTools makes sure the install script can be fetched at all. curl is what
// the documented one-liner uses, so a host without it is fixed here rather than
// failing inside the installer.
func (s *Service) stepTools(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	if p.skipInstall {
		return StepSkipped, "Nothing to fetch", nil
	}

	if _, err := s.rt.runFor(ctx, p.node, probeTimeout, "sh", "-c", "command -v curl"); err == nil {
		return StepSkipped, "curl is already installed", nil
	}

	// Only the two families the docker script itself supports are attempted;
	// anything else is left to the user with a clear message.
	install := p.privileged("(apt-get update -qq && apt-get install -y -qq curl) || " +
		"dnf install -y -q curl || yum install -y -q curl")

	if _, err := s.rt.shell(ctx, p.node, installTimeout, install); err != nil {
		return StepFailed, "", fmt.Errorf("%w: %s", ErrCurlMissing, err)
	}
	return StepDone, "Installed curl", nil
}

// stepDocker installs docker with the official convenience script, but only
// when it is genuinely absent — reinstalling a working docker would be a
// surprising thing for a Configure button to do.
func (s *Service) stepDocker(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	if out, err := s.rt.runFor(ctx, p.node, probeTimeout, "docker", "--version"); err == nil {
		return StepSkipped, lastLine(out), nil
	}

	if _, err := s.rt.shell(ctx, p.node, installTimeout, p.privileged(recoverPackages+dockerInstallScript)); err != nil {
		return StepFailed, "", fmt.Errorf("%w: %s", ErrDockerInstall, err)
	}

	// Confirm rather than trust the installer's exit code.
	out, err := s.rt.runFor(ctx, p.node, probeTimeout, "docker", "--version")
	if err != nil {
		return StepFailed, "", fmt.Errorf("%w: docker is still not on PATH", ErrDockerInstall)
	}
	return StepDone, lastLine(out), nil
}

// stepDaemon makes sure dockerd is up now and after a reboot. Not every host
// runs systemd (containers, some minimal images), so its absence is a warning
// rather than a failure — the join below is the real test.
func (s *Service) stepDaemon(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	if _, err := s.rt.docker(ctx, p.node, "info", "--format", "{{.ServerVersion}}"); err == nil {
		return StepSkipped, "The docker service is already running", nil
	}

	if p.skipInstall {
		return StepFailed, "", ErrDockerNotRunning
	}

	if _, err := s.rt.runFor(ctx, p.node, probeTimeout, "sh", "-c", "command -v systemctl"); err != nil {
		return StepWarned, "No systemctl on this host — start the docker service yourself", nil
	}

	if _, err := s.rt.shell(ctx, p.node, serviceTimeout, p.privileged("systemctl enable --now docker")); err != nil {
		return StepFailed, "", fmt.Errorf("%w: %s", ErrDockerNotRunning, err)
	}
	return StepDone, "Started and enabled the docker service", nil
}

// stepAccess adds the ssh login user to the docker group, so stacker (and the
// user) can talk to the socket without sudo on the next login. Until then,
// docker calls fall back to sudo automatically.
func (s *Service) stepAccess(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	// Root already has access, and the local node runs as whoever started
	// stacker — changing that user's groups is not this feature's business.
	if p.skipInstall || p.sudo == "" || p.node.Local {
		return StepSkipped, "No group change needed", nil
	}

	user, _ := splitSsh(p.node.Ssh)
	if user == "" {
		return StepSkipped, "No login user to add", nil
	}

	if _, err := s.rt.shell(ctx, p.node, probeTimeout, fmt.Sprintf("id -nG %s | tr ' ' '\\n' | grep -qx docker", shellQuote(user))); err == nil {
		return StepSkipped, fmt.Sprintf("%s is already in the docker group", user), nil
	}

	if _, err := s.rt.shell(ctx, p.node, serviceTimeout, p.privileged("usermod -aG docker "+shellQuote(user))); err != nil {
		// Stacker falls back to sudo for docker commands, so this is a
		// convenience, not a requirement.
		return StepWarned, fmt.Sprintf("Could not add %s to the docker group — stacker will use sudo instead", user), nil
	}
	return StepDone, fmt.Sprintf("Added %s to the docker group", user), nil
}

// stepPorts reports on the ports a swarm needs. It only ever looks: opening
// ports on someone's host is a security decision that is theirs to make, and a
// silent firewall change is exactly the kind of surprise a Configure button
// should not spring.
func (s *Service) stepPorts(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	return StepSkipped, "Only managers accept inbound swarm traffic", nil
}

// stepSwarm joins the remote node to the installer-created manager.
func (s *Service) stepSwarm(ctx context.Context, p *provisionCtx) (StepState, string, error) {
	item, err := s.repo.Get(p.node.ID)
	if err != nil {
		return StepFailed, "", err
	}

	result, err := s.joinWorker(ctx, item)
	if err != nil {
		return StepFailed, "", err
	}

	p.node = result.Node
	return StepDone, result.Message, nil
}

/* ---- helpers ---- */

// privileged prefixes a script with sudo when one is needed. The whole script
// runs under it, so a pipeline's writes are privileged too.
func (p *provisionCtx) privileged(script string) string {
	if p.sudo == "" {
		return script
	}
	return p.sudo + " sh -c " + shellQuote(script)
}

// setStep updates one line of the checklist under the lock, since the UI polls
// it while the job's goroutine writes it.
func (s *Service) setStep(job *ProvisionJob, index int, state StepState, detail string, starting bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	job.Steps[index].State = state
	if detail != "" {
		job.Steps[index].Detail = detail
	}
	if starting {
		job.Steps[index].StartedAt = &now
		return
	}
	job.Steps[index].EndedAt = &now
}

func (s *Service) finish(job *ProvisionJob, state ProvisionState, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	job.State = state
	job.Message = message
	job.FinishedAt = &now
}

// snapshot copies a job under the lock, so a caller never marshals a struct the
// job's goroutine is writing to.
func (s *Service) snapshot(job *ProvisionJob) *ProvisionJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	clone := *job
	clone.Steps = append([]StepResult(nil), job.Steps...)
	return &clone
}
