package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"stacker/internal/modules/node"

	"github.com/gin-gonic/gin"
)

func testEngine(nodes *fakeNodes) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	New(nodes, slog.New(slog.NewTextHandler(io.Discard, nil))).RegisterRoutes(engine.Group("/api"))
	return engine
}

func request(engine *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestRespondError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		err  error
		code int
		body string
	}{
		{ErrUnknownResource, http.StatusNotFound, ErrUnknownResource.Error()},
		{ErrUnknownAction, http.StatusNotFound, ErrUnknownAction.Error()},
		{ErrNodeRequired, http.StatusBadRequest, ErrNodeRequired.Error()},
		{ErrUnknownNode, http.StatusBadRequest, ErrUnknownNode.Error()},
		{ErrNameRequired, http.StatusBadRequest, ErrNameRequired.Error()},
		{ErrImageRequired, http.StatusBadRequest, ErrImageRequired.Error()},
		{ErrContentRequired, http.StatusBadRequest, ErrContentRequired.Error()},
		{ErrReplicasNeeded, http.StatusBadRequest, ErrReplicasNeeded.Error()},
		{ErrGlobalService, http.StatusBadRequest, ErrGlobalService.Error()},
		{ErrNoManager, http.StatusPreconditionFailed, ErrNoManager.Error()},
		{node.ErrDockerMissing, http.StatusPreconditionFailed, node.ErrDockerMissing.Error()},
		{node.ErrDockerNotRunning, http.StatusPreconditionFailed, node.ErrDockerNotRunning.Error()},
		{node.ErrSwarmUnreachable, http.StatusBadGateway, node.ErrSwarmUnreachable.Error()},
		{fmt.Errorf("%w: connection refused", node.ErrSwarmCommand), http.StatusBadGateway, "connection refused"},
		{node.ErrSshKeyMissing, http.StatusBadGateway, node.ErrSshKeyMissing.Error()},
		{errors.New("boom"), http.StatusInternalServerError, "internal server error"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d/%s", tt.code, tt.body), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondError(c, tt.err)
			if w.Code != tt.code {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tt.code, w.Body.String())
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error != tt.body {
				t.Errorf("error = %q, want %q", body.Error, tt.body)
			}
		})
	}
}

func TestListRoute(t *testing.T) {
	engine := testEngine(&fakeNodes{
		roster: []node.Node{manager("local")},
		replies: map[string]string{
			"local service ls --format {{json .}}": `{"ID":"a1","Name":"web","Mode":"replicated","Replicas":"1/1","Image":"nginx","Ports":""}`,
		},
	})

	w := request(engine, http.MethodGet, "/api/swarm/services", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var body struct {
		Data ListResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data.Rows) != 1 || body.Data.Rows[0]["name"] != "web" {
		t.Fatalf("data = %+v", body.Data)
	}
}

func TestListRouteUnknownResource(t *testing.T) {
	w := request(testEngine(&fakeNodes{}), http.MethodGet, "/api/swarm/widgets", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestActionRoute(t *testing.T) {
	engine := testEngine(&fakeNodes{roster: []node.Node{manager("local")}})

	w := request(engine, http.MethodPost, "/api/swarm/services/action", `{"action":"remove","id":"web"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var body struct {
		Data ActionResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Message != "Removed the service web" {
		t.Errorf("message = %q", body.Data.Message)
	}
}

func TestActionRouteBindError(t *testing.T) {
	w := request(testEngine(&fakeNodes{}), http.MethodPost, "/api/swarm/services/action", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestActionRouteUnknownAction(t *testing.T) {
	engine := testEngine(&fakeNodes{roster: []node.Node{manager("local")}})
	w := request(engine, http.MethodPost, "/api/swarm/services/action", `{"action":"exec","id":"web"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestCreateRoute(t *testing.T) {
	engine := testEngine(&fakeNodes{roster: []node.Node{manager("local")}})

	w := request(engine, http.MethodPost, "/api/swarm/secrets", `{"name":"db_password","content":"hunter2"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var body struct {
		Data ActionResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Message != "Created the secret db_password" {
		t.Errorf("message = %q", body.Data.Message)
	}
}

func TestCreateRouteBindError(t *testing.T) {
	w := request(testEngine(&fakeNodes{}), http.MethodPost, "/api/swarm/secrets", `{`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestCreateRouteMissingImage(t *testing.T) {
	engine := testEngine(&fakeNodes{roster: []node.Node{manager("local")}})
	w := request(engine, http.MethodPost, "/api/swarm/services", `{"name":"web"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
}
