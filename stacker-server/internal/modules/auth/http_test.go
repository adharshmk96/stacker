package auth

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRespondErrorTable(t *testing.T) {
	tests := []struct {
		err  error
		code int
		body string
	}{
		{ErrUnauthorized, http.StatusUnauthorized, ErrUnauthorized.Error()},
		{ErrInvalidCredentials, http.StatusUnauthorized, ErrInvalidCredentials.Error()},
		{ErrUserNotFound, http.StatusNotFound, ErrUserNotFound.Error()},
		{ErrSessionNotFound, http.StatusNotFound, ErrSessionNotFound.Error()},
		{ErrAlreadyRegistered, http.StatusConflict, ErrAlreadyRegistered.Error()},
		{ErrEmailTaken, http.StatusConflict, ErrEmailTaken.Error()},
		{ErrUsernameTaken, http.StatusConflict, ErrUsernameTaken.Error()},
		{ErrInvalidUsername, http.StatusBadRequest, ErrInvalidUsername.Error()},
		{ErrWeakPassword, http.StatusBadRequest, ErrWeakPassword.Error()},
		{ErrInvalidResetToken, http.StatusBadRequest, ErrInvalidResetToken.Error()},
		{errors.New("boom"), http.StatusInternalServerError, "internal server error"},
		{errors.Join(ErrUnauthorized), http.StatusUnauthorized, ErrUnauthorized.Error()},
	}

	for _, test := range tests {
		t.Run(test.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			respondError(ctx, test.err)
			if rec.Code != test.code {
				t.Fatalf("status = %d, want %d", rec.Code, test.code)
			}
			var payload struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if payload.Error != test.body {
				t.Fatalf("error = %q, want %q", payload.Error, test.body)
			}
		})
	}
}

func TestAppOrigin(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*http.Request)
		want  string
	}{
		{
			name:  "origin header wins",
			setup: func(r *http.Request) { r.Header.Set("Origin", "https://ui.example") },
			want:  "https://ui.example",
		},
		{
			name: "forwarded https",
			setup: func(r *http.Request) {
				r.Host = "stacker.local"
				r.Header.Set("X-Forwarded-Proto", "https")
			},
			want: "https://stacker.local",
		},
		{
			name:  "plain http",
			setup: func(r *http.Request) { r.Host = "localhost:8080" },
			want:  "http://localhost:8080",
		},
		{
			name: "tls",
			setup: func(r *http.Request) {
				r.Host = "secure.local"
				r.TLS = &tls.ConnectionState{}
			},
			want: "https://secure.local",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			test.setup(req)
			ctx.Request = req
			if got := appOrigin(ctx); got != test.want {
				t.Fatalf("origin = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "empty", header: "", want: ""},
		{name: "short", header: "Bear", want: ""},
		{name: "no bearer", header: "Token abc", want: ""},
		{name: "bearer missing value", header: "Bearer", want: ""},
		{name: "bearer space only", header: "Bearer   ", want: ""},
		{name: "bearer", header: "Bearer tok-123", want: "tok-123"},
		{name: "prefix case", header: "bearer tok-123", want: "tok-123"},
		{name: "trims", header: "Bearer   tok-123  ", want: "tok-123"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				req.Header.Set("Authorization", test.header)
			}
			ctx.Request = req
			if got := bearerToken(ctx); got != test.want {
				t.Fatalf("token = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCurrentUserAndSession(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	if _, ok := CurrentUser(ctx); ok {
		t.Fatal("expected no user")
	}
	if _, ok := CurrentSession(ctx); ok {
		t.Fatal("expected no session")
	}

	ctx.Set(contextUser, "not-a-user")
	ctx.Set(contextSession, "not-a-session")
	if _, ok := CurrentUser(ctx); ok {
		t.Fatal("wrong type should miss")
	}
	if _, ok := CurrentSession(ctx); ok {
		t.Fatal("wrong type should miss")
	}

	wantUser := User{ID: "u1"}
	wantSession := Session{ID: "s1"}
	ctx.Set(contextUser, wantUser)
	ctx.Set(contextSession, wantSession)
	user, ok := CurrentUser(ctx)
	if !ok || user.ID != "u1" {
		t.Fatalf("user = %+v ok=%v", user, ok)
	}
	session, ok := CurrentSession(ctx)
	if !ok || session.ID != "s1" {
		t.Fatalf("session = %+v ok=%v", session, ok)
	}
}

func TestRequireAuthAndPublicPrivateRoutes(t *testing.T) {
	mod, _ := testModule(t, nil)
	engine := testRouter(mod)

	status := doJSON(t, engine, http.MethodGet, "/api/auth/status", "", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("public status = %d, body=%s", status.Code, status.Body)
	}
	if decodeData[Status](t, status).Registered {
		t.Fatal("expected unregistered")
	}

	me := doJSON(t, engine, http.MethodGet, "/api/auth/me", "", nil)
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("private me without token = %d, want 401", me.Code)
	}

	me = doJSON(t, engine, http.MethodGet, "/api/auth/me", "not-a-jwt", nil)
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("private me with bad token = %d, want 401", me.Code)
	}

	created := doJSON(t, engine, http.MethodPost, "/api/auth/register", "", sampleRegister())
	if created.Code != http.StatusCreated {
		t.Fatalf("register = %d, body=%s", created.Code, created.Body)
	}
	result := decodeData[LoginResult](t, created)

	me = doJSON(t, engine, http.MethodGet, "/api/auth/me", result.Token, nil)
	if me.Code != http.StatusOK {
		t.Fatalf("private me with token = %d, body=%s", me.Code, me.Body)
	}
	if decodeData[User](t, me).ID != result.User.ID {
		t.Fatal("me returned a different user")
	}

	conflict := doJSON(t, engine, http.MethodPost, "/api/auth/register", "", sampleRegister())
	if conflict.Code != http.StatusConflict {
		t.Fatalf("second register = %d, want 409", conflict.Code)
	}

	login := doJSON(t, engine, http.MethodPost, "/api/auth/login", "", LoginRequest{
		Identifier: "ada", Password: "wrong-password",
	})
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", login.Code)
	}
}

func TestHTTPBindFailures(t *testing.T) {
	mod, _ := testModule(t, nil)
	engine := testRouter(mod)
	result := mustRegister(t, mod.Service)

	paths := []struct {
		method string
		path   string
		token  string
		body   string
	}{
		{http.MethodPost, "/api/auth/register", "", `{}`},
		{http.MethodPost, "/api/auth/register", "", `{`},
		{http.MethodPost, "/api/auth/login", "", `{}`},
		{http.MethodPost, "/api/auth/forgot-password", "", `{}`},
		{http.MethodPost, "/api/auth/reset-password", "", `{}`},
		{http.MethodPut, "/api/auth/profile", result.Token, `{}`},
		{http.MethodPost, "/api/auth/change-password", result.Token, `{}`},
		{http.MethodPost, "/api/auth/register", "", `{"name":"Ada","username":"ada","email":"not-an-email","password":"password1"}`},
		{http.MethodPost, "/api/auth/register", "", `{"name":"Ada","username":"ada","email":"ada@example.com","password":"short"}`},
	}

	for _, test := range paths {
		rec := doRaw(t, engine, test.method, test.path, test.token, test.body, http.Header{
			"Content-Type": []string{"application/json"},
		})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want 400 body=%s", test.method, test.path, rec.Code, rec.Body)
		}
	}
}

func TestHTTPAccountFlow(t *testing.T) {
	mod, db := testModule(t, nil)
	engine := testRouter(mod)
	result := mustRegister(t, mod.Service)

	status := doJSON(t, engine, http.MethodGet, "/api/auth/status", "", nil)
	if !decodeData[Status](t, status).Registered {
		t.Fatal("status after register")
	}

	login := doJSON(t, engine, http.MethodPost, "/api/auth/login", "", LoginRequest{
		Identifier: "ADA@example.com", Password: "password1",
	})
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d, body=%s", login.Code, login.Body)
	}
	second := decodeData[LoginResult](t, login)

	// Username that passes gin binding (min=2) but fails the service pattern.
	badName := doJSON(t, engine, http.MethodPut, "/api/auth/profile", result.Token, UpdateProfileRequest{
		Name: "Ada", Username: "no spaces!", Email: "ada@example.com",
	})
	if badName.Code != http.StatusBadRequest {
		t.Fatalf("invalid username = %d, want 400", badName.Code)
	}

	insertUser(t, db, User{Name: "Other", Username: "other", Email: "other@example.com"})
	taken := doJSON(t, engine, http.MethodPut, "/api/auth/profile", result.Token, UpdateProfileRequest{
		Name: "Ada", Username: "ada", Email: "other@example.com",
	})
	if taken.Code != http.StatusConflict {
		t.Fatalf("email taken = %d, want 409", taken.Code)
	}
	taken = doJSON(t, engine, http.MethodPut, "/api/auth/profile", result.Token, UpdateProfileRequest{
		Name: "Ada", Username: "other", Email: "ada@example.com",
	})
	if taken.Code != http.StatusConflict {
		t.Fatalf("username taken = %d, want 409", taken.Code)
	}

	updated := doJSON(t, engine, http.MethodPut, "/api/auth/profile", result.Token, UpdateProfileRequest{
		Name: "Ada Byron", Username: "ada", Email: "ada@example.com",
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("profile = %d, body=%s", updated.Code, updated.Body)
	}

	wrongPass := doJSON(t, engine, http.MethodPost, "/api/auth/change-password", result.Token, ChangePasswordRequest{
		CurrentPassword: "nope", NewPassword: "password2",
	})
	if wrongPass.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", wrongPass.Code)
	}
	changed := doJSON(t, engine, http.MethodPost, "/api/auth/change-password", result.Token, ChangePasswordRequest{
		CurrentPassword: "password1", NewPassword: "password2",
	})
	if changed.Code != http.StatusOK {
		t.Fatalf("change password = %d, body=%s", changed.Code, changed.Body)
	}

	sessions := doJSON(t, engine, http.MethodGet, "/api/auth/sessions", result.Token, nil)
	if sessions.Code != http.StatusOK {
		t.Fatalf("sessions = %d, body=%s", sessions.Code, sessions.Body)
	}
	items := decodeData[[]struct {
		ID      string `json:"id"`
		Current bool   `json:"current"`
	}](t, sessions)
	if len(items) != 1 || !items[0].Current || items[0].ID != result.SessionID {
		t.Fatalf("sessions after password change = %+v", items)
	}
	if _, _, err := mod.Service.Authenticate(second.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("other session survived a password change")
	}

	extra, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password2"}, "other", "2.2.2.2")
	if err != nil {
		t.Fatalf("extra login: %v", err)
	}
	revoke := doJSON(t, engine, http.MethodDelete, "/api/auth/sessions/"+extra.SessionID, result.Token, nil)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d, body=%s", revoke.Code, revoke.Body)
	}
	missing := doJSON(t, engine, http.MethodDelete, "/api/auth/sessions/"+extra.SessionID, result.Token, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("revoke missing = %d, want 404", missing.Code)
	}

	logout := doJSON(t, engine, http.MethodPost, "/api/auth/logout", result.Token, nil)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout = %d, body=%s", logout.Code, logout.Body)
	}
	if doJSON(t, engine, http.MethodGet, "/api/auth/me", result.Token, nil).Code != http.StatusUnauthorized {
		t.Fatal("token still worked after logout")
	}
}

func TestHTTPForgotAndReset(t *testing.T) {
	mod, db := testModule(t, nil)
	engine := testRouter(mod)
	first := mustRegister(t, mod.Service)

	unknown := doJSON(t, engine, http.MethodPost, "/api/auth/forgot-password", "", ForgotPasswordRequest{Email: "nobody@example.com"})
	if unknown.Code != http.StatusOK {
		t.Fatalf("unknown forgot = %d, body=%s", unknown.Code, unknown.Body)
	}

	header := http.Header{"Origin": []string{"https://ui.example"}, "Content-Type": []string{"application/json"}}
	known := doRaw(t, engine, http.MethodPost, "/api/auth/forgot-password", "", `{"email":"ada@example.com"}`, header)
	if known.Code != http.StatusOK {
		t.Fatalf("known forgot = %d, body=%s", known.Code, known.Body)
	}

	var resets []PasswordReset
	if err := db.Find(&resets).Error; err != nil {
		t.Fatalf("list resets: %v", err)
	}
	if len(resets) != 1 || resets[0].UserID != first.User.ID {
		t.Fatalf("resets = %+v", resets)
	}

	bad := doJSON(t, engine, http.MethodPost, "/api/auth/reset-password", "", ResetPasswordRequest{
		Token: "nope", Password: "password9",
	})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad reset = %d, want 400", bad.Code)
	}

	token := newResetToken()
	if err := mod.Service.repo.CreateReset(&PasswordReset{
		TokenHash: hashToken(token),
		UserID:    first.User.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create reset: %v", err)
	}
	ok := doJSON(t, engine, http.MethodPost, "/api/auth/reset-password", "", ResetPasswordRequest{
		Token: token, Password: "password9",
	})
	if ok.Code != http.StatusOK {
		t.Fatalf("reset = %d, body=%s", ok.Code, ok.Body)
	}
}

func TestHTTPResetData(t *testing.T) {
	mod, db := testModule(t, nil)
	mod.Service.resetData = func() error { return wipeAuth(db) }
	engine := testRouter(mod)
	result := mustRegister(t, mod.Service)

	reset := doJSON(t, engine, http.MethodPost, "/api/auth/reset-data", result.Token, nil)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset-data = %d, body=%s", reset.Code, reset.Body)
	}
	status := doJSON(t, engine, http.MethodGet, "/api/auth/status", "", nil)
	if decodeData[Status](t, status).Registered {
		t.Fatal("account survived http reset-data")
	}

	failing, _ := testModule(t, func() error { return errors.New("wipe failed") })
	failEngine := testRouter(failing)
	failResult := mustRegister(t, failing.Service)
	rec := doJSON(t, failEngine, http.MethodPost, "/api/auth/reset-data", failResult.Token, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("reset-data error = %d, want 500", rec.Code)
	}
}

func TestHTTPHandlerDatabaseErrors(t *testing.T) {
	mod, db := testModule(t, nil)
	engine := testRouter(mod)
	result := mustRegister(t, mod.Service)

	closeDB(t, db)

	if doJSON(t, engine, http.MethodGet, "/api/auth/status", "", nil).Code != http.StatusInternalServerError {
		t.Fatal("status should fail closed db")
	}
	if doJSON(t, engine, http.MethodPost, "/api/auth/forgot-password", "", ForgotPasswordRequest{Email: "ada@example.com"}).Code != http.StatusInternalServerError {
		t.Fatal("forgot should fail closed db")
	}
	if doJSON(t, engine, http.MethodGet, "/api/auth/sessions", result.Token, nil).Code != http.StatusUnauthorized {
		t.Fatal("sessions after close should be unauthorized: authenticate fails first")
	}
}

func TestHTTPRegisterInvalidUsername(t *testing.T) {
	mod, _ := testModule(t, nil)
	engine := testRouter(mod)

	rec := doJSON(t, engine, http.MethodPost, "/api/auth/register", "", RegisterRequest{
		Name: "Ada", Username: "no spaces!", Email: "ada@example.com", Password: "password1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), ErrInvalidUsername.Error()) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogoutIgnoresAlreadyRevoked(t *testing.T) {
	mod, _ := testModule(t, nil)
	handler := NewHandler(mod.Service)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(contextSession, Session{ID: "missing", UserID: "missing"})
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)

	handler.logout(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout missing session = %d, want 200 body=%s", rec.Code, rec.Body)
	}

	closed, db := testModule(t, nil)
	closeDB(t, db)
	failRec := httptest.NewRecorder()
	failCtx, _ := gin.CreateTestContext(failRec)
	failCtx.Set(contextSession, Session{ID: "s", UserID: "u"})
	failCtx.Request = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	NewHandler(closed.Service).logout(failCtx)
	if failRec.Code != http.StatusInternalServerError {
		t.Fatalf("logout closed db = %d, want 500", failRec.Code)
	}
}
