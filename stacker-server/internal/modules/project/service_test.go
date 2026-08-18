package project

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/goccy/go-yaml"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

/* ---- helpers ---- */

func testDB(t *testing.T) *gorm.DB {
	t.Helper()

	// The foreign_keys pragma matches database.Open. Without it sqlite ignores
	// the constraints AutoMigrate declares, and a delete ordered the wrong way
	// round would pass here and fail against a real installation.
	path := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&Project{}, &Environment{}, &Deployment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recorder is a fake Exec that logs the commands it was asked to run and replies
// with canned output, so the deploy engine can be exercised without docker.
type recorder struct {
	mu   sync.Mutex
	runs []Command
	// reply returns the lines a command emits and the error it ends with, keyed
	// on a substring of the rendered command.
	reply func(cmd Command) ([]string, error)
	// passThrough names commands that really run instead of being faked. Only
	// git is ever let through, and only by the test that needs a real checkout
	// on disk to check against.
	passThrough map[string]bool
}

func (r *recorder) exec(ctx context.Context, cmd Command, sink Sink) error {
	r.mu.Lock()
	r.runs = append(r.runs, cmd)
	reply := r.reply
	live := r.passThrough[cmd.Name]
	r.mu.Unlock()

	if live {
		return execCommand(ctx, cmd, sink)
	}

	var lines []string
	var err error
	if reply != nil {
		lines, err = reply(cmd)
	}
	for _, line := range lines {
		sink(line)
	}
	return err
}

// find returns the first recorded command containing every fragment.
func (r *recorder) find(fragments ...string) (Command, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, cmd := range r.runs {
		text := cmd.String()
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return cmd, true
		}
	}
	return Command{}, false
}

func testService(t *testing.T, opts Options) (*Service, *recorder) {
	t.Helper()

	if opts.WorkRoot == "" {
		opts.WorkRoot = filepath.Join(t.TempDir(), "work")
	}
	if opts.TraefikDynamicPath == "" {
		dir := t.TempDir()
		opts.TraefikDynamicPath = filepath.Join(dir, "stacker.yml")
	}
	if opts.Network == "" {
		opts.Network = "stacker_proxy"
	}

	service := NewService(NewRepository(testDB(t)), opts, silentLog())

	rec := &recorder{}
	service.engine.exec = rec.exec
	service.status.exec = rec.exec
	if err := os.MkdirAll(opts.WorkRoot, 0o700); err != nil {
		t.Fatalf("workroot: %v", err)
	}
	return service, rec
}

func writeRequest() WriteRequest {
	return WriteRequest{
		Name:       "storefront",
		SourceKind: SourceCompose,
		Compose:    "services:\n  web:\n    image: nginx:alpine\n",
		Environments: []EnvironmentRequest{{
			Name: "production",
			Domains: []DomainRequest{{
				Host: "shop.acme.dev", Service: "web", Port: 3000, TLS: TLSAuto,
			}},
			Deploy: DeploySettings{Strategy: StrategyRolling, Replicas: 2},
		}},
	}
}

/* ---- validation ---- */

func TestCreateNormalisesAndStores(t *testing.T) {
	service, _ := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(item.Environments) != 1 {
		t.Fatalf("environments = %d, want 1", len(item.Environments))
	}

	env := item.Environments[0]
	if env.ID == "" {
		t.Error("environment id was not generated")
	}
	if got := env.Domains[0].ID; got == "" {
		t.Error("domain id was not generated")
	}
	if got := env.Deploy.HealthGraceSec; got != 30 {
		t.Errorf("health grace = %d, want the 30s default", got)
	}

	// Reading it back proves the JSON columns round-trip through sqlite.
	stored, err := service.Get(item.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := stored.Environments[0].Domains[0].Host; got != "shop.acme.dev" {
		t.Errorf("host = %q, want shop.acme.dev", got)
	}
	if got := stored.Environments[0].Deploy.Replicas; got != 2 {
		t.Errorf("replicas = %d, want 2", got)
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	cases := map[string]struct {
		mutate func(*WriteRequest)
		want   error
	}{
		"blank name":        {func(r *WriteRequest) { r.Name = " " }, ErrInvalidName},
		"name with a space": {func(r *WriteRequest) { r.Name = "my project" }, ErrInvalidName},
		"unknown source":    {func(r *WriteRequest) { r.SourceKind = "svn" }, ErrInvalidSource},
		"no compose": {func(r *WriteRequest) {
			r.Compose = ""
		}, ErrComposeRequired},
		"compose with no services": {func(r *WriteRequest) {
			r.Compose = "version: '3'\n"
		}, ErrNoServices},
		"unparseable compose": {func(r *WriteRequest) {
			r.Compose = "services: [oh: no: yes\n"
		}, ErrComposeInvalid},
		"git with no repo": {func(r *WriteRequest) {
			r.SourceKind = SourceGit
			r.Git = GitSource{Branch: "main", ComposePath: "docker-compose.yml"}
		}, ErrRepoRequired},
		"git with no branch": {func(r *WriteRequest) {
			r.SourceKind = SourceGit
			r.Git = GitSource{Repo: "acme/store", ComposePath: "docker-compose.yml"}
		}, ErrBranchRequired},
		"git with no compose path": {func(r *WriteRequest) {
			r.SourceKind = SourceGit
			r.Git = GitSource{Repo: "acme/store", Branch: "main"}
		}, ErrComposePath},
		"no environments": {func(r *WriteRequest) {
			r.Environments = nil
		}, ErrEnvRequired},
		"domain port too high": {func(r *WriteRequest) {
			r.Environments[0].Domains[0].Port = 70000
		}, ErrDomainPort},
		"host with a scheme": {func(r *WriteRequest) {
			r.Environments[0].Domains[0].Host = "https://shop.acme.dev"
		}, ErrDomainHost},
		"host with a path": {func(r *WriteRequest) {
			r.Environments[0].Domains[0].Host = "shop.acme.dev/app"
		}, ErrDomainHost},
		"single label host": {func(r *WriteRequest) {
			r.Environments[0].Domains[0].Host = "localhost"
		}, ErrDomainHost},
		"domain with no service": {func(r *WriteRequest) {
			r.Environments[0].Domains[0].Service = ""
		}, ErrDomainService},
		"duplicate environment names": {func(r *WriteRequest) {
			r.Environments = append(r.Environments, EnvironmentRequest{Name: "production"})
		}, ErrEnvName},
		"too many replicas": {func(r *WriteRequest) {
			r.Environments[0].Deploy.Replicas = 5000
		}, ErrReplicas},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			service, _ := testService(t, Options{})
			req := writeRequest()
			tc.mutate(&req)

			if _, err := service.Create(req); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateRejectsADuplicateName(t *testing.T) {
	service, _ := testService(t, Options{})
	if _, err := service.Create(writeRequest()); err != nil {
		t.Fatalf("create: %v", err)
	}

	second := writeRequest()
	second.Environments[0].Domains[0].Host = "other.acme.dev"
	if _, err := service.Create(second); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("error = %v, want %v", err, ErrNameTaken)
	}
}

func TestCreateRejectsAHostAnotherProjectRoutes(t *testing.T) {
	service, _ := testService(t, Options{})
	if _, err := service.Create(writeRequest()); err != nil {
		t.Fatalf("create: %v", err)
	}

	second := writeRequest()
	second.Name = "other"
	if _, err := service.Create(second); !errors.Is(err, ErrDomainTaken) {
		t.Fatalf("error = %v, want %v", err, ErrDomainTaken)
	}
}

// Secrets are redacted on the way out, so a save that round-trips the redacted
// values must not wipe them.
func TestUpdateKeepsSecretsSentBackBlank(t *testing.T) {
	service, _ := testService(t, Options{})

	req := writeRequest()
	req.Environments[0].Secrets = []EnvVar{{Key: "SESSION_SECRET", Value: "s3cret"}}
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := item.Environments[0].Secrets[0].Value; got != "" {
		t.Errorf("the response carried the secret value %q", got)
	}

	// What the browser would send back: the same key, no value.
	update := writeRequest()
	update.Environments[0].ID = item.Environments[0].ID
	update.Environments[0].Secrets = []EnvVar{{Key: "SESSION_SECRET"}}
	if _, err := service.Update(context.Background(), item.ID, update); err != nil {
		t.Fatalf("update: %v", err)
	}

	stored, err := service.repo.Get(item.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got := stored.Environments[0].Secrets[0].Value; got != "s3cret" {
		t.Errorf("stored secret = %q, want it kept", got)
	}
}

func TestUpdateRemovesAnAbsentEnvironment(t *testing.T) {
	service, rec := testService(t, Options{})

	req := writeRequest()
	req.Environments = append(req.Environments, EnvironmentRequest{
		Name:    "staging",
		Domains: []DomainRequest{{Host: "staging.acme.dev", Service: "web", Port: 3000}},
	})
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	removed := item.Environments[1]

	update := writeRequest()
	update.Environments[0].ID = item.Environments[0].ID
	saved, err := service.Update(context.Background(), item.ID, update)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(saved.Environments) != 1 {
		t.Fatalf("environments = %d, want 1", len(saved.Environments))
	}

	// The stack of the removed environment has to go with it, or it keeps
	// running with nothing in the UI pointing at it.
	if _, ok := rec.find("stack", "rm", StackName(item, removed)); !ok {
		t.Error("the removed environment's stack was not torn down")
	}
}

// Guards the guard: AutoMigrate declares a foreign key from environments onto
// projects, and if the test database ever stopped enforcing it, the delete above
// would pass here while failing on a real installation.
func TestTestDatabaseEnforcesForeignKeys(t *testing.T) {
	db := testDB(t)

	var on int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&on).Error; err != nil {
		t.Fatalf("read pragma: %v", err)
	}
	if on != 1 {
		t.Fatal("the test database is not enforcing foreign keys")
	}

	item := Project{ID: newID(), Name: "x", Environments: []Environment{{ID: newID(), Name: "e"}}}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Delete(&Project{}, "id = ?", item.ID).Error; err == nil {
		t.Fatal("deleting a project before its environments was allowed")
	}
}

func TestDeleteTearsDownEveryEnvironment(t *testing.T) {
	service, rec := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stack := StackName(item, item.Environments[0])

	if err := service.Delete(context.Background(), item.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := rec.find("stack", "rm", stack); !ok {
		t.Error("the stack was not removed")
	}
	if _, err := service.Get(item.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("error = %v, want %v", err, ErrNotFound)
	}
}

/* ---- compose overlay ---- */

func TestParseComposeReadsWhatTheDeployNeeds(t *testing.T) {
	spec, err := parseCompose(`
services:
  web:
    build: .
    ports: ["3000:3000"]
  worker:
    build:
      context: ./worker
    image: acme/worker:1
  db:
    image: postgres:16
    deploy:
      replicas: 1
      placement:
        constraints: [node.role == manager]
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if got := strings.Join(spec.Names, ","); got != "db,web,worker" {
		t.Errorf("names = %q, want them sorted", got)
	}
	if got := strings.Join(spec.Builds, ","); got != "web,worker" {
		t.Errorf("builds = %q, want web,worker", got)
	}
	// `worker` names its own image, so stacker must not invent a tag for it.
	if !spec.NeedsImageTag["web"] || spec.NeedsImageTag["worker"] {
		t.Errorf("needs-image-tag = %v, want only web", spec.NeedsImageTag)
	}
	if !spec.PinnedReplicas["db"] || spec.PinnedReplicas["web"] {
		t.Errorf("pinned replicas = %v, want only db", spec.PinnedReplicas)
	}
	if !spec.PinnedPlacement["db"] {
		t.Error("db's placement constraint was not detected")
	}
}

func TestBuildOverrideLeavesPinnedServicesAlone(t *testing.T) {
	spec, err := parseCompose(`
services:
  web:
    build: .
  db:
    image: postgres:16
    deploy:
      replicas: 1
`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	out, err := buildOverride(spec, overrideOptions{
		Stack:         "stk-shop-production",
		Network:       "stacker_proxy",
		ProxyServices: map[string]bool{"web": true},
		Env:           map[string]string{"NODE_ENV": "production"},
		Deploy: DeploySettings{
			Strategy: StrategyRolling, Replicas: 3, Placement: "node.labels.tier==edge",
			HealthGraceSec: 45, AutoRollback: true,
		},
	})
	if err != nil {
		t.Fatalf("override: %v", err)
	}

	var doc struct {
		Services map[string]struct {
			Image       string
			Networks    []string
			Environment map[string]string
			Deploy      struct {
				Replicas     *int
				Placement    *struct{ Constraints []string }
				UpdateConfig struct {
					Order         string
					Monitor       string
					FailureAction string `yaml:"failure_action"`
				} `yaml:"update_config"`
			}
		}
		Networks map[string]struct {
			External bool
			Name     string
		}
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("the override is not valid yaml: %v\n%s", err, out)
	}

	web := doc.Services["web"]
	if web.Image != "stacker/stk-shop-production/web:latest" {
		t.Errorf("web image = %q, want a generated tag", web.Image)
	}
	if web.Deploy.Replicas == nil || *web.Deploy.Replicas != 3 {
		t.Errorf("web replicas = %v, want 3", web.Deploy.Replicas)
	}
	if got := strings.Join(web.Networks, ","); got != "default,proxy" {
		t.Errorf("web networks = %q, want default,proxy — dropping default cuts it off from db", got)
	}
	if web.Environment["NODE_ENV"] != "production" {
		t.Errorf("web environment = %v, want NODE_ENV", web.Environment)
	}
	if web.Deploy.UpdateConfig.Order != "start-first" {
		t.Errorf("update order = %q, want start-first for a rolling deploy", web.Deploy.UpdateConfig.Order)
	}
	if web.Deploy.UpdateConfig.Monitor != "45s" {
		t.Errorf("monitor = %q, want 45s", web.Deploy.UpdateConfig.Monitor)
	}
	if web.Deploy.UpdateConfig.FailureAction != "rollback" {
		t.Errorf("failure action = %q, want rollback", web.Deploy.UpdateConfig.FailureAction)
	}

	db := doc.Services["db"]
	if db.Deploy.Replicas != nil {
		t.Errorf("db replicas = %v, want the compose file's own value left alone", *db.Deploy.Replicas)
	}
	// Placement is a property of where the environment runs, not of what is
	// routed, so it does apply to db — unlike replicas, which db pins itself.
	if db.Deploy.Placement == nil || len(db.Deploy.Placement.Constraints) != 1 {
		t.Errorf("db placement = %+v, want the environment's constraint", db.Deploy.Placement)
	}
	if db.Image != "" {
		t.Errorf("db image = %q, want it untouched", db.Image)
	}
	if len(db.Networks) != 0 {
		t.Errorf("db networks = %v, want none — nothing routes to it", db.Networks)
	}

	if network := doc.Networks["proxy"]; !network.External || network.Name != "stacker_proxy" {
		t.Errorf("proxy network = %+v, want the external stacker_proxy", network)
	}
}

func TestBuildOverrideStopsFirstWhenRecreating(t *testing.T) {
	spec, err := parseCompose("services:\n  web:\n    image: nginx\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	out, err := buildOverride(spec, overrideOptions{
		Stack:  "stk-a-b",
		Deploy: DeploySettings{Strategy: StrategyRecreate, Replicas: 1},
	})
	if err != nil {
		t.Fatalf("override: %v", err)
	}
	if !strings.Contains(out, "stop-first") {
		t.Errorf("override does not stop first:\n%s", out)
	}
}

/* ---- traefik routes ---- */

func TestRenderRoutesPointsAtTheSwarmService(t *testing.T) {
	out, err := renderRoutes("stk-shop-production", []Domain{
		{Host: "shop.acme.dev", Service: "web", Port: 3000, TLS: TLSAuto, RedirectWww: true},
		{Host: "internal.acme.dev", Service: "api", Port: 8080, TLS: TLSNone},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc routes
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("the routes are not valid yaml: %v\n%s", err, out)
	}

	first := doc.HTTP.Routers["stk-shop-production-0"]
	if !strings.Contains(first.Rule, "Host(`shop.acme.dev`, `www.shop.acme.dev`)") {
		t.Errorf("rule = %q, want both hosts so the www redirect is reachable", first.Rule)
	}
	if first.TLS == nil || first.TLS.CertResolver != certResolver {
		t.Errorf("tls = %+v, want the acme resolver", first.TLS)
	}
	if got := strings.Join(first.EntryPoints, ","); got != "websecure" {
		t.Errorf("entrypoints = %q, want websecure", got)
	}
	if len(first.Middlewares) != 1 {
		t.Errorf("middlewares = %v, want the www redirect", first.Middlewares)
	}
	if doc.HTTP.Middlewares["stk-shop-production-0-www"].RedirectRegex == nil {
		t.Error("the www redirect middleware is missing")
	}

	url := doc.HTTP.Services["stk-shop-production-0"].LoadBalancer.Servers[0].URL
	if url != "http://stk-shop-production_web:3000" {
		t.Errorf("backend = %q, want the stack-prefixed swarm service", url)
	}

	second := doc.HTTP.Routers["stk-shop-production-1"]
	if second.TLS != nil {
		t.Errorf("tls = %+v, want none for a plain-http domain", second.TLS)
	}
	if got := strings.Join(second.EntryPoints, ","); got != "web" {
		t.Errorf("entrypoints = %q, want web", got)
	}
}

func TestRouterReplacesAndRemovesItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	r := newRouter(filepath.Join(dir, "stacker.yml"))

	domains := []Domain{{Host: "shop.acme.dev", Service: "web", Port: 80, TLS: TLSAuto}}
	if err := r.Apply("env1", "stk-shop-production", domains); err != nil {
		t.Fatalf("apply: %v", err)
	}

	path := filepath.Join(dir, "stacker-project-env1.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the route file was not written: %v", err)
	}

	// No domains means no route: an environment that loses its last hostname
	// must stop being routed rather than keep pointing at a dead service.
	if err := r.Apply("env1", "stk-shop-production", nil); err != nil {
		t.Fatalf("apply empty: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the route file survived an empty apply: %v", err)
	}

	// Removing twice has to be safe: teardown runs on delete and on rollback.
	if err := r.Remove("env1"); err != nil {
		t.Errorf("remove of a missing file: %v", err)
	}
}

func TestApplyFailsWhenTraefikIsNotInstalled(t *testing.T) {
	r := newRouter(filepath.Join(t.TempDir(), "missing", "stacker.yml"))

	err := r.Apply("env1", "stack", []Domain{{Host: "a.acme.dev", Service: "web", Port: 80}})
	if !errors.Is(err, ErrTraefikMissing) {
		t.Fatalf("error = %v, want %v", err, ErrTraefikMissing)
	}
}

/* ---- the deploy run ---- */

// waitFor polls until the condition holds. The run is on its own goroutine, so
// there is no handle to join on — only the row and the log it writes.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestDeployRunsTheWholeSequence(t *testing.T) {
	traefikDir := t.TempDir()
	service, rec := testService(t, Options{TraefikDynamicPath: filepath.Join(traefikDir, "stacker.yml")})

	req := writeRequest()
	req.Compose = "services:\n  web:\n    build: .\n"
	req.Environments[0].Variables = []EnvVar{{Key: "NODE_ENV", Value: "production"}}
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	env := item.Environments[0]

	deployment, err := service.Deploy(item.ID, env.ID, DeployRequest{Message: "first run", Actor: "adharsh"})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if deployment.Status != StatusQueued {
		t.Errorf("status = %q, want queued", deployment.Status)
	}
	if deployment.Number != 1 {
		t.Errorf("number = %d, want 1", deployment.Number)
	}

	waitFor(t, "the run to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	stored, err := service.Deployment(deployment.ID)
	if err != nil {
		t.Fatalf("read the deployment: %v", err)
	}
	if stored.Status != StatusSucceeded {
		t.Fatalf("status = %q (%s), want succeeded\n%s", stored.Status, stored.Error, stored.Log)
	}
	if stored.DurationSec == nil || stored.FinishedAt == nil {
		t.Error("the run did not record when it finished")
	}

	stack := StackName(item, env)
	build, ok := rec.find("compose", "build")
	if !ok {
		t.Fatal("the compose file declares a build, but nothing was built")
	}
	// The environment's variables have to reach the build, or a `${VAR}` in the
	// compose file interpolates to nothing.
	if !contains(build.Env, "NODE_ENV=production") {
		t.Error("the build did not get the environment's variables")
	}
	// `docker compose` takes -f. Passing -c here is rejected outright.
	if !strings.Contains(build.String(), " -f ") {
		t.Errorf("build args = %q, want compose files passed as -f", build.String())
	}

	deploy, ok := rec.find("stack", "deploy", stack)
	if !ok {
		t.Fatal("the stack was never deployed")
	}
	// Without this, docker asks a registry to resolve a tag that only exists on
	// this host and the deploy fails.
	if !strings.Contains(deploy.String(), "--resolve-image never") {
		t.Errorf("deploy args = %q, want --resolve-image never", deploy.String())
	}
	if !strings.Contains(deploy.String(), "--prune") {
		t.Errorf("deploy args = %q, want --prune so removed services go away", deploy.String())
	}
	// `docker stack deploy` spells the same flag -c and rejects -f, which is a
	// mistake that only shows up against a real docker.
	if !strings.Contains(deploy.String(), " -c ") || strings.Contains(deploy.String(), " -f ") {
		t.Errorf("deploy args = %q, want compose files passed as -c", deploy.String())
	}

	// The hostname is published only after the stack is accepted.
	if _, err := os.Stat(filepath.Join(traefikDir, "stacker-project-"+env.ID+".yml")); err != nil {
		t.Errorf("the route was not written: %v", err)
	}

	// And the workspace is gone: that is the "clean up after deploy" half.
	entries, err := os.ReadDir(service.engine.workRoot)
	if err != nil {
		t.Fatalf("read the work root: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the workspace survived the run: %v", entries)
	}
}

// A git source clones a real repository here, because the thing being checked is
// exactly what a fake cannot show: where on disk the generated overlay lands.
// Compose resolves `build: .` against the compose file's own directory, so an
// overlay written anywhere but beside the repository's file moves the build
// context and the build fails looking for a Dockerfile that is right there.
func TestDeployGeneratesTheOverlayBesideTheRepositoryComposeFile(t *testing.T) {
	repo := gitRepo(t, map[string]string{
		"deploy/docker-compose.yml": "services:\n  web:\n    build: .\n",
		"deploy/Dockerfile":         "FROM scratch\n",
	})

	service, rec := testService(t, Options{})
	// git runs for real so there is an actual checkout to locate; docker stays
	// faked, since what is being checked is the arguments it would be handed.
	rec.passThrough = map[string]bool{"git": true}

	req := writeRequest()
	req.SourceKind = SourceGit
	req.Compose = ""
	req.Git = GitSource{Repo: "file://" + repo, Branch: "main", ComposePath: "deploy/docker-compose.yml"}
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	stored, _ := service.Deployment(deployment.ID)
	if stored.Status != StatusSucceeded {
		t.Fatalf("status = %q (%s), want succeeded\n%s", stored.Status, stored.Error, stored.Log)
	}
	// The clone really ran, so the revision is a sha rather than the placeholder.
	if stored.Revision == "pending" || stored.Revision == "" {
		t.Errorf("revision = %q, want the checked-out commit", stored.Revision)
	}

	deploy, ok := rec.find("stack", "deploy")
	if !ok {
		t.Fatal("the stack was never deployed")
	}
	if !strings.HasSuffix(deploy.Dir, "/repo/deploy") {
		t.Errorf("deploy ran in %q, want the directory holding the repository's compose file", deploy.Dir)
	}
	if !strings.Contains(deploy.String(), deploy.Dir+"/.stacker-override.yml") {
		t.Errorf("deploy args = %q, want the overlay written beside the compose file", deploy.String())
	}
}

// gitRepo writes files into a fresh repository with one commit and returns its
// path, for use as a `file://` clone source.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A"},
		{"-c", "user.email=test@stacker", "-c", "user.name=test", "commit", "-qm", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git is not usable here (%v): %s", err, out)
		}
	}
	return dir
}

func TestDeployRefusesASecondConcurrentRun(t *testing.T) {
	service, rec := testService(t, Options{})

	// Hold the first run inside its build so it is still in flight.
	release := make(chan struct{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack deploy") {
			<-release
		}
		return nil, nil
	}

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	env := item.Environments[0]

	if _, err := service.Deploy(item.ID, env.ID, DeployRequest{}); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	waitFor(t, "the first run to reach the deploy step", func() bool {
		_, ok := rec.find("stack", "deploy")
		return ok
	})

	_, secondErr := service.Deploy(item.ID, env.ID, DeployRequest{})

	// The first run has to finish before the test returns: it writes into the
	// temp directories t.TempDir is about to remove.
	close(release)
	waitFor(t, "the first run to finish", func() bool {
		items, err := service.Deployments(item.ID, 10)
		return err == nil && len(items) == 1 && items[0].Status.Done()
	})

	if !errors.Is(secondErr, ErrAlreadyDeploying) {
		t.Fatalf("error = %v, want %v", secondErr, ErrAlreadyDeploying)
	}
}

func TestDeployFailsWhenADomainNamesAnUnknownService(t *testing.T) {
	service, _ := testService(t, Options{})

	req := writeRequest()
	req.Environments[0].Domains[0].Service = "frontend" // the compose file has `web`
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to fail", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	stored, _ := service.Deployment(deployment.ID)
	if stored.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", stored.Status)
	}
	if !strings.Contains(stored.Error, "frontend") {
		t.Errorf("error = %q, want it to name the missing service", stored.Error)
	}
}

func TestDeployLogRedactsSecrets(t *testing.T) {
	service, rec := testService(t, Options{})
	rec.reply = func(Command) ([]string, error) {
		// A build that echoes a secret is exactly the accident redaction is for.
		return []string{"building with SESSION_SECRET=s3cret-value"}, nil
	}

	req := writeRequest()
	req.Environments[0].Secrets = []EnvVar{{Key: "SESSION_SECRET", Value: "s3cret-value"}}
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	stored, _ := service.Deployment(deployment.ID)
	if strings.Contains(stored.Log, "s3cret-value") {
		t.Errorf("the secret reached the log:\n%s", stored.Log)
	}
}

func TestLogsReadFromACursor(t *testing.T) {
	service, rec := testService(t, Options{})
	rec.reply = func(Command) ([]string, error) { return []string{"one", "two"}, nil }

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	all, err := service.Logs(deployment.ID, 0)
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	if len(all.Lines) == 0 || !all.Done {
		t.Fatalf("logs = %+v, want a finished run's lines", all)
	}

	// The second poll asks for what came after, and there is nothing.
	tail, err := service.Logs(deployment.ID, all.Next)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	if len(tail.Lines) != 0 {
		t.Errorf("tail = %v, want nothing new", tail.Lines)
	}
	if tail.Next != all.Next {
		t.Errorf("cursor moved from %d to %d with no new lines", all.Next, tail.Next)
	}
}

func TestRecoverClosesOutRunsFromAPreviousProcess(t *testing.T) {
	service, _ := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	orphan := Deployment{
		ID: newID(), ProjectID: item.ID, ProjectName: item.Name,
		EnvironmentID: item.Environments[0].ID, Environment: "production",
		Status: StatusRunning, StartedAt: timeNow(),
	}
	if err := service.repo.CreateDeployment(&orphan); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := service.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}

	stored, err := service.Deployment(orphan.ID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored.Status != StatusFailed {
		t.Errorf("status = %q, want failed — nobody is running it any more", stored.Status)
	}
	if stored.Error == "" {
		t.Error("the closed-out run does not say why")
	}
}

/* ---- live status ---- */

func TestStatusReportsWhatDockerRuns(t *testing.T) {
	service, rec := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stack := StackName(item, item.Environments[0])

	rec.reply = func(cmd Command) ([]string, error) {
		if !strings.Contains(cmd.String(), "service ls") {
			return nil, nil
		}
		return []string{
			`WARNING: something docker felt like saying`,
			`{"Name":"` + stack + `_web","Mode":"replicated","Replicas":"2/3","Image":"nginx:alpine@sha256:abc"}`,
		}, nil
	}

	status, err := service.Status(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if len(status.Environments) != 1 {
		t.Fatalf("environments = %d, want 1", len(status.Environments))
	}

	env := status.Environments[0]
	if env.State != RuntimeDegraded {
		t.Errorf("state = %q, want degraded — one task of three is missing", env.State)
	}
	if env.Running != 2 || env.Desired != 3 {
		t.Errorf("replicas = %d/%d, want 2/3", env.Running, env.Desired)
	}
	if len(env.Services) != 1 || env.Services[0].Name != "web" {
		t.Fatalf("services = %+v, want web with the stack prefix stripped", env.Services)
	}
	if env.Services[0].Image != "nginx:alpine" {
		t.Errorf("image = %q, want the digest dropped", env.Services[0].Image)
	}
	if status.State != RuntimeDegraded {
		t.Errorf("project state = %q, want the worst of its environments", status.State)
	}
}

func TestStatusStatesFromDocker(t *testing.T) {
	cases := map[string]struct {
		lines []string
		err   error
		want  RuntimeState
	}{
		"nothing deployed":  {nil, nil, RuntimeStopped},
		"docker unreadable": {nil, errors.New("docker daemon is not running"), RuntimeUnknown},
		"all tasks up": {
			[]string{`{"Name":"x_web","Replicas":"2/2"}`}, nil, RuntimeRunning,
		},
		// A global service reports a suffix docker adds; it must still parse.
		"global service": {
			[]string{`{"Name":"x_web","Replicas":"1/1 (max 1 per node)"}`}, nil, RuntimeRunning,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			service, rec := testService(t, Options{})
			rec.reply = func(Command) ([]string, error) { return tc.lines, tc.err }

			item, err := service.Create(writeRequest())
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			status, err := service.Status(context.Background(), item.ID)
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if got := status.Environments[0].State; got != tc.want {
				t.Errorf("state = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusSaysDeployingWhileARunIsInFlight(t *testing.T) {
	service, rec := testService(t, Options{})

	release := make(chan struct{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack deploy") {
			<-release
		}
		return nil, nil
	}

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to reach the deploy step", func() bool {
		_, ok := rec.find("stack", "deploy")
		return ok
	})

	status, statusErr := service.Status(context.Background(), item.ID)

	// Let the run finish before the test returns: it writes into the temp
	// directories t.TempDir is about to remove.
	close(release)
	waitFor(t, "the run to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	if statusErr != nil {
		t.Fatalf("status: %v", statusErr)
	}
	// Mid-rollout a stack is short of tasks by design; reporting that as
	// degraded would raise an alarm on every single deploy.
	if got := status.Environments[0].State; got != RuntimeDeploying {
		t.Errorf("state = %q, want deploying", got)
	}
}

func TestStackNameIsDockerSafeAndStable(t *testing.T) {
	item := Project{ID: "abc", Name: "My Store_v2!"}
	env := Environment{ID: "def", Name: "Pre Production"}

	got := StackName(item, env)
	if got != "stk-my-store-v2-pre-production" {
		t.Fatalf("stack = %q", got)
	}
	if got != StackName(item, env) {
		t.Error("the stack name is not stable across calls")
	}
}

/* ---- output scanning ---- */

func TestScanSplitsProgressCarriageReturns(t *testing.T) {
	var lines []string
	scan(strings.NewReader("step 1\rstep 2\nstep 3"), func(line string) {
		lines = append(lines, line)
	})

	if got := strings.Join(lines, "|"); got != "step 1|step 2|step 3" {
		t.Errorf("lines = %q, want docker's repainted progress split", got)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestListRedactsSecrets(t *testing.T) {
	service, _ := testService(t, Options{})

	req := writeRequest()
	req.Environments[0].Secrets = []EnvVar{{Key: "SESSION_SECRET", Value: "s3cret"}}
	if _, err := service.Create(req); err != nil {
		t.Fatalf("create: %v", err)
	}

	items, err := service.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("list = %d, want 1", len(items))
	}
	if got := items[0].Environments[0].Secrets[0].Value; got != "" {
		t.Errorf("listed secret = %q, want it blank", got)
	}
}

func TestStatusAllReportsEveryProject(t *testing.T) {
	service, rec := testService(t, Options{})
	rec.reply = func(Command) ([]string, error) {
		return []string{`{"Name":"x_web","Replicas":"1/1"}`, `{not json`, `{"Name":"x_api","Replicas":"n/a"}`}, nil
	}

	first, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	secondReq := writeRequest()
	secondReq.Name = "other"
	secondReq.Environments[0].Domains[0].Host = "other.acme.dev"
	second, err := service.Create(secondReq)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	statuses, err := service.StatusAll(context.Background())
	if err != nil {
		t.Fatalf("status all: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("status all = %d, want 2", len(statuses))
	}

	seen := map[string]bool{}
	for _, status := range statuses {
		seen[status.ProjectID] = true
	}
	if !seen[first.ID] || !seen[second.ID] {
		t.Errorf("status all ids = %v, want both projects", seen)
	}
}

func TestStopRemovesTheStack(t *testing.T) {
	service, rec := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	env := item.Environments[0]
	stack := StackName(item, env)

	if err := service.Stop(context.Background(), item.ID, env.ID); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, ok := rec.find("stack", "rm", stack); !ok {
		t.Error("the stack was not removed")
	}

	if err := service.Stop(context.Background(), "missing", env.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing project: %v, want %v", err, ErrNotFound)
	}
	if err := service.Stop(context.Background(), item.ID, "missing"); !errors.Is(err, ErrEnvNotFound) {
		t.Errorf("missing env: %v, want %v", err, ErrEnvNotFound)
	}
}

func TestTeardownIgnoresNothingFound(t *testing.T) {
	service, rec := testService(t, Options{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack rm") {
			return nil, errors.New("Nothing found in stack: stk-storefront-production")
		}
		return nil, nil
	}

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := service.Stop(context.Background(), item.ID, item.Environments[0].ID); err != nil {
		t.Fatalf("stop with nothing found: %v", err)
	}
}

func TestTeardownReportsARealDockerError(t *testing.T) {
	service, rec := testService(t, Options{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack rm") {
			return nil, errors.New("Cannot connect to the Docker daemon")
		}
		return nil, nil
	}

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := service.Stop(context.Background(), item.ID, item.Environments[0].ID); err == nil {
		t.Fatal("stop swallowed a docker error")
	}

	// Delete still succeeds: the record has to go even when docker is down.
	if err := service.Delete(context.Background(), item.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestCancelStopsALiveRun(t *testing.T) {
	service, rec := testService(t, Options{})

	release := make(chan struct{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack deploy") {
			<-release
			return nil, context.Canceled
		}
		return nil, nil
	}

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to reach deploy", func() bool {
		_, ok := rec.find("stack", "deploy")
		return ok
	})

	live, err := service.Logs(deployment.ID, 0)
	if err != nil {
		t.Fatalf("live logs: %v", err)
	}
	if live.Done {
		t.Error("live logs reported the run as done")
	}
	if len(live.Lines) == 0 {
		t.Error("live logs were empty")
	}

	if err := service.Cancel(deployment.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	close(release)

	waitFor(t, "the run to cancel", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})
	stored, _ := service.Deployment(deployment.ID)
	if stored.Status != StatusCancelled {
		t.Errorf("status = %q, want cancelled", stored.Status)
	}

	if err := service.Cancel(deployment.ID); !errors.Is(err, ErrNotRunning) {
		t.Errorf("second cancel: %v, want %v", err, ErrNotRunning)
	}
}

func TestUpdateRenameTearsDownTheOldStack(t *testing.T) {
	service, rec := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	oldStack := StackName(item, item.Environments[0])

	update := writeRequest()
	update.Name = "shop"
	update.Environments[0].ID = item.Environments[0].ID
	saved, err := service.Update(context.Background(), item.ID, update)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if saved.Name != "shop" {
		t.Errorf("name = %q, want shop", saved.Name)
	}
	if _, ok := rec.find("stack", "rm", oldStack); !ok {
		t.Error("the renamed project's old stack was not torn down")
	}
}

func TestDeployAlwaysPull(t *testing.T) {
	t.Run("build gets --pull", func(t *testing.T) {
		service, rec := testService(t, Options{})

		req := writeRequest()
		req.Compose = "services:\n  web:\n    build: .\n"
		req.Environments[0].Deploy.AlwaysPull = true
		item, err := service.Create(req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
		if err != nil {
			t.Fatalf("deploy: %v", err)
		}
		waitFor(t, "the run to finish", func() bool {
			stored, err := service.Deployment(deployment.ID)
			return err == nil && stored.Status.Done()
		})

		build, ok := rec.find("compose", "build")
		if !ok {
			t.Fatal("nothing was built")
		}
		if !strings.Contains(build.String(), "--pull") {
			t.Errorf("build args = %q, want --pull", build.String())
		}
	})

	t.Run("image-only pulls then continues on failure", func(t *testing.T) {
		service, rec := testService(t, Options{})
		rec.reply = func(cmd Command) ([]string, error) {
			if strings.Contains(cmd.String(), " pull ") {
				return []string{"unauthorized"}, errors.New("pull failed")
			}
			return nil, nil
		}

		req := writeRequest()
		req.Environments[0].Deploy.AlwaysPull = true
		item, err := service.Create(req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
		if err != nil {
			t.Fatalf("deploy: %v", err)
		}
		waitFor(t, "the run to finish", func() bool {
			stored, err := service.Deployment(deployment.ID)
			return err == nil && stored.Status.Done()
		})

		stored, _ := service.Deployment(deployment.ID)
		if stored.Status != StatusSucceeded {
			t.Fatalf("status = %q (%s), want succeeded\n%s", stored.Status, stored.Error, stored.Log)
		}
		if _, ok := rec.find("compose", "pull", "--ignore-buildable"); !ok {
			t.Error("images were not pulled")
		}
		if !strings.Contains(stored.Log, "pull failed, continuing") {
			t.Errorf("log = %s, want it to keep going after a pull failure", stored.Log)
		}
	})
}

func TestCloneURLAndTokenRedaction(t *testing.T) {
	cases := []struct {
		name        string
		repo        string
		token       string
		tokenErr    error
		wantURL     string
		wantDisplay string
		wantErr     error
		wantMasked  bool
	}{
		{"empty repo", "", "", nil, "", "", ErrRepoRequired, false},
		{"owner name", "acme/app", "", nil, "https://github.com/acme/app.git", "https://github.com/acme/app.git", nil, false},
		{
			name:        "https github token",
			repo:        "acme/app",
			token:       "ghs_secret",
			wantURL:     "https://x-access-token:ghs_secret@github.com/acme/app.git",
			wantDisplay: "https://github.com/acme/app.git",
			wantMasked:  true,
		},
		{
			name:        "ssh ignores token",
			repo:        "git@github.com:acme/app.git",
			token:       "ghs_secret",
			wantURL:     "git@github.com:acme/app.git",
			wantDisplay: "git@github.com:acme/app.git",
		},
		{
			name:        "token error is not fatal",
			repo:        "acme/app",
			tokenErr:    errors.New("github is not connected"),
			wantURL:     "https://github.com/acme/app.git",
			wantDisplay: "https://github.com/acme/app.git",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{}
			if tc.token != "" || tc.tokenErr != nil {
				opts.Token = func(context.Context) (string, error) { return tc.token, tc.tokenErr }
			}
			service, _ := testService(t, opts)

			url, display, err := service.engine.cloneURL(context.Background(), Project{
				Git: GitSource{Repo: tc.repo},
			}, &run{})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("cloneURL: %v", err)
			}
			if url != tc.wantURL {
				t.Errorf("url = %q, want %q", url, tc.wantURL)
			}
			if display != tc.wantDisplay {
				t.Errorf("display = %q, want %q", display, tc.wantDisplay)
			}
		})
	}

	service, rec := testService(t, Options{
		Token: func(context.Context) (string, error) { return "ghs_live_secret_token", nil },
	})
	rec.reply = func(cmd Command) ([]string, error) {
		return []string{"fatal: using ghs_live_secret_token"}, nil
	}

	req := writeRequest()
	req.SourceKind = SourceGit
	req.Compose = ""
	req.Git = GitSource{Repo: "acme/store", Branch: "main", ComposePath: "docker-compose.yml"}
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	deployment, err := service.Deploy(item.ID, item.Environments[0].ID, DeployRequest{})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	waitFor(t, "the run to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	clone, ok := rec.find("git", "clone")
	if !ok {
		t.Fatal("the repository was not cloned")
	}
	if !strings.Contains(clone.String(), "x-access-token:ghs_live_secret_token@github.com/acme/store.git") {
		t.Errorf("clone = %q, want the token in the git URL", clone.String())
	}

	stored, _ := service.Deployment(deployment.ID)
	if strings.Contains(stored.Log, "ghs_live_secret_token") {
		t.Errorf("the token reached the log:\n%s", stored.Log)
	}
	if !strings.Contains(stored.Log, "https://github.com/acme/store.git") {
		t.Errorf("log = %s, want the display URL", stored.Log)
	}
}

func TestRecoverRemovesAbandonedWorkspaces(t *testing.T) {
	service, _ := testService(t, Options{})

	orphan := filepath.Join(service.engine.workRoot, "orphan")
	if err := os.MkdirAll(orphan, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "leftover"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := service.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	entries, err := os.ReadDir(service.engine.workRoot)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("abandoned workspace survived: %v", entries)
	}
}

func TestDeploymentsLimitDefaults(t *testing.T) {
	service, _ := testService(t, Options{})

	item, err := service.Create(writeRequest())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 3; i++ {
		row := Deployment{
			ID: newID(), ProjectID: item.ID, ProjectName: item.Name,
			EnvironmentID: item.Environments[0].ID, Environment: "production",
			Status: StatusSucceeded, StartedAt: timeNow().Add(time.Duration(i) * time.Second),
		}
		if err := service.repo.CreateDeployment(&row); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	all, err := service.Deployments(item.ID, 0)
	if err != nil {
		t.Fatalf("default limit: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("default = %d, want 3", len(all))
	}

	capped, err := service.Deployments(item.ID, 1)
	if err != nil {
		t.Fatalf("limit 1: %v", err)
	}
	if len(capped) != 1 {
		t.Fatalf("capped = %d, want 1", len(capped))
	}
}

func TestLogsUnknownDeployment(t *testing.T) {
	service, _ := testService(t, Options{})
	if _, err := service.Logs("missing", 0); !errors.Is(err, ErrDeployNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrDeployNotFound)
	}
}

func TestEmitTruncatesTheLog(t *testing.T) {
	service, _ := testService(t, Options{})
	state := &run{}
	for i := 0; i < 5002; i++ {
		service.engine.emit(state, "line")
	}
	if len(state.lines) != 5000 {
		t.Fatalf("lines = %d, want 5000", len(state.lines))
	}
	if state.lines[4999] != "--> log truncated" {
		t.Errorf("last = %q", state.lines[4999])
	}
}

func TestUpdateRejectsATakenName(t *testing.T) {
	service, _ := testService(t, Options{})
	if _, err := service.Create(writeRequest()); err != nil {
		t.Fatalf("create: %v", err)
	}

	other := writeRequest()
	other.Name = "other"
	other.Environments[0].Domains[0].Host = "other.acme.dev"
	item, err := service.Create(other)
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	clash := writeRequest()
	clash.Environments[0].ID = item.Environments[0].ID
	clash.Environments[0].Domains[0].Host = "other.acme.dev"
	if _, err := service.Update(context.Background(), item.ID, clash); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("error = %v, want %v", err, ErrNameTaken)
	}
}

func TestCreateAcceptsBlankDomainRowsAndDefaults(t *testing.T) {
	service, _ := testService(t, Options{})

	req := writeRequest()
	req.Environments[0].Domains = append(req.Environments[0].Domains, DomainRequest{})
	req.Environments[0].Variables = []EnvVar{{Key: " ", Value: "skip"}, {Key: "NODE_ENV", Value: "production"}}
	req.Environments[0].Trigger = DeployTrigger{Kind: "nope", Pattern: " * "}
	req.Environments[0].Deploy.Strategy = "canary"
	req.Environments[0].Deploy.HealthGraceSec = 9000
	req.Environments[0].Domains[0].TLS = "mystery"
	req.Environments[0].Domains[0].Port = 0

	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	env := item.Environments[0]
	if len(env.Domains) != 1 {
		t.Fatalf("domains = %d, want the blank row dropped", len(env.Domains))
	}
	if env.Domains[0].Port != 80 {
		t.Errorf("port = %d, want 80", env.Domains[0].Port)
	}
	if env.Domains[0].TLS != TLSAuto {
		t.Errorf("tls = %q, want auto", env.Domains[0].TLS)
	}
	if env.Deploy.Strategy != StrategyRolling {
		t.Errorf("strategy = %q, want rolling", env.Deploy.Strategy)
	}
	if env.Deploy.HealthGraceSec != 30 {
		t.Errorf("health grace = %d, want 30", env.Deploy.HealthGraceSec)
	}
	if env.Trigger.Kind != TriggerManual {
		t.Errorf("trigger = %q, want manual", env.Trigger.Kind)
	}
	if len(env.Variables) != 1 || env.Variables[0].Key != "NODE_ENV" {
		t.Errorf("variables = %+v, want the blank key dropped", env.Variables)
	}
}

func TestCreateRejectsADuplicateHostInsideTheProject(t *testing.T) {
	service, _ := testService(t, Options{})

	req := writeRequest()
	req.Environments = append(req.Environments, EnvironmentRequest{
		Name:    "staging",
		Domains: []DomainRequest{{Host: "shop.acme.dev", Service: "web", Port: 3000}},
	})
	if _, err := service.Create(req); !errors.Is(err, ErrDomainTaken) {
		t.Fatalf("error = %v, want %v", err, ErrDomainTaken)
	}
}

func TestEnvMapSecretsWinAKeyCollision(t *testing.T) {
	got := envMap(Environment{
		Variables: []EnvVar{{Key: "TOKEN", Value: "public"}},
		Secrets:   []EnvVar{{Key: "TOKEN", Value: "secret"}, {Key: " ", Value: "skip"}},
	})
	if got["TOKEN"] != "secret" {
		t.Errorf("TOKEN = %q, want the secret", got["TOKEN"])
	}
	if _, ok := got[""]; ok {
		t.Error("a blank key leaked into the environment")
	}
}

func TestHandlePushMatchesRepositoryAndEffectiveBranch(t *testing.T) {
	service, _ := testService(t, Options{})
	req := writeRequest()
	req.SourceKind = SourceGit
	req.Compose = ""
	req.Git = GitSource{Repo: "https://github.com/Acme/Store.git", Branch: "main", ComposePath: "docker-compose.yml"}
	req.Environments[0].Trigger = DeployTrigger{Kind: TriggerPush}
	item, err := service.Create(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ignored, err := service.HandlePush("acme/other", "main", "ada", "abc123", "ignored")
	if err != nil || len(ignored) != 0 {
		t.Fatalf("repository mismatch = (%d, %v), want no deployments", len(ignored), err)
	}
	ignored, err = service.HandlePush("acme/store", "feature", "ada", "abc123", "ignored")
	if err != nil || len(ignored) != 0 {
		t.Fatalf("branch mismatch = (%d, %v), want no deployments", len(ignored), err)
	}

	queued, err := service.HandlePush("acme/store", "main", "ada", "abc123", "ship it")
	if err != nil {
		t.Fatalf("handle push: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("deployments = %d, want 1", len(queued))
	}
	deployment := queued[0]
	if deployment.ProjectID != item.ID || deployment.TriggeredBy != TriggerPush || deployment.Actor != "ada" || deployment.Revision != "abc123" || deployment.Message != "ship it" {
		t.Fatalf("deployment = %+v", deployment)
	}
	waitFor(t, "the push deployment to finish", func() bool {
		stored, err := service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})
}

func TestHandlePushUsesEnvironmentBranchAndSkipsManual(t *testing.T) {
	service, _ := testService(t, Options{})
	req := writeRequest()
	req.SourceKind = SourceGit
	req.Compose = ""
	req.Git = GitSource{Repo: "git@github.com:acme/store.git", Branch: "main", ComposePath: "docker-compose.yml"}
	req.Environments = []EnvironmentRequest{
		{Name: "staging", Branch: "develop", Trigger: DeployTrigger{Kind: TriggerPush}, Deploy: DeploySettings{Replicas: 1}},
		{Name: "production", Trigger: DeployTrigger{Kind: TriggerManual}, Deploy: DeploySettings{Replicas: 1}},
	}
	if _, err := service.Create(req); err != nil {
		t.Fatalf("create: %v", err)
	}

	queued, err := service.HandlePush("ACME/STORE", "develop", "ada", "abc123", "staging")
	if err != nil || len(queued) != 1 || queued[0].Environment != "staging" {
		t.Fatalf("deployments = %+v, error = %v", queued, err)
	}
	waitFor(t, "the staging deployment to finish", func() bool {
		stored, err := service.Deployment(queued[0].ID)
		return err == nil && stored.Status.Done()
	})
}

func TestHandleTagMatchesPattern(t *testing.T) {
	service, _ := testService(t, Options{})
	req := writeRequest()
	req.SourceKind = SourceGit
	req.Compose = ""
	req.Git = GitSource{Repo: "https://github.com/acme/store.git", Branch: "main", ComposePath: "docker-compose.yml"}
	req.Environments = []EnvironmentRequest{
		{Name: "production", Trigger: DeployTrigger{Kind: TriggerTag, Pattern: "v*"}, Deploy: DeploySettings{Replicas: 1}},
		{Name: "staging", Trigger: DeployTrigger{Kind: TriggerPush}, Deploy: DeploySettings{Replicas: 1}},
	}
	if _, err := service.Create(req); err != nil {
		t.Fatalf("create: %v", err)
	}

	ignored, err := service.HandleTag("acme/store", "nightly-2026-08-18", "ada", "abc123", "ignored")
	if err != nil || len(ignored) != 0 {
		t.Fatalf("pattern mismatch = (%d, %v), want no deployments", len(ignored), err)
	}

	queued, err := service.HandleTag("acme/store", "v1.2.0", "ada", "abc123", "release")
	if err != nil {
		t.Fatalf("handle tag: %v", err)
	}
	// The push environment must stay out of it: a tag is not a branch push.
	if len(queued) != 1 || queued[0].Environment != "production" || queued[0].TriggeredBy != TriggerTag {
		t.Fatalf("deployments = %+v", queued)
	}
	waitFor(t, "the tag deployment to finish", func() bool {
		stored, err := service.Deployment(queued[0].ID)
		return err == nil && stored.Status.Done()
	})
}

func TestCleanTriggerValidatesPatternsAndSource(t *testing.T) {
	cases := []struct {
		name    string
		trigger DeployTrigger
		source  SourceKind
		want    DeployTrigger
		wantErr error
	}{
		{
			name:    "a tag trigger with no pattern takes every tag",
			trigger: DeployTrigger{Kind: TriggerTag},
			source:  SourceGit,
			want:    DeployTrigger{Kind: TriggerTag, Pattern: "*"},
		},
		{
			name:    "a broken glob is refused",
			trigger: DeployTrigger{Kind: TriggerTag, Pattern: "v[1"},
			source:  SourceGit,
			wantErr: ErrTagPattern,
		},
		{
			name:    "a tag trigger needs a repository to watch",
			trigger: DeployTrigger{Kind: TriggerTag, Pattern: "v*"},
			source:  SourceCompose,
			wantErr: ErrTriggerSource,
		},
		{
			name:    "a schedule is kept with its cron expression",
			trigger: DeployTrigger{Kind: TriggerSchedule, Pattern: " 0 3 * * * "},
			source:  SourceCompose,
			want:    DeployTrigger{Kind: TriggerSchedule, Pattern: "0 3 * * *"},
		},
		{
			name:    "a cron expression that can never fire is refused",
			trigger: DeployTrigger{Kind: TriggerSchedule, Pattern: "every night"},
			source:  SourceGit,
			wantErr: ErrCronExpression,
		},
		{
			name:    "an unknown kind falls back to manual and drops the pattern",
			trigger: DeployTrigger{Kind: "whenever", Pattern: "v*"},
			source:  SourceGit,
			want:    DeployTrigger{Kind: TriggerManual},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cleanTrigger(tc.trigger, tc.source)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if !errIsValidation(err) {
					t.Error("the error should be reported to the user as a 400")
				}
				return
			}
			if err != nil {
				t.Fatalf("clean: %v", err)
			}
			if got != tc.want {
				t.Errorf("trigger = %+v, want %+v", got, tc.want)
			}
		})
	}
}
