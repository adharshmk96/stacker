package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stacker/internal/modules/auth"

	"github.com/gin-gonic/gin"
)

func testModuleHTTP(t *testing.T, username string) (*gin.Engine, *Module, *recorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	opts := Options{
		WorkRoot:           filepath.Join(t.TempDir(), "work"),
		TraefikDynamicPath: filepath.Join(t.TempDir(), "stacker.yml"),
		Network:            "stacker_proxy",
	}
	if err := os.MkdirAll(opts.WorkRoot, 0o700); err != nil {
		t.Fatalf("workroot: %v", err)
	}

	m := New(testDB(t), opts, silentLog())
	rec := &recorder{}
	m.Service.engine.exec = rec.exec
	m.Service.status.exec = rec.exec

	engine := gin.New()
	if username != "" {
		engine.Use(func(c *gin.Context) {
			c.Set("auth.user", auth.User{Username: username})
			c.Next()
		})
	}
	m.RegisterRoutes(engine.Group(""))
	return engine, m, rec
}

func doJSON(t *testing.T, engine http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func decodeData[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	return envelope.Data
}

func TestProjectHTTPCRUD(t *testing.T) {
	engine, _, _ := testModuleHTTP(t, "")

	listed := doJSON(t, engine, http.MethodGet, "/projects", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list empty: %d %s", listed.Code, listed.Body.String())
	}
	if items := decodeData[[]Project](t, listed); len(items) != 0 {
		t.Fatalf("list = %d, want 0", len(items))
	}

	created := doJSON(t, engine, http.MethodPost, "/projects", writeRequest())
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	item := decodeData[Project](t, created)
	if item.ID == "" || item.Name != "storefront" {
		t.Fatalf("created = %+v", item)
	}

	got := doJSON(t, engine, http.MethodGet, "/projects/"+item.ID, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}
	if decodeData[Project](t, got).ID != item.ID {
		t.Fatal("get returned a different project")
	}

	update := writeRequest()
	update.Description = "the shop"
	update.Environments[0].ID = item.Environments[0].ID
	updated := doJSON(t, engine, http.MethodPut, "/projects/"+item.ID, update)
	if updated.Code != http.StatusOK {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	if decodeData[Project](t, updated).Description != "the shop" {
		t.Fatal("description was not saved")
	}

	all := doJSON(t, engine, http.MethodGet, "/projects", nil)
	if len(decodeData[[]Project](t, all)) != 1 {
		t.Fatal("list after create is not the new project")
	}

	removed := doJSON(t, engine, http.MethodDelete, "/projects/"+item.ID, nil)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", removed.Code, removed.Body.String())
	}

	missing := doJSON(t, engine, http.MethodGet, "/projects/"+item.ID, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("get after delete: %d, want 404", missing.Code)
	}
}

func TestStatusAllIsNotCapturedAsProjectID(t *testing.T) {
	engine, _, rec := testModuleHTTP(t, "")
	rec.reply = func(Command) ([]string, error) {
		return []string{`{"Name":"x_web","Replicas":"1/1"}`}, nil
	}

	created := doJSON(t, engine, http.MethodPost, "/projects", writeRequest())
	item := decodeData[Project](t, created)

	all := doJSON(t, engine, http.MethodGet, "/projects/status", nil)
	if all.Code == http.StatusNotFound {
		t.Fatal("/projects/status was captured as :id")
	}
	if all.Code != http.StatusOK {
		t.Fatalf("status all: %d %s", all.Code, all.Body.String())
	}
	statuses := decodeData[[]ProjectStatus](t, all)
	if len(statuses) != 1 || statuses[0].ProjectID != item.ID {
		t.Fatalf("status all = %+v", statuses)
	}

	one := doJSON(t, engine, http.MethodGet, "/projects/"+item.ID+"/status", nil)
	if one.Code != http.StatusOK {
		t.Fatalf("status: %d %s", one.Code, one.Body.String())
	}
	status := decodeData[ProjectStatus](t, one)
	if status.ProjectID != item.ID || len(status.Environments) != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestProjectHTTPDeployStopLogsAndCancel(t *testing.T) {
	engine, m, rec := testModuleHTTP(t, "adharsh")

	created := doJSON(t, engine, http.MethodPost, "/projects", writeRequest())
	item := decodeData[Project](t, created)
	envID := item.Environments[0].ID
	stack := StackName(item, item.Environments[0])

	queued := doJSON(t, engine, http.MethodPost, "/projects/"+item.ID+"/environments/"+envID+"/deploy", nil)
	if queued.Code != http.StatusAccepted {
		t.Fatalf("deploy: %d %s", queued.Code, queued.Body.String())
	}
	deployment := decodeData[Deployment](t, queued)
	if deployment.Actor != "adharsh" {
		t.Errorf("actor = %q, want the signed-in user", deployment.Actor)
	}
	if deployment.Status != StatusQueued {
		t.Errorf("status = %q, want queued", deployment.Status)
	}

	waitFor(t, "the run to finish", func() bool {
		stored, err := m.Service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	listed := doJSON(t, engine, http.MethodGet, "/deployments?projectId="+item.ID+"&limit=10", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("deployments: %d %s", listed.Code, listed.Body.String())
	}
	runs := decodeData[[]Deployment](t, listed)
	if len(runs) != 1 || runs[0].ID != deployment.ID {
		t.Fatalf("deployments = %+v", runs)
	}

	one := doJSON(t, engine, http.MethodGet, "/deployments/"+deployment.ID, nil)
	if one.Code != http.StatusOK || decodeData[Deployment](t, one).ID != deployment.ID {
		t.Fatalf("deployment: %d %s", one.Code, one.Body.String())
	}

	logs := doJSON(t, engine, http.MethodGet, "/deployments/"+deployment.ID+"/logs?after=0", nil)
	if logs.Code != http.StatusOK {
		t.Fatalf("logs: %d %s", logs.Code, logs.Body.String())
	}
	chunk := decodeData[LogChunk](t, logs)
	if !chunk.Done || len(chunk.Lines) == 0 {
		t.Fatalf("logs = %+v, want a finished run", chunk)
	}

	cancel := doJSON(t, engine, http.MethodPost, "/deployments/"+deployment.ID+"/cancel", nil)
	if cancel.Code != http.StatusPreconditionFailed {
		t.Fatalf("cancel finished: %d, want 412 %s", cancel.Code, cancel.Body.String())
	}

	stopped := doJSON(t, engine, http.MethodPost, "/projects/"+item.ID+"/environments/"+envID+"/stop", nil)
	if stopped.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", stopped.Code, stopped.Body.String())
	}
	if _, ok := rec.find("stack", "rm", stack); !ok {
		t.Error("stop did not remove the stack")
	}
}

func TestDeployHTTPUsesNamedActorOverAuthUser(t *testing.T) {
	engine, m, rec := testModuleHTTP(t, "from-auth")

	release := make(chan struct{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack deploy") {
			<-release
		}
		return nil, nil
	}

	created := doJSON(t, engine, http.MethodPost, "/projects", writeRequest())
	item := decodeData[Project](t, created)

	queued := doJSON(t, engine, http.MethodPost,
		"/projects/"+item.ID+"/environments/"+item.Environments[0].ID+"/deploy",
		DeployRequest{Actor: "from-body", Message: "hotfix"})
	if queued.Code != http.StatusAccepted {
		t.Fatalf("deploy: %d %s", queued.Code, queued.Body.String())
	}
	deployment := decodeData[Deployment](t, queued)
	if deployment.Actor != "from-body" {
		t.Errorf("actor = %q, want the payload to win", deployment.Actor)
	}

	waitFor(t, "the run to reach deploy", func() bool {
		_, ok := rec.find("stack", "deploy")
		return ok
	})

	live := doJSON(t, engine, http.MethodGet, "/deployments/"+deployment.ID+"/logs", nil)
	close(release)
	waitFor(t, "the run to finish", func() bool {
		stored, err := m.Service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})

	if live.Code != http.StatusOK {
		t.Fatalf("live logs: %d %s", live.Code, live.Body.String())
	}
	chunk := decodeData[LogChunk](t, live)
	if chunk.Done {
		t.Error("live logs reported the run as done")
	}
	if len(chunk.Lines) == 0 {
		t.Error("live logs were empty")
	}
}

func TestCancelHTTPStopsALiveRun(t *testing.T) {
	engine, m, rec := testModuleHTTP(t, "")

	release := make(chan struct{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack deploy") {
			<-release
			return nil, context.Canceled
		}
		return nil, nil
	}

	created := doJSON(t, engine, http.MethodPost, "/projects", writeRequest())
	item := decodeData[Project](t, created)

	queued := doJSON(t, engine, http.MethodPost,
		"/projects/"+item.ID+"/environments/"+item.Environments[0].ID+"/deploy", nil)
	deployment := decodeData[Deployment](t, queued)

	waitFor(t, "the run to reach deploy", func() bool {
		_, ok := rec.find("stack", "deploy")
		return ok
	})

	cancel := doJSON(t, engine, http.MethodPost, "/deployments/"+deployment.ID+"/cancel", nil)
	close(release)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", cancel.Code, cancel.Body.String())
	}

	waitFor(t, "the run to cancel", func() bool {
		stored, err := m.Service.Deployment(deployment.ID)
		return err == nil && stored.Status.Done()
	})
	stored, _ := m.Service.Deployment(deployment.ID)
	if stored.Status != StatusCancelled {
		t.Errorf("status = %q, want cancelled", stored.Status)
	}
}

func TestProjectHTTPRespondError(t *testing.T) {
	engine, m, rec := testModuleHTTP(t, "")

	if rec := doJSON(t, engine, http.MethodGet, "/projects/missing", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing project: %d, want 404", rec.Code)
	}
	if rec := doJSON(t, engine, http.MethodGet, "/deployments/missing", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing deployment: %d, want 404", rec.Code)
	}
	if rec := doJSON(t, engine, http.MethodGet, "/projects/missing/status", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing status: %d, want 404", rec.Code)
	}

	created := doJSON(t, engine, http.MethodPost, "/projects", writeRequest())
	item := decodeData[Project](t, created)

	stopMissingEnv := doJSON(t, engine, http.MethodPost, "/projects/"+item.ID+"/environments/nope/stop", nil)
	if stopMissingEnv.Code != http.StatusNotFound {
		t.Errorf("missing env stop: %d, want 404", stopMissingEnv.Code)
	}

	dupName := writeRequest()
	dupName.Environments[0].Domains[0].Host = "other.acme.dev"
	if rec := doJSON(t, engine, http.MethodPost, "/projects", dupName); rec.Code != http.StatusConflict {
		t.Errorf("name taken: %d, want 409 %s", rec.Code, rec.Body.String())
	}

	dupHost := writeRequest()
	dupHost.Name = "other"
	if rec := doJSON(t, engine, http.MethodPost, "/projects", dupHost); rec.Code != http.StatusConflict {
		t.Errorf("domain taken: %d, want 409 %s", rec.Code, rec.Body.String())
	}

	badName := writeRequest()
	badName.Name = "my project"
	if rec := doJSON(t, engine, http.MethodPost, "/projects", badName); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid name: %d, want 400 %s", rec.Code, rec.Body.String())
	}

	bind := doJSON(t, engine, http.MethodPost, "/projects", map[string]any{"name": "x"})
	if bind.Code != http.StatusBadRequest {
		t.Errorf("bind create: %d, want 400", bind.Code)
	}
	putBind := httptest.NewRequest(http.MethodPut, "/projects/"+item.ID, strings.NewReader("{"))
	putBind.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	engine.ServeHTTP(putRec, putBind)
	if putRec.Code != http.StatusBadRequest {
		t.Errorf("bind update: %d, want 400", putRec.Code)
	}

	deployBind := httptest.NewRequest(http.MethodPost,
		"/projects/"+item.ID+"/environments/"+item.Environments[0].ID+"/deploy",
		strings.NewReader("{"))
	deployBind.Header.Set("Content-Type", "application/json")
	deployRec := httptest.NewRecorder()
	engine.ServeHTTP(deployRec, deployBind)
	if deployRec.Code != http.StatusBadRequest {
		t.Errorf("bind deploy: %d, want 400", deployRec.Code)
	}

	release := make(chan struct{})
	rec.reply = func(cmd Command) ([]string, error) {
		if strings.Contains(cmd.String(), "stack deploy") {
			<-release
		}
		return nil, nil
	}
	first := doJSON(t, engine, http.MethodPost,
		"/projects/"+item.ID+"/environments/"+item.Environments[0].ID+"/deploy", nil)
	waitFor(t, "the first run to reach deploy", func() bool {
		_, ok := rec.find("stack", "deploy")
		return ok
	})
	second := doJSON(t, engine, http.MethodPost,
		"/projects/"+item.ID+"/environments/"+item.Environments[0].ID+"/deploy", nil)
	close(release)
	waitFor(t, "the first run to finish", func() bool {
		items, err := m.Service.Deployments(item.ID, 10)
		return err == nil && len(items) == 1 && items[0].Status.Done()
	})
	if first.Code != http.StatusAccepted {
		t.Errorf("first deploy: %d, want 202", first.Code)
	}
	if second.Code != http.StatusConflict {
		t.Errorf("second deploy: %d, want 409 %s", second.Code, second.Body.String())
	}

	if rec := doJSON(t, engine, http.MethodPost, "/deployments/missing/cancel", nil); rec.Code != http.StatusPreconditionFailed {
		t.Errorf("cancel unknown: %d, want 412", rec.Code)
	}

	updateMissing := doJSON(t, engine, http.MethodPut, "/projects/missing", writeRequest())
	if updateMissing.Code != http.StatusNotFound {
		t.Errorf("update missing: %d, want 404", updateMissing.Code)
	}
	deleteMissing := doJSON(t, engine, http.MethodDelete, "/projects/missing", nil)
	if deleteMissing.Code != http.StatusNotFound {
		t.Errorf("delete missing: %d, want 404", deleteMissing.Code)
	}
}

func TestRespondErrorStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		err  error
		code int
		msg  string
	}{
		{ErrNotFound, http.StatusNotFound, "project not found"},
		{ErrEnvNotFound, http.StatusNotFound, "environment not found"},
		{ErrDeployNotFound, http.StatusNotFound, "deployment not found"},
		{ErrNameTaken, http.StatusConflict, "already exists"},
		{ErrDomainTaken, http.StatusConflict, "already routed"},
		{ErrAlreadyDeploying, http.StatusConflict, "already running"},
		{ErrNotRunning, http.StatusPreconditionFailed, "already finished"},
		{ErrTraefikMissing, http.StatusPreconditionFailed, "traefik"},
		{ErrInvalidName, http.StatusBadRequest, "letters"},
		{ErrBranchRequired, http.StatusBadRequest, "branch"},
		{ErrComposePath, http.StatusBadRequest, "path"},
		{ErrComposeRequired, http.StatusBadRequest, "paste"},
		{ErrEnvRequired, http.StatusBadRequest, "at least one"},
		{ErrDomainPort, http.StatusBadRequest, "port"},
		{ErrUnknownService, http.StatusBadRequest, "no such service"},
		{errors.New("disk full"), http.StatusInternalServerError, "internal server error"},
	}

	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			respondError(c, tc.err)
			if rec.Code != tc.code {
				t.Errorf("status = %d, want %d", rec.Code, tc.code)
			}
			if !strings.Contains(rec.Body.String(), tc.msg) {
				t.Errorf("body = %s, want %q", rec.Body.String(), tc.msg)
			}
		})
	}
}

func TestListHTTPReturns500WhenDatabaseIsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := testDB(t)
	opts := Options{
		WorkRoot:           t.TempDir(),
		TraefikDynamicPath: filepath.Join(t.TempDir(), "stacker.yml"),
		Network:            "stacker_proxy",
	}
	m := New(db, opts, silentLog())
	engine := gin.New()
	m.RegisterRoutes(engine.Group(""))

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	listed := doJSON(t, engine, http.MethodGet, "/projects", nil)
	if listed.Code != http.StatusInternalServerError {
		t.Fatalf("list: %d, want 500 %s", listed.Code, listed.Body.String())
	}

	statuses := doJSON(t, engine, http.MethodGet, "/projects/status", nil)
	if statuses.Code != http.StatusInternalServerError {
		t.Fatalf("status all: %d, want 500", statuses.Code)
	}
}
