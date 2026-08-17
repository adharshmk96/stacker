package github

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStartRejectsInvalidName(t *testing.T) {
	service := testFileService(t)
	for _, name := range []string{"", "  ", "bad!", "has@sign"} {
		if _, err := service.Start(CreateRequest{Name: name, BaseURL: "https://stacker.example"}); err != ErrInvalidName {
			t.Fatalf("name %q: got %v", name, err)
		}
	}
}

func TestStartRejectsInvalidOrganization(t *testing.T) {
	service := testFileService(t)
	if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example", Organization: "acme_inc"}); err != ErrInvalidName {
		t.Fatalf("got %v", err)
	}
}

func TestStartRejectsFragmentURL(t *testing.T) {
	service := testFileService(t)
	if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example#next"}); err != ErrInvalidBaseURL {
		t.Fatalf("got %v", err)
	}
}

func TestCurrentNotFound(t *testing.T) {
	service := testFileService(t)
	if _, err := service.Current(); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestDeleteRemovesApp(t *testing.T) {
	service := testFileService(t)
	if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Current(); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestCloneTokenEmpty(t *testing.T) {
	service := testFileService(t)
	tok, err := service.CloneToken(context.Background())
	if err != nil || tok != "" {
		t.Fatalf("no app: got %q %v", tok, err)
	}
	if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example"}); err != nil {
		t.Fatal(err)
	}
	tok, err = service.CloneToken(context.Background())
	if err != nil || tok != "" {
		t.Fatalf("pending app: got %q %v", tok, err)
	}
}

func TestConvertPersistsCredentials(t *testing.T) {
	pemKey := pkcs8PEM(t)
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/conversions") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("conversion must not send a bearer token")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             42,
			"slug":           "stacker-home",
			"client_id":      "cid",
			"client_secret":  "csec",
			"webhook_secret": "whsec",
			"pem":            pemKey,
		})
	})

	service := testFileService(t)
	attachAPI(service, srv)
	if _, err := service.Start(CreateRequest{Name: "Stacker", BaseURL: "https://stacker.example"}); err != nil {
		t.Fatal(err)
	}
	app, err := service.Current()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Convert(context.Background(), app.ID, app.CallbackSecret, "manifest-code"); err != nil {
		t.Fatal(err)
	}
	got, err := service.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got.AppID != 42 || got.Slug != "stacker-home" || got.ClientID != "cid" || got.PrivateKey != pemKey || got.WebhookSecret != "whsec" {
		t.Fatalf("incomplete save: %+v", got)
	}
}

func TestConvertIncompleteCredentials(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "slug": "x"})
	})
	service := testFileService(t)
	attachAPI(service, srv)
	app := seedApp(t, service, App{})
	err := service.Convert(context.Background(), app.ID, app.CallbackSecret, "code")
	if err == nil || err.Error() != "GitHub API: conversion returned incomplete credentials" {
		t.Fatalf("got %v", err)
	}
}

func TestConvertInvalidCallback(t *testing.T) {
	service := testFileService(t)
	if err := service.Convert(context.Background(), "missing", "secret", "code"); err != ErrInvalidCallback {
		t.Fatalf("got %v", err)
	}
	app := seedApp(t, service, App{})
	if err := service.Convert(context.Background(), app.ID, "", "code"); err != ErrInvalidCallback {
		t.Fatalf("empty secret: got %v", err)
	}
}

func TestCompleteInstallationPersistsAccount(t *testing.T) {
	var sawBearer bool
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/app/installations/") {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing jwt")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		sawBearer = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"account":              map[string]string{"login": "acme", "type": "Organization"},
			"repository_selection": "selected",
		})
	})

	service := testFileService(t)
	attachAPI(service, srv)
	app := seedApp(t, service, App{AppID: 99, PrivateKey: pkcs8PEM(t)})
	if err := service.CompleteInstallation(context.Background(), app.ID, app.CallbackSecret, 7); err != nil {
		t.Fatal(err)
	}
	if !sawBearer {
		t.Fatal("installation fetch was not called")
	}
	got, err := service.Current()
	if err != nil {
		t.Fatal(err)
	}
	if got.InstallationID != 7 || got.AccountLogin != "acme" || got.AccountType != "Organization" || got.RepositoryMode != "selected" {
		t.Fatalf("got %+v", got)
	}
}

func TestCompleteInstallationRejectsUnconvertedApp(t *testing.T) {
	service := testFileService(t)
	app := seedApp(t, service, App{})
	if err := service.CompleteInstallation(context.Background(), app.ID, app.CallbackSecret, 7); err != ErrInvalidCallback {
		t.Fatalf("got %v", err)
	}
	if err := service.CompleteInstallation(context.Background(), app.ID, app.CallbackSecret, 0); err != ErrInvalidCallback {
		t.Fatalf("zero installation: got %v", err)
	}
}

func TestRepositoriesAndCloneToken(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/access_tokens"):
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "bad jwt"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "ghs_install"})
		case r.Method == http.MethodGet && r.URL.Path == "/installation/repositories":
			if r.Header.Get("Authorization") != "Bearer ghs_install" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repositories": []map[string]any{{
					"id":             11,
					"full_name":      "acme/api",
					"private":        true,
					"html_url":       "https://github.com/acme/api",
					"default_branch": "develop",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	})

	service := testFileService(t)
	attachAPI(service, srv)
	seedApp(t, service, App{AppID: 99, InstallationID: 7, PrivateKey: pkcs8PEM(t)})

	repos, err := service.Repositories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].FullName != "acme/api" || !repos[0].Private || repos[0].DefaultBranch != "develop" || repos[0].HTMLURL == "" {
		t.Fatalf("got %+v", repos)
	}

	tok, err := service.CloneToken(context.Background())
	if err != nil || tok != "ghs_install" {
		t.Fatalf("clone token %q %v", tok, err)
	}
}

func TestRepositoriesNotInstalled(t *testing.T) {
	service := testFileService(t)
	if _, err := service.Repositories(context.Background()); err != ErrNotFound {
		t.Fatalf("empty: got %v", err)
	}
	seedApp(t, service, App{})
	if _, err := service.Repositories(context.Background()); err != ErrNotInstalled {
		t.Fatalf("pending: got %v", err)
	}
}

func TestAppJWTPKCS1AndPKCS8(t *testing.T) {
	service := testFileService(t)

	pkcs8, err := service.appJWT(App{AppID: 1, PrivateKey: pkcs8PEM(t)})
	if err != nil {
		t.Fatal(err)
	}
	if parts := strings.Split(pkcs8, "."); len(parts) != 3 {
		t.Fatalf("pkcs8 jwt parts: %s", pkcs8)
	}

	pkcs1, err := service.appJWT(App{AppID: 1, PrivateKey: pkcs1PEM(t)})
	if err != nil {
		t.Fatal(err)
	}
	if parts := strings.Split(pkcs1, "."); len(parts) != 3 {
		t.Fatalf("pkcs1 jwt parts: %s", pkcs1)
	}
}

func TestAppJWTInvalidKeys(t *testing.T) {
	service := testFileService(t)
	if _, err := service.appJWT(App{}); err == nil || !strings.Contains(err.Error(), "invalid private key") {
		t.Fatalf("empty pem: got %v", err)
	}
	garbage := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("nope")}))
	if _, err := service.appJWT(App{PrivateKey: garbage}); err == nil || !strings.Contains(err.Error(), "invalid RSA private key") {
		t.Fatalf("garbage: got %v", err)
	}
	if _, err := service.appJWT(App{PrivateKey: ecPEM(t)}); err == nil || !strings.Contains(err.Error(), "invalid RSA private key") {
		t.Fatalf("ec: got %v", err)
	}
}

func TestDoErrorBodies(t *testing.T) {
	service := testFileService(t)

	t.Run("json message", func(t *testing.T) {
		srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
		})
		attachAPI(service, srv)
		err := service.do(context.Background(), http.MethodGet, srv.URL+"/fail", "tok", nil, nil)
		if err == nil || err.Error() != "GitHub API: Bad credentials" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("status fallback", func(t *testing.T) {
		srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{}`))
		})
		attachAPI(service, srv)
		err := service.do(context.Background(), http.MethodGet, srv.URL+"/fail", "", nil, nil)
		if err == nil || err.Error() != "GitHub API: 502 Bad Gateway" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		})
		attachAPI(service, srv)
		var out struct{ Token string }
		err := service.do(context.Background(), http.MethodGet, srv.URL+"/ok", "", nil, &out)
		if err == nil || !strings.Contains(err.Error(), "decode GitHub response") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("json body", func(t *testing.T) {
		srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"hello":"world"`) {
				t.Errorf("body %s", raw)
			}
			if r.Header.Get("Authorization") != "Bearer tok" {
				t.Errorf("auth %s", r.Header.Get("Authorization"))
			}
			if r.Header.Get("Accept") != "application/vnd.github+json" {
				t.Errorf("accept %s", r.Header.Get("Accept"))
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "ok"})
		})
		attachAPI(service, srv)
		var out struct{ Token string }
		err := service.do(context.Background(), http.MethodPost, srv.URL+"/echo", "tok", map[string]string{"hello": "world"}, &out)
		if err != nil || out.Token != "ok" {
			t.Fatalf("got %+v %v", out, err)
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		err := service.do(context.Background(), http.MethodPost, "http://example", "", make(chan int), nil)
		if err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("bad url", func(t *testing.T) {
		err := service.do(context.Background(), http.MethodGet, "://", "", nil, nil)
		if err == nil {
			t.Fatal("expected request error")
		}
	})

	t.Run("client error", func(t *testing.T) {
		srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {})
		attachAPI(service, srv)
		srv.Close()
		err := service.do(context.Background(), http.MethodGet, srv.URL+"/x", "", nil, nil)
		if err == nil || !strings.Contains(err.Error(), "GitHub API:") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("empty success body", func(t *testing.T) {
		srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		attachAPI(service, srv)
		var out struct{ Token string }
		if err := service.do(context.Background(), http.MethodGet, srv.URL+"/empty", "", nil, &out); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unreadable body", func(t *testing.T) {
		service.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(errReader{}),
				Header:     make(http.Header),
				Request:    r,
			}, nil
		})}
		err := service.do(context.Background(), http.MethodGet, "http://github.test/x", "", nil, nil)
		if err == nil || err.Error() != "read fail" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestInstallationTokenEmpty(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"token": ""})
	})
	service := testFileService(t)
	attachAPI(service, srv)
	_, err := service.installationToken(context.Background(), App{AppID: 1, InstallationID: 2, PrivateKey: pkcs8PEM(t)})
	if err == nil || err.Error() != "GitHub API: token response was empty" {
		t.Fatalf("got %v", err)
	}
}

func TestRepositoryGet(t *testing.T) {
	service := testFileService(t)
	if _, err := service.repo.Get("missing"); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
	app := seedApp(t, service, App{Name: "One"})
	got, err := service.repo.Get(app.ID)
	if err != nil || got.Name != "One" {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestCallbackAppWrongSecret(t *testing.T) {
	service := testFileService(t)
	app := seedApp(t, service, App{})
	if _, err := service.callbackApp(app.ID, "wrong"); err != ErrInvalidCallback {
		t.Fatalf("got %v", err)
	}
}

func TestConvertGitHubError(t *testing.T) {
	srv := githubServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	service := testFileService(t)
	attachAPI(service, srv)
	app := seedApp(t, service, App{})
	err := service.Convert(context.Background(), app.ID, app.CallbackSecret, "code")
	if err == nil || err.Error() != "GitHub API: boom" {
		t.Fatalf("got %v", err)
	}
}

func TestCompleteInstallationJWTError(t *testing.T) {
	service := testFileService(t)
	app := seedApp(t, service, App{AppID: 9, PrivateKey: "not-a-pem"})
	err := service.CompleteInstallation(context.Background(), app.ID, app.CallbackSecret, 1)
	if err == nil || !strings.Contains(err.Error(), "invalid private key") {
		t.Fatalf("got %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read fail") }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
