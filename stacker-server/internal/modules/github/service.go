package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const githubAPI = "https://api.github.com"

var appNamePattern = regexp.MustCompile(`^[A-Za-z0-9 ._-]+$`)

type Service struct {
	repo   *RepositoryStore
	client *http.Client
	apiURL string
	log    *slog.Logger
}

func NewService(repo *RepositoryStore, log *slog.Logger) *Service {
	return &Service{repo: repo, client: &http.Client{Timeout: 15 * time.Second}, apiURL: githubAPI, log: log}
}

func (s *Service) Current() (App, error) { return s.repo.Current() }

func (s *Service) Start(req CreateRequest) (ManifestStart, error) {
	name := strings.TrimSpace(req.Name)
	if !appNamePattern.MatchString(name) {
		return ManifestStart{}, ErrInvalidName
	}
	base, err := url.Parse(strings.TrimRight(req.BaseURL, "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return ManifestStart{}, ErrInvalidBaseURL
	}
	base.Path = ""

	app := App{ID: randomHex(12), Name: name, CallbackSecret: randomHex(24)}
	if err := s.repo.Replace(&app); err != nil {
		return ManifestStart{}, err
	}

	callback := fmt.Sprintf("%s/api/github/callback/%s?token=%s", base.String(), app.ID, app.CallbackSecret)
	setup := fmt.Sprintf("%s/api/github/installations/%s/callback?token=%s", base.String(), app.ID, app.CallbackSecret)
	manifest := map[string]any{
		"name":                name,
		"url":                 base.String(),
		"redirect_url":        callback,
		"setup_url":           setup,
		"hook_attributes":     map[string]any{"url": base.String() + "/api/github/webhooks", "active": true},
		"public":              false,
		"default_permissions": map[string]string{"contents": "read", "metadata": "read", "pull_requests": "read"},
		"default_events":      []string{"push", "pull_request"},
	}

	manifestURL := "https://github.com/settings/apps/new"
	if org := strings.TrimSpace(req.Organization); org != "" {
		if !regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString(org) {
			return ManifestStart{}, ErrInvalidName
		}
		manifestURL = "https://github.com/organizations/" + url.PathEscape(org) + "/settings/apps/new"
	}
	return ManifestStart{URL: manifestURL, Manifest: manifest}, nil
}

func (s *Service) Convert(ctx context.Context, id, secret, code string) error {
	app, err := s.callbackApp(id, secret)
	if err != nil {
		return err
	}
	var result struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		WebhookSecret string `json:"webhook_secret"`
		PEM           string `json:"pem"`
	}
	if err := s.do(ctx, http.MethodPost, s.apiURL+"/app-manifests/"+url.PathEscape(code)+"/conversions", "", nil, &result); err != nil {
		return err
	}
	if result.ID == 0 || result.Slug == "" || result.PEM == "" {
		return errorsFromGitHub("conversion returned incomplete credentials")
	}
	app.AppID, app.Slug, app.ClientID = result.ID, result.Slug, result.ClientID
	app.ClientSecret, app.WebhookSecret, app.PrivateKey = result.ClientSecret, result.WebhookSecret, result.PEM
	return s.repo.Save(&app)
}

func (s *Service) CompleteInstallation(ctx context.Context, id, secret string, installationID int64) error {
	app, err := s.callbackApp(id, secret)
	if err != nil {
		return err
	}
	if app.AppID == 0 || installationID == 0 {
		return ErrInvalidCallback
	}
	token, err := s.appJWT(app)
	if err != nil {
		return err
	}
	var installation struct {
		Account             struct{ Login, Type string }
		RepositorySelection string `json:"repository_selection"`
	}
	endpoint := fmt.Sprintf("%s/app/installations/%d", s.apiURL, installationID)
	if err := s.do(ctx, http.MethodGet, endpoint, token, nil, &installation); err != nil {
		return err
	}
	app.InstallationID = installationID
	app.AccountLogin, app.AccountType, app.RepositoryMode = installation.Account.Login, installation.Account.Type, installation.RepositorySelection
	return s.repo.Save(&app)
}

func (s *Service) Repositories(ctx context.Context) ([]Repository, error) {
	app, err := s.repo.Current()
	if err != nil {
		return nil, err
	}
	if app.InstallationID == 0 {
		return nil, ErrNotInstalled
	}
	token, err := s.installationToken(ctx, app)
	if err != nil {
		return nil, err
	}
	var result struct {
		Repositories []struct {
			ID       int64
			FullName string `json:"full_name"`
			Private  bool
			HTMLURL  string `json:"html_url"`
		} `json:"repositories"`
	}
	if err := s.do(ctx, http.MethodGet, s.apiURL+"/installation/repositories?per_page=100", token, nil, &result); err != nil {
		return nil, err
	}
	repos := make([]Repository, 0, len(result.Repositories))
	for _, r := range result.Repositories {
		repos = append(repos, Repository{ID: r.ID, FullName: r.FullName, Private: r.Private, HTMLURL: r.HTMLURL})
	}
	return repos, nil
}

func (s *Service) Delete() error { return s.repo.Delete() }

func (s *Service) callbackApp(id, secret string) (App, error) {
	app, err := s.repo.Get(id)
	if err != nil || secret == "" || subtle.ConstantTimeCompare([]byte(subtleHash(secret)), []byte(subtleHash(app.CallbackSecret))) != 1 {
		return App{}, ErrInvalidCallback
	}
	return app, nil
}

func (s *Service) installationToken(ctx context.Context, app App) (string, error) {
	jwt, err := s.appJWT(app)
	if err != nil {
		return "", err
	}
	var result struct {
		Token string `json:"token"`
	}
	endpoint := fmt.Sprintf("%s/app/installations/%d/access_tokens", s.apiURL, app.InstallationID)
	if err := s.do(ctx, http.MethodPost, endpoint, jwt, nil, &result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", errorsFromGitHub("token response was empty")
	}
	return result.Token, nil
}

func (s *Service) appJWT(app App) (string, error) {
	block, _ := pem.Decode([]byte(app.PrivateKey))
	if block == nil {
		return "", errorsFromGitHub("invalid private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if rsaKey, e := x509.ParsePKCS1PrivateKey(block.Bytes); e == nil {
			key = rsaKey
			err = nil
		}
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if err != nil || !ok {
		return "", errorsFromGitHub("invalid RSA private key")
	}
	now := time.Now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": app.AppID})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *Service) do(ctx context.Context, method, endpoint, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("GitHub API: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct{ Message string }
		_ = json.Unmarshal(data, &e)
		if e.Message == "" {
			e.Message = resp.Status
		}
		return errorsFromGitHub(e.Message)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode GitHub response: %w", err)
		}
	}
	return nil
}

type githubError string

func (e githubError) Error() string         { return "GitHub API: " + string(e) }
func errorsFromGitHub(message string) error { return githubError(message) }
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func subtleHash(v string) string { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
