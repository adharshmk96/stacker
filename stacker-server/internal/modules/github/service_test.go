package github

import (
	"io"
	"log/slog"
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
