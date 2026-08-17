package sshkey

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func testRouter(t *testing.T) (*gin.Engine, *Module) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, keyDir := testDB(t)
	mod := New(db, keyDir, silentLog())
	router := gin.New()
	mod.RegisterRoutes(router.Group("/api"))
	return router, mod
}

func doRequest(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRespondError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		err  error
		code int
		msg  string
	}{
		{"not found", ErrNotFound, http.StatusNotFound, ErrNotFound.Error()},
		{"wrapped not found", errors.Join(errors.New("wrap"), ErrNotFound), http.StatusNotFound, errors.Join(errors.New("wrap"), ErrNotFound).Error()},
		{"name taken", ErrNameTaken, http.StatusConflict, ErrNameTaken.Error()},
		{"in use", ErrKeyInUse, http.StatusConflict, ErrKeyInUse.Error()},
		{"default", ErrDefaultKey, http.StatusConflict, ErrDefaultKey.Error()},
		{"invalid name", ErrInvalidName, http.StatusBadRequest, ErrInvalidName.Error()},
		{"unknown type", ErrUnknownType, http.StatusBadRequest, ErrUnknownType.Error()},
		{"internal", errors.New("boom"), http.StatusInternalServerError, "internal server error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			respondError(c, tt.err)
			if rec.Code != tt.code {
				t.Fatalf("status = %d, want %d", rec.Code, tt.code)
			}
			var payload struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error != tt.msg {
				t.Fatalf("error = %q, want %q", payload.Error, tt.msg)
			}
		})
	}
}

func TestHandlersCRUD(t *testing.T) {
	router, mod := testRouter(t)

	list := doRequest(t, router, http.MethodGet, "/api/ssh-keys", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list empty status = %d body = %s", list.Code, list.Body.Bytes())
	}

	created := doRequest(t, router, http.MethodPost, "/api/ssh-keys", CreateRequest{Name: "ci", Type: KeyTypeEd25519})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", created.Code, created.Body.Bytes())
	}
	var createdBody struct {
		Data SshKey `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	key := createdBody.Data
	if key.ID == "" || key.Name != "ci" || key.PrivateKeyPath != "" {
		t.Fatalf("created payload leaked private path or missed fields: %+v", key)
	}

	got := doRequest(t, router, http.MethodGet, "/api/ssh-keys/"+key.ID, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", got.Code, got.Body.Bytes())
	}

	listed := doRequest(t, router, http.MethodGet, "/api/ssh-keys", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d", listed.Code)
	}

	dup := doRequest(t, router, http.MethodPost, "/api/ssh-keys", CreateRequest{Name: "ci", Type: KeyTypeEd25519})
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d body = %s", dup.Code, dup.Body.Bytes())
	}

	badName := doRequest(t, router, http.MethodPost, "/api/ssh-keys", CreateRequest{Name: "bad name", Type: KeyTypeEd25519})
	if badName.Code != http.StatusBadRequest {
		t.Fatalf("invalid name status = %d body = %s", badName.Code, badName.Body.Bytes())
	}

	bindFail := doRequest(t, router, http.MethodPost, "/api/ssh-keys", map[string]string{"name": "x"})
	if bindFail.Code != http.StatusBadRequest {
		t.Fatalf("bind fail status = %d body = %s", bindFail.Code, bindFail.Body.Bytes())
	}

	missing := doRequest(t, router, http.MethodGet, "/api/ssh-keys/does-not-exist", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("get missing status = %d", missing.Code)
	}

	def, err := mod.Service.EnsureDefault()
	if err != nil {
		t.Fatal(err)
	}

	rotateDefault := doRequest(t, router, http.MethodPost, "/api/ssh-keys/"+def.ID+"/rotate", nil)
	if rotateDefault.Code != http.StatusOK {
		t.Fatalf("rotate default status = %d body = %s", rotateDefault.Code, rotateDefault.Body.Bytes())
	}

	rotateOther := doRequest(t, router, http.MethodPost, "/api/ssh-keys/"+key.ID+"/rotate", nil)
	if rotateOther.Code != http.StatusConflict {
		t.Fatalf("rotate non-default status = %d body = %s", rotateOther.Code, rotateOther.Body.Bytes())
	}

	deleteDefault := doRequest(t, router, http.MethodDelete, "/api/ssh-keys/"+def.ID, nil)
	if deleteDefault.Code != http.StatusConflict {
		t.Fatalf("delete default status = %d body = %s", deleteDefault.Code, deleteDefault.Body.Bytes())
	}

	removed := doRequest(t, router, http.MethodDelete, "/api/ssh-keys/"+key.ID, nil)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body = %s", removed.Code, removed.Body.Bytes())
	}

	deleteMissing := doRequest(t, router, http.MethodDelete, "/api/ssh-keys/"+key.ID, nil)
	if deleteMissing.Code != http.StatusNotFound {
		t.Fatalf("delete missing status = %d", deleteMissing.Code)
	}

	rotateMissing := doRequest(t, router, http.MethodPost, "/api/ssh-keys/missing/rotate", nil)
	if rotateMissing.Code != http.StatusNotFound {
		t.Fatalf("rotate missing status = %d", rotateMissing.Code)
	}
}

func TestHandlersCreateRSAAndInUseDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, keyDir := testDB(t)
	mod := New(db, keyDir, silentLog())
	router := gin.New()
	mod.RegisterRoutes(router.Group("/api"))

	created := doRequest(t, router, http.MethodPost, "/api/ssh-keys", CreateRequest{Name: "rsa-key", Type: KeyTypeRSA})
	if created.Code != http.StatusCreated {
		t.Fatalf("create rsa status = %d body = %s", created.Code, created.Body.Bytes())
	}
	var body struct {
		Data SshKey `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&nodeRef{ID: "n1", SshKeyID: body.Data.ID}).Error; err != nil {
		t.Fatal(err)
	}

	inUse := doRequest(t, router, http.MethodDelete, "/api/ssh-keys/"+body.Data.ID, nil)
	if inUse.Code != http.StatusConflict {
		t.Fatalf("delete in-use status = %d body = %s", inUse.Code, inUse.Body.Bytes())
	}
}
