package web

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRegisterAPINotFound(t *testing.T) {
	r := gin.New()
	if err := Register(r); err != nil {
		t.Fatalf("Register: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("content-type = %q, want json", rec.Header().Get("Content-Type"))
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "not found" {
		t.Errorf("error = %q, want not found", body.Error)
	}
}

func TestRegisterRoot(t *testing.T) {
	r := gin.New()
	if err := Register(r); err != nil {
		t.Fatalf("Register: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	_, indexErr := fs.ReadFile(assets, "index.html")

	if indexErr != nil {
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503 when the UI is not embedded", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "not embedded") {
			t.Errorf("body = %q, want the unbuilt-UI notice", rec.Body.String())
		}
		return
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with an embedded UI", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("content-type = %q, want html", rec.Header().Get("Content-Type"))
	}
}

func TestRegisterServesFilesAndSPA(t *testing.T) {
	assets, err := fs.Sub(dist, "dist")
	if err != nil {
		t.Fatalf("sub: %v", err)
	}
	if _, err := fs.ReadFile(assets, "index.html"); err != nil {
		t.Skip("UI is not embedded; file-server and SPA fallback need index.html")
	}

	r := gin.New()
	if err := Register(r); err != nil {
		t.Fatalf("Register: %v", err)
	}

	fileRec := httptest.NewRecorder()
	r.ServeHTTP(fileRec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	if fileRec.Code != http.StatusOK {
		t.Errorf("robots.txt status = %d, want 200", fileRec.Code)
	}

	spaRec := httptest.NewRecorder()
	r.ServeHTTP(spaRec, httptest.NewRequest(http.MethodGet, "/dashboard/nodes", nil))
	if spaRec.Code != http.StatusOK {
		t.Errorf("spa status = %d, want 200", spaRec.Code)
	}
	if !strings.Contains(spaRec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("spa content-type = %q, want html", spaRec.Header().Get("Content-Type"))
	}

	assetRec := httptest.NewRecorder()
	r.ServeHTTP(assetRec, httptest.NewRequest(http.MethodGet, "/_nuxt/missing.js", nil))
	if assetRec.Code != http.StatusNotFound {
		t.Errorf("_nuxt status = %d, want 404", assetRec.Code)
	}
}

func TestExists(t *testing.T) {
	assets := fstest.MapFS{
		"robots.txt":  {Data: []byte("User-agent: *\n")},
		"dir/file.js": {Data: []byte("x")},
	}

	cases := []struct {
		path string
		want bool
	}{
		{"", false},
		{"/", false},
		{".", false},
		{"robots.txt", true},
		{"/robots.txt", true},
		{"missing", false},
		{"dir", false},
		{"/dir/", false},
		{"/dir/file.js", true},
	}

	for _, tc := range cases {
		if got := exists(assets, tc.path); got != tc.want {
			t.Errorf("exists(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
