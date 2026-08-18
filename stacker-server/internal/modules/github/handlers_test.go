package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCurrentNull(t *testing.T) {
	mod := testModule(t)
	rec := doJSON(t, testRouter(mod), http.MethodGet, "/api/github", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["data"] != nil {
		t.Fatalf("want data null, got %#v", got["data"])
	}
}

func TestCurrentReturnsApp(t *testing.T) {
	mod := testModule(t)
	if _, err := mod.Service.Start(CreateRequest{Name: "Stacker Home", BaseURL: "https://stacker.example"}); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, testRouter(mod), http.MethodGet, "/api/github", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Data App `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data.Name != "Stacker Home" {
		t.Fatalf("got %+v", got.Data)
	}
}

func TestStartAndRemoveHandlers(t *testing.T) {
	mod := testModule(t)
	router := testRouter(mod)

	badJSON := httptest.NewRequest(http.MethodPost, "/api/github/apps", strings.NewReader("{"))
	badJSON.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	router.ServeHTTP(badRec, badJSON)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("bind status %d", badRec.Code)
	}

	created := doJSON(t, router, http.MethodPost, "/api/github/apps", CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status %d body %s", created.Code, created.Body.String())
	}

	removed := doJSON(t, router, http.MethodDelete, "/api/github", nil)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("delete status %d", removed.Code)
	}
}

func TestWebhookHMAC(t *testing.T) {
	mod := testModule(t)
	router := testRouter(mod)

	missing := httptest.NewRequest(http.MethodPost, "/api/github/webhooks", strings.NewReader(`{"ok":true}`))
	missRec := httptest.NewRecorder()
	router.ServeHTTP(missRec, missing)
	if missRec.Code != http.StatusNotFound {
		t.Fatalf("no app: %d", missRec.Code)
	}

	app := seedApp(t, mod.Service, App{WebhookSecret: "hook-secret"})

	unsigned := httptest.NewRequest(http.MethodPost, "/api/github/webhooks", strings.NewReader(`{"ok":true}`))
	unsigned.Header.Set("X-Hub-Signature-256", "sha256=00")
	unRec := httptest.NewRecorder()
	router.ServeHTTP(unRec, unsigned)
	if unRec.Code != http.StatusUnauthorized {
		t.Fatalf("bad hmac: %d", unRec.Code)
	}

	noPrefix := httptest.NewRequest(http.MethodPost, "/api/github/webhooks", strings.NewReader(`{"ok":true}`))
	noPrefix.Header.Set("X-Hub-Signature-256", hex.EncodeToString([]byte("nope")))
	npRec := httptest.NewRecorder()
	router.ServeHTTP(npRec, noPrefix)
	if npRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing sha256 prefix: %d", npRec.Code)
	}

	body := []byte(`{"zen":"keep it simple"}`)
	mac := hmac.New(sha256.New, []byte(app.WebhookSecret))
	_, _ = mac.Write(body)
	ok := httptest.NewRequest(http.MethodPost, "/api/github/webhooks", strings.NewReader(string(body)))
	ok.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	okRec := httptest.NewRecorder()
	router.ServeHTTP(okRec, ok)
	if okRec.Code != http.StatusNoContent {
		t.Fatalf("valid hmac: %d", okRec.Code)
	}

	seedApp(t, mod.Service, App{WebhookSecret: ""})
	empty := httptest.NewRequest(http.MethodPost, "/api/github/webhooks", strings.NewReader(`{}`))
	emptyRec := httptest.NewRecorder()
	router.ServeHTTP(emptyRec, empty)
	if emptyRec.Code != http.StatusNotFound {
		t.Fatalf("empty webhook secret: %d", emptyRec.Code)
	}
}

func TestWebhookDispatchesVerifiedBranchPush(t *testing.T) {
	mod := testModule(t)
	app := seedApp(t, mod.Service, App{WebhookSecret: "hook-secret"})
	var got PushEvent
	mod.SetPushHandler(func(_ context.Context, event PushEvent) error { got = event; return nil })

	body := []byte(`{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"Acme/App"},"sender":{"login":"ada"},"head_commit":{"message":"ship it"}}`)
	rec := signedWebhook(t, testRouter(mod), app.WebhookSecret, "push", body)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	want := (PushEvent{Repository: "Acme/App", Branch: "main", Actor: "ada", Revision: "abc123", Message: "ship it"})
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestWebhookDispatchesTagPushes(t *testing.T) {
	mod := testModule(t)
	app := seedApp(t, mod.Service, App{WebhookSecret: "hook-secret"})
	var got PushEvent
	mod.SetPushHandler(func(_ context.Context, event PushEvent) error { got = event; return nil })

	body := []byte(`{"ref":"refs/tags/v1.0.0","after":"abc123","repository":{"full_name":"acme/app"},` +
		`"sender":{"login":"octocat"},"head_commit":{"message":"release"}}`)
	if rec := signedWebhook(t, testRouter(mod), app.WebhookSecret, "push", body); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}

	want := PushEvent{Repository: "acme/app", Tag: "v1.0.0", Actor: "octocat", Revision: "abc123", Message: "release"}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestWebhookIgnoresDeletionsAndOtherRefs(t *testing.T) {
	mod := testModule(t)
	app := seedApp(t, mod.Service, App{WebhookSecret: "hook-secret"})
	calls := 0
	mod.SetPushHandler(func(_ context.Context, _ PushEvent) error { calls++; return nil })

	for _, body := range [][]byte{
		[]byte(`{"ref":"refs/pull/7/head","repository":{"full_name":"acme/app"}}`),
		[]byte(`{"ref":"refs/heads/main","deleted":true,"repository":{"full_name":"acme/app"}}`),
		[]byte(`{"ref":"refs/tags/v1.0.0","deleted":true,"repository":{"full_name":"acme/app"}}`),
	} {
		if rec := signedWebhook(t, testRouter(mod), app.WebhookSecret, "push", body); rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0", calls)
	}
}

func TestWebhookReturnsServerErrorWhenDispatchFails(t *testing.T) {
	mod := testModule(t)
	app := seedApp(t, mod.Service, App{WebhookSecret: "hook-secret"})
	mod.SetPushHandler(func(_ context.Context, _ PushEvent) error { return errors.New("queue failed") })
	body := []byte(`{"ref":"refs/heads/main","repository":{"full_name":"acme/app"}}`)
	if rec := signedWebhook(t, testRouter(mod), app.WebhookSecret, "push", body); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func signedWebhook(t *testing.T, router http.Handler, secret, event string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/api/github/webhooks", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCallbackRedirects(t *testing.T) {
	pemKey := pkcs8PEM(t)
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/conversions") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 5, "slug": "stacker", "client_id": "cid", "client_secret": "csec",
			"webhook_secret": "wh", "pem": pemKey,
		})
	})

	mod := testModule(t)
	attachAPI(mod.Service, srv)
	app := seedApp(t, mod.Service, App{})
	router := testRouter(mod)

	fail := doJSON(t, router, http.MethodGet, "/api/github/callback/"+app.ID+"?token=wrong&code=x", nil)
	if fail.Code != http.StatusFound || !strings.Contains(fail.Header().Get("Location"), "github=error") {
		t.Fatalf("error redirect: %d %s", fail.Code, fail.Header().Get("Location"))
	}

	ok := doJSON(t, router, http.MethodGet, "/api/github/callback/"+app.ID+"?token="+app.CallbackSecret+"&code=ok", nil)
	if ok.Code != http.StatusFound || !strings.Contains(ok.Header().Get("Location"), "github=created") {
		t.Fatalf("created redirect: %d %s", ok.Code, ok.Header().Get("Location"))
	}
}

func TestInstallationCallbackRedirects(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account":              map[string]string{"login": "ada", "type": "User"},
			"repository_selection": "all",
		})
	})

	mod := testModule(t)
	attachAPI(mod.Service, srv)
	app := seedApp(t, mod.Service, App{AppID: 3, PrivateKey: pkcs8PEM(t)})
	router := testRouter(mod)

	badID := doJSON(t, router, http.MethodGet, "/api/github/installations/"+app.ID+"/callback?token="+app.CallbackSecret+"&installation_id=nope", nil)
	if badID.Code != http.StatusFound || !strings.Contains(badID.Header().Get("Location"), "github=error") {
		t.Fatalf("parse error: %d %s", badID.Code, badID.Header().Get("Location"))
	}

	badTok := doJSON(t, router, http.MethodGet, "/api/github/installations/"+app.ID+"/callback?token=wrong&installation_id=9", nil)
	if badTok.Code != http.StatusFound || !strings.Contains(badTok.Header().Get("Location"), "github=error") {
		t.Fatalf("service error: %d %s", badTok.Code, badTok.Header().Get("Location"))
	}

	ok := doJSON(t, router, http.MethodGet, "/api/github/installations/"+app.ID+"/callback?token="+app.CallbackSecret+"&installation_id=9", nil)
	if ok.Code != http.StatusFound || !strings.Contains(ok.Header().Get("Location"), "github=installed") {
		t.Fatalf("installed: %d %s", ok.Code, ok.Header().Get("Location"))
	}
}

func TestRespondErrorViaRegisterRoutes(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, *Module)
		method string
		path   string
		body   any
		status int
		err    string
	}{
		{
			name:   "not found",
			method: http.MethodGet,
			path:   "/api/github/repositories",
			status: http.StatusNotFound,
			err:    ErrNotFound.Error(),
		},
		{
			name:   "invalid name",
			method: http.MethodPost,
			path:   "/api/github/apps",
			body:   CreateRequest{Name: "bad!", BaseURL: "https://stacker.example"},
			status: http.StatusBadRequest,
			err:    ErrInvalidName.Error(),
		},
		{
			name:   "invalid base url",
			method: http.MethodPost,
			path:   "/api/github/apps",
			body:   CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example#frag"},
			status: http.StatusBadRequest,
			err:    ErrInvalidBaseURL.Error(),
		},
		{
			name: "not installed",
			setup: func(t *testing.T, mod *Module) {
				if _, err := mod.Service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example"}); err != nil {
					t.Fatal(err)
				}
			},
			method: http.MethodGet,
			path:   "/api/github/repositories",
			status: http.StatusBadRequest,
			err:    ErrNotInstalled.Error(),
		},
		{
			name: "github api",
			setup: func(t *testing.T, mod *Module) {
				srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"message": "Bad credentials"})
				})
				attachAPI(mod.Service, srv)
				seedApp(t, mod.Service, App{AppID: 1, InstallationID: 2, PrivateKey: pkcs8PEM(t)})
			},
			method: http.MethodGet,
			path:   "/api/github/repositories",
			status: http.StatusBadGateway,
			err:    "GitHub API: Bad credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod := testModule(t)
			if tt.setup != nil {
				tt.setup(t, mod)
			}
			rec := doJSON(t, testRouter(mod), tt.method, tt.path, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status %d want %d body %s", rec.Code, tt.status, rec.Body.String())
			}
			var got struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v body %s", err, rec.Body.String())
			}
			if got.Error != tt.err {
				t.Fatalf("error %q want %q", got.Error, tt.err)
			}
		})
	}
}

func TestRespondErrorTable(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{ErrNotFound, http.StatusNotFound},
		{ErrInvalidName, http.StatusBadRequest},
		{ErrInvalidBaseURL, http.StatusBadRequest},
		{ErrInvalidCallback, http.StatusBadRequest},
		{ErrNotInstalled, http.StatusBadRequest},
		{errorsFromGitHub("nope"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		engine := gin.New()
		engine.GET("/e", func(c *gin.Context) { respondError(c, tc.err) })
		req := httptest.NewRequest(http.MethodGet, "/e", nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%v: %d want %d", tc.err, rec.Code, tc.status)
		}
	}
}

func TestRepositoriesHandlerSuccess(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/access_tokens") {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "ghs_x"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"repositories": []any{}})
	})
	mod := testModule(t)
	attachAPI(mod.Service, srv)
	seedApp(t, mod.Service, App{AppID: 1, InstallationID: 2, PrivateKey: pkcs8PEM(t)})
	rec := doJSON(t, testRouter(mod), http.MethodGet, "/api/github/repositories", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestCurrentAndRemoveDBError(t *testing.T) {
	mod := testModule(t)
	closeDB(t, mod.Service.repo.db)
	router := testRouter(mod)

	cur := doJSON(t, router, http.MethodGet, "/api/github", nil)
	if cur.Code != http.StatusBadGateway {
		t.Fatalf("current: %d body %s", cur.Code, cur.Body.String())
	}
	del := doJSON(t, router, http.MethodDelete, "/api/github", nil)
	if del.Code != http.StatusBadGateway {
		t.Fatalf("delete: %d body %s", del.Code, del.Body.String())
	}
}
