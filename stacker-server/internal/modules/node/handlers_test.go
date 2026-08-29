package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"stacker/internal/modules/sshkey"
)

func TestRespondError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		err  error
		want int
	}{
		{ErrNotFound, http.StatusNotFound},
		{ErrNoProvisionRun, http.StatusNotFound},
		{fmt.Errorf("%w: wrapped", ErrNotFound), http.StatusNotFound},
		{ErrNameTaken, http.StatusConflict},
		{ErrAlreadyInSwarm, http.StatusConflict},
		{ErrForeignSwarm, http.StatusConflict},
		{ErrNodeInSwarm, http.StatusConflict},
		{ErrSwarmBusy, http.StatusConflict},
		{ErrLocalNode, http.StatusForbidden},
		{ErrLocalSetupManaged, http.StatusForbidden},
		{ErrInvalidSsh, http.StatusBadRequest},
		{ErrSshKeyMissing, http.StatusBadRequest},
		{ErrNotWorker, http.StatusBadRequest},
		{ErrNotManagerRole, http.StatusBadRequest},
		{ErrNotInSwarm, http.StatusBadRequest},
		{ErrLastManager, http.StatusBadRequest},
		{ErrAdvertiseAddr, http.StatusBadRequest},
		{ErrPasswordRequired, http.StatusBadRequest},
		{ErrCopyIDMissing, http.StatusPreconditionFailed},
		{ErrDockerMissing, http.StatusPreconditionFailed},
		{ErrDockerNotRunning, http.StatusPreconditionFailed},
		{ErrNoManager, http.StatusPreconditionFailed},
		{ErrKeyNotVerified, http.StatusPreconditionFailed},
		{ErrUnsupportedOS, http.StatusPreconditionFailed},
		{ErrLocalNotLinux, http.StatusPreconditionFailed},
		{ErrSudoRequired, http.StatusPreconditionFailed},
		{ErrCurlMissing, http.StatusPreconditionFailed},
		{ErrDockerInstall, http.StatusPreconditionFailed},
		{ErrManagerUnhealthy, http.StatusBadGateway},
		{ErrSwarmUnreachable, http.StatusBadGateway},
		{ErrSwarmCommand, http.StatusBadGateway},
		{errors.New("unexpected"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			respondError(c, tc.err)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestRequireID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("empty", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Params = gin.Params{{Key: "id", Value: ""}}
		requireID()(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
		if !c.IsAborted() {
			t.Fatal("expected abort")
		}
	})

	t.Run("present", func(t *testing.T) {
		r := gin.New()
		r.GET("/n/:id", requireID(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/n/abc", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

func TestHTTPCRUDAndConfigure(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	r := testRouter(t, s)

	createBody := `{"name":"edge","ssh":"deploy@10.0.0.2","sshKeyId":"key1","port":22}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data Node `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created.Data.ID
	if id == "" {
		t.Fatal("missing id")
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nodes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nodes/"+id, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nodes/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing = %d", rec.Code)
	}

	updateBody := `{"name":"edge-2","ssh":"deploy@10.0.0.2","sshKeyId":"key1","port":22}`
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/nodes/"+id, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes", strings.NewReader(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create bind = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/nodes/"+id, strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update bind = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/nodes/install-key", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("install bind = %d", rec.Code)
	}

	item, err := s.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	item.KeyStatus = KeyStatusOK
	if err := s.repo.Save(&item); err != nil {
		t.Fatal(err)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+id+"/swarm/configure", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("configure = %d body=%s", rec.Code, rec.Body.String())
	}
	job := waitProvision(t, s, id)
	if job.State != ProvisionSucceeded {
		t.Fatalf("job = %+v", job)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nodes/"+id+"/swarm/configure", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("provision status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nodes/unknown/swarm/configure", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("provision status missing = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+LocalID+"/swarm/configure", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("configure local = %d", rec.Code)
	}
}

func TestHTTPPingAndSwarm(t *testing.T) {
	s := testService(t)
	fake := newRTFake()
	fake.attach(s)
	seedManager(t, s)
	fake.setRole(LocalID, SwarmRoleManager, "mgr1")
	stubProbe(t, true, "")
	r := testRouter(t, s)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+LocalID+"/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping local = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping all = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/refresh-swarm", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh all = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+LocalID+"/swarm/refresh", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh one = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+LocalID+"/check-key", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("check-key local = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes/install-key", strings.NewReader(`{"ssh":"bad","sshKeyId":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("install invalid ssh = %d", rec.Code)
	}

	worker := seedWorker(t, s, "w1", "edge")
	worker.SwarmRole = SwarmRoleWorker
	worker.SwarmNodeID = "wkr1"
	if err := s.repo.Save(&worker); err != nil {
		t.Fatal(err)
	}
	fake.setRole(worker.ID, SwarmRoleWorker, "wkr1")

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+worker.ID+"/swarm/promote", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("promote = %d body=%s", rec.Code, rec.Body.String())
	}

	// now two managers; demote the worker-turned-manager
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+worker.ID+"/swarm/demote", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("demote = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/"+worker.ID+"/swarm/leave", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("leave = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/nodes/"+worker.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/nodes/"+LocalID, nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete local = %d", rec.Code)
	}
}

func TestNewWiresModule(t *testing.T) {
	db := testDB(t)
	keys := sshkey.NewService(sshkey.NewRepository(db), t.TempDir(), silentLog())
	m := New(db, keys, "", false, silentLog())
	if m.Service == nil || m.handler == nil {
		t.Fatal("expected wired module")
	}
}

func TestHTTPHandlerErrors(t *testing.T) {
	s := testService(t)
	r := testRouter(t, s)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/missing/ping", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ping missing = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/missing/check-key", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("check-key missing = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/refresh-swarm", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh empty = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/nodes/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ping empty = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nodes", strings.NewReader(`{"name":"n","ssh":"bad","sshKeyId":"k"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create invalid ssh = %d", rec.Code)
	}
}
