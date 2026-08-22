package github

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&App{}); err != nil {
		t.Fatal(err)
	}
	return NewService(NewRepositoryStore(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestConnectExistingApp(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/app/installations/42" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account":              map[string]string{"login": "acme", "type": "Organization"},
			"repository_selection": "selected",
		})
	})
	service := testService(t)
	attachAPI(service, srv)
	app, err := service.ConnectExisting(context.Background(), ExistingAppRequest{
		Name: "Existing App", AppID: 7, InstallationID: 42,
		PrivateKey: pkcs8PEM(t), WebhookSecret: "webhook-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if app.AccountLogin != "acme" || app.RepositoryMode != "selected" {
		t.Fatalf("unexpected app: %+v", app)
	}
}

func TestConnectExistingRejectsInvalidCredentials(t *testing.T) {
	service := testService(t)
	if _, err := service.ConnectExisting(context.Background(), ExistingAppRequest{Name: "App"}); err != ErrInvalidExistingApp {
		t.Fatalf("got %v", err)
	}
}

func TestStartBuildsPersonalManifest(t *testing.T) {
	service := testService(t)
	result, err := service.Start(CreateRequest{Name: "Stacker Home", BaseURL: "https://stacker.example/"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://github.com/settings/apps/new" {
		t.Fatalf("unexpected URL %q", result.URL)
	}
	if result.Manifest["redirect_url"] == "" || result.Manifest["setup_url"] == "" {
		t.Fatal("callback URLs are required")
	}
	app, err := service.Current()
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "Stacker Home" || app.CallbackSecret == "" {
		t.Fatal("pending app was not persisted")
	}
}

func TestStartBuildsOrganizationManifest(t *testing.T) {
	service := testService(t)
	result, err := service.Start(CreateRequest{Name: "Stacker Org", BaseURL: "https://stacker.example", Organization: "acme-inc"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://github.com/organizations/acme-inc/settings/apps/new" {
		t.Fatalf("unexpected URL %q", result.URL)
	}
}

func TestStartRejectsUnsafeBaseURL(t *testing.T) {
	service := testService(t)
	for _, baseURL := range []string{"javascript:alert(1)", "https://user@stacker.example", "https://stacker.example?next=evil"} {
		if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: baseURL}); err != ErrInvalidBaseURL {
			t.Fatalf("baseURL %q: got %v", baseURL, err)
		}
	}
}

func TestCallbackSecretIsRequired(t *testing.T) {
	service := testService(t)
	if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example"}); err != nil {
		t.Fatal(err)
	}
	app, err := service.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.callbackApp(app.ID, "wrong"); err != ErrInvalidCallback {
		t.Fatalf("got %v", err)
	}
}
