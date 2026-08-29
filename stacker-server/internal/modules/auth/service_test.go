package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStatusBeforeAndAfterRegister(t *testing.T) {
	mod, _ := testModule(t, nil)

	status, err := mod.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Registered {
		t.Fatal("expected no account yet")
	}

	mustRegister(t, mod.Service)

	status, err = mod.Service.Status()
	if err != nil {
		t.Fatalf("status after register: %v", err)
	}
	if !status.Registered {
		t.Fatal("expected an account after register")
	}
}

func TestRegisterLoginAndCredentials(t *testing.T) {
	mod, _ := testModule(t, nil)
	agent := strings.Repeat("a", 500)
	ip := strings.Repeat("9", 100)

	result, err := mod.Service.Register(sampleRegister(), agent, ip)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.Token == "" || result.SessionID == "" {
		t.Fatal("register did not return a session")
	}
	if result.User.Email != "ada@example.com" {
		t.Fatalf("email = %q, want normalized ada@example.com", result.User.Email)
	}
	if result.User.PasswordHash == "" {
		t.Fatal("register did not store a password hash")
	}

	session, err := mod.Service.repo.GetSession(result.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if len(session.UserAgent) != 400 {
		t.Fatalf("user agent length = %d, want 400", len(session.UserAgent))
	}
	if len(session.IP) != 80 {
		t.Fatalf("ip length = %d, want 80", len(session.IP))
	}

	user, caught, err := mod.Service.Authenticate(result.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if user.ID != result.User.ID || caught.ID != result.SessionID {
		t.Fatal("authenticate returned the wrong user or session")
	}

	for _, identifier := range []string{"ada@example.com", "ADA@EXAMPLE.COM", "ada"} {
		login, err := mod.Service.Login(LoginRequest{Identifier: identifier, Password: "password1"}, "ua", "1.1.1.1")
		if err != nil {
			t.Fatalf("login %q: %v", identifier, err)
		}
		if login.User.ID != result.User.ID {
			t.Fatalf("login %q returned a different user", identifier)
		}
	}

	_, err = mod.Service.Login(LoginRequest{Identifier: "ada", Password: "wrong-password"}, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want %v", err, ErrInvalidCredentials)
	}

	_, err = mod.Service.Login(LoginRequest{Identifier: "missing@example.com", Password: "password1"}, "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown account error = %v, want %v", err, ErrInvalidCredentials)
	}
}

func TestRegisterRejectsSecondAccountAndBadUsername(t *testing.T) {
	mod, _ := testModule(t, nil)
	mustRegister(t, mod.Service)

	_, err := mod.Service.Register(RegisterRequest{
		Name: "Second", Username: "second", Email: "second@example.com", Password: "password1",
	}, "", "")
	if !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("second register error = %v, want %v", err, ErrAlreadyRegistered)
	}

	fresh, _ := testModule(t, nil)
	for _, username := range []string{"bad name", "ada!", "", "  "} {
		_, err := fresh.Service.Register(RegisterRequest{
			Name: "Ada", Username: username, Email: "ada@example.com", Password: "password1",
		}, "", "")
		if !errors.Is(err, ErrInvalidUsername) {
			t.Fatalf("username %q error = %v, want %v", username, err, ErrInvalidUsername)
		}
	}
}

func TestSessionsRevokeAndLogout(t *testing.T) {
	mod, _ := testModule(t, nil)
	first := mustRegister(t, mod.Service)
	second, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password1"}, "other", "2.2.2.2")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	sessions, err := mod.Service.Sessions(first.User.ID)
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}

	if err := mod.Service.RevokeSession(first.User.ID, second.SessionID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(second.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked token error = %v, want %v", err, ErrUnauthorized)
	}

	sessions, err = mod.Service.Sessions(first.User.ID)
	if err != nil {
		t.Fatalf("sessions after revoke: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != first.SessionID {
		t.Fatalf("remaining session = %+v, want only %s", sessions, first.SessionID)
	}

	if err := mod.Service.RevokeSession(first.User.ID, second.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoke missing error = %v, want %v", err, ErrSessionNotFound)
	}
	if err := mod.Service.RevokeSession("other-user", first.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoke other user error = %v, want %v", err, ErrSessionNotFound)
	}

	current, err := mod.Service.repo.GetSession(first.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if err := mod.Service.Logout(current); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(first.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("logout left the session active")
	}
	if err := mod.Service.Logout(current); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("second logout error = %v, want %v", err, ErrSessionNotFound)
	}
}

func TestAuthenticateRejectsBadAndStaleSessions(t *testing.T) {
	mod, db := testModule(t, nil)
	result := mustRegister(t, mod.Service)

	if _, _, err := mod.Service.Authenticate("not-a-jwt"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad jwt error = %v, want %v", err, ErrUnauthorized)
	}
	if _, _, err := mod.Service.Authenticate(""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("empty token error = %v, want %v", err, ErrUnauthorized)
	}

	missing, err := signToken(mod.Service.secret, claims{
		SessionID: "missing",
		UserID:    result.User.ID,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign missing: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(missing); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("missing session error = %v, want %v", err, ErrUnauthorized)
	}

	wrongUser, err := signToken(mod.Service.secret, claims{
		SessionID: result.SessionID,
		UserID:    "somebody-else",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign wrong user: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(wrongUser); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("user mismatch error = %v, want %v", err, ErrUnauthorized)
	}

	if err := db.Model(&Session{}).Where("id = ?", result.SessionID).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expire session: %v", err)
	}
	stillValid, err := signToken(mod.Service.secret, claims{
		SessionID: result.SessionID,
		UserID:    result.User.ID,
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign still-valid jwt: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(stillValid); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expired session error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestAuthenticateTouchesAndMissingUser(t *testing.T) {
	mod, db := testModule(t, nil)
	result := mustRegister(t, mod.Service)

	stale := time.Now().Add(-2 * time.Minute)
	if err := db.Model(&Session{}).Where("id = ?", result.SessionID).Update("last_seen_at", stale).Error; err != nil {
		t.Fatalf("stale last seen: %v", err)
	}

	if _, _, err := mod.Service.Authenticate(result.Token); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	stored, err := mod.Service.repo.GetSession(result.SessionID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if !stored.LastSeenAt.After(stale) {
		t.Fatalf("database last_seen_at was not updated: %v", stored.LastSeenAt)
	}

	if err := db.Where("id = ?", result.User.ID).Delete(&User{}).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(result.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("missing user error = %v, want %v", err, ErrUnauthorized)
	}
}

func TestUpdateProfileAndCollisions(t *testing.T) {
	mod, db := testModule(t, nil)
	result := mustRegister(t, mod.Service)

	updated, err := mod.Service.UpdateProfile(result.User.ID, UpdateProfileRequest{
		Name: " Ada ", Username: "ada", Email: "ADA@Example.com",
	})
	if err != nil {
		t.Fatalf("same identity: %v", err)
	}
	if updated.Name != "Ada" || updated.Username != "ada" || updated.Email != "ada@example.com" {
		t.Fatalf("updated = %+v", updated)
	}

	renamed, err := mod.Service.UpdateProfile(result.User.ID, UpdateProfileRequest{
		Name: "Countess", Username: "countess", Email: "countess@example.com",
	})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Username != "countess" || renamed.Email != "countess@example.com" {
		t.Fatalf("rename = %+v", renamed)
	}

	insertUser(t, db, User{Name: "Other", Username: "other", Email: "other@example.com"})

	_, err = mod.Service.UpdateProfile(result.User.ID, UpdateProfileRequest{
		Name: "Countess", Username: "countess", Email: "other@example.com",
	})
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("email taken error = %v, want %v", err, ErrEmailTaken)
	}

	_, err = mod.Service.UpdateProfile(result.User.ID, UpdateProfileRequest{
		Name: "Countess", Username: "other", Email: "countess@example.com",
	})
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("username taken error = %v, want %v", err, ErrUsernameTaken)
	}

	_, err = mod.Service.UpdateProfile(result.User.ID, UpdateProfileRequest{
		Name: "Countess", Username: "bad name", Email: "countess@example.com",
	})
	if !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("invalid username error = %v, want %v", err, ErrInvalidUsername)
	}

	_, err = mod.Service.UpdateProfile("missing", UpdateProfileRequest{
		Name: "X", Username: "xx", Email: "x@example.com",
	})
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestChangePasswordKeepsCurrentSession(t *testing.T) {
	mod, _ := testModule(t, nil)
	first := mustRegister(t, mod.Service)
	other, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password1"}, "other", "2.2.2.2")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if err := mod.Service.ChangePassword(first.User.ID, first.SessionID, ChangePasswordRequest{
		CurrentPassword: "wrong",
		NewPassword:     "password2",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong current error = %v, want %v", err, ErrInvalidCredentials)
	}

	if err := mod.Service.ChangePassword(first.User.ID, first.SessionID, ChangePasswordRequest{
		CurrentPassword: "password1",
		NewPassword:     "password2",
	}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	if _, _, err := mod.Service.Authenticate(first.Token); err != nil {
		t.Fatalf("current session dropped: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(other.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("other session error = %v, want %v", err, ErrUnauthorized)
	}

	if _, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password1"}, "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("old password still worked")
	}
	if _, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password2"}, "", ""); err != nil {
		t.Fatalf("new password: %v", err)
	}

	if err := mod.Service.ChangePassword("missing", "", ChangePasswordRequest{
		CurrentPassword: "password2", NewPassword: "password3",
	}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestForgotAndResetPassword(t *testing.T) {
	mod, db := testModule(t, nil)
	first := mustRegister(t, mod.Service)
	other, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password1"}, "other", "2.2.2.2")
	if err != nil {
		t.Fatalf("second login: %v", err)
	}

	if err := mod.Service.ForgotPassword(ForgotPasswordRequest{Email: "nobody@example.com"}, "https://stacker.local"); err != nil {
		t.Fatalf("unknown email: %v", err)
	}

	if err := mod.Service.ForgotPassword(ForgotPasswordRequest{Email: " ADA@Example.com "}, "https://stacker.local/"); err != nil {
		t.Fatalf("forgot: %v", err)
	}

	var resets []PasswordReset
	if err := db.Find(&resets).Error; err != nil {
		t.Fatalf("list resets: %v", err)
	}
	if len(resets) != 1 {
		t.Fatalf("resets = %d, want 1", len(resets))
	}

	token := newResetToken()
	if err := mod.Service.repo.CreateReset(&PasswordReset{
		TokenHash: hashToken(token),
		UserID:    first.User.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create reset: %v", err)
	}

	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: " " + token + " ", Password: "password9"}); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, _, err := mod.Service.Authenticate(first.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("reset left sessions alive")
	}
	if _, _, err := mod.Service.Authenticate(other.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("reset left the other session alive")
	}
	if _, err := mod.Service.Login(LoginRequest{Identifier: "ada", Password: "password9"}, "", ""); err != nil {
		t.Fatalf("login after reset: %v", err)
	}

	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: token, Password: "password8"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("reuse error = %v, want %v", err, ErrInvalidResetToken)
	}
	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: "missing", Password: "password8"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("unknown token error = %v, want %v", err, ErrInvalidResetToken)
	}

	expired := newResetToken()
	if err := mod.Service.repo.CreateReset(&PasswordReset{
		TokenHash: hashToken(expired),
		UserID:    first.User.ID,
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("expired reset: %v", err)
	}
	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: expired, Password: "password8"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("expired error = %v, want %v", err, ErrInvalidResetToken)
	}

	orphan := newResetToken()
	if err := mod.Service.repo.CreateReset(&PasswordReset{
		TokenHash: hashToken(orphan),
		UserID:    "ghost",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("orphan reset: %v", err)
	}
	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: orphan, Password: "password8"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("orphan error = %v, want %v", err, ErrInvalidResetToken)
	}
}

func TestResetPasswordRejectsAlreadyUsed(t *testing.T) {
	mod, _ := testModule(t, nil)
	first := mustRegister(t, mod.Service)

	usedAt := time.Now()
	token := newResetToken()
	if err := mod.Service.repo.CreateReset(&PasswordReset{
		TokenHash: hashToken(token),
		UserID:    first.User.ID,
		ExpiresAt: time.Now().Add(time.Hour),
		UsedAt:    &usedAt,
	}); err != nil {
		t.Fatalf("used reset: %v", err)
	}
	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: token, Password: "password8"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("used token error = %v, want %v", err, ErrInvalidResetToken)
	}
}

func TestChangePasswordInvalidatesOutstandingResets(t *testing.T) {
	mod, _ := testModule(t, nil)
	first := mustRegister(t, mod.Service)

	token := newResetToken()
	if err := mod.Service.repo.CreateReset(&PasswordReset{
		TokenHash: hashToken(token),
		UserID:    first.User.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create reset: %v", err)
	}

	if err := mod.Service.ChangePassword(first.User.ID, first.SessionID, ChangePasswordRequest{
		CurrentPassword: "password1",
		NewPassword:     "password2",
	}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	if err := mod.Service.ResetPassword(ResetPasswordRequest{Token: token, Password: "password8"}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("reset after password change error = %v, want %v", err, ErrInvalidResetToken)
	}
}

func TestResetAllData(t *testing.T) {
	wiped := false
	mod, db := testModule(t, nil)
	mod.Service.resetData = func() error {
		wiped = true
		return wipeAuth(db)
	}
	first := mustRegister(t, mod.Service)
	oldSecret := string(mod.Service.secret)

	if err := mod.Service.ResetAllData(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !wiped {
		t.Fatal("resetData was not called")
	}
	if _, _, err := mod.Service.Authenticate(first.Token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("old token still verified")
	}

	status, err := mod.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Registered {
		t.Fatal("account survived the wipe")
	}
	if string(mod.Service.secret) == oldSecret {
		t.Fatal("signing secret was not rotated")
	}

	if _, err := mod.Service.Register(sampleRegister(), "", ""); err != nil {
		t.Fatalf("register after reset: %v", err)
	}
}

func TestResetAllDataNilAndErrors(t *testing.T) {
	mod, _ := testModule(t, nil)
	mustRegister(t, mod.Service)
	if err := mod.Service.ResetAllData(); err != nil {
		t.Fatalf("nil resetData: %v", err)
	}
	status, err := mod.Service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Registered {
		t.Fatal("nil resetData wiped the account")
	}

	want := errors.New("wipe failed")
	failing, _ := testModule(t, func() error { return want })
	if err := failing.Service.ResetAllData(); !errors.Is(err, want) {
		t.Fatalf("reset error = %v, want %v", err, want)
	}

	closed, db := testModule(t, nil)
	closed.Service.resetData = func() error {
		closeDB(t, db)
		return nil
	}
	if err := closed.Service.ResetAllData(); err == nil {
		t.Fatal("expected secret reload to fail after a closed database")
	}
}

func TestNewServiceReusesSecret(t *testing.T) {
	mod, db := testModule(t, nil)
	first := string(mod.Service.secret)

	again, err := NewService(NewRepository(db), nil, nil, silentLog())
	if err != nil {
		t.Fatalf("second new: %v", err)
	}
	if string(again.secret) != first {
		t.Fatal("signing secret was regenerated")
	}

	closeDB(t, db)
	if _, err := New(db, nil, nil, silentLog()); err == nil {
		t.Fatal("expected New to fail on a closed database")
	}
}

func TestSessionActive(t *testing.T) {
	now := time.Now()
	revoked := now.Add(-time.Minute)

	tests := []struct {
		name    string
		session Session
		want    bool
	}{
		{name: "live", session: Session{ExpiresAt: now.Add(time.Minute)}, want: true},
		{name: "expired equal", session: Session{ExpiresAt: now}, want: false},
		{name: "expired past", session: Session{ExpiresAt: now.Add(-time.Second)}, want: false},
		{name: "revoked", session: Session{ExpiresAt: now.Add(time.Hour), RevokedAt: &revoked}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.session.Active(now); got != test.want {
				t.Fatalf("active = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNormalizeEmailAndTruncate(t *testing.T) {
	if got := normalizeEmail(" Ada@Example.COM "); got != "ada@example.com" {
		t.Fatalf("normalize = %q", got)
	}
	if got := truncate("hello", 10); got != "hello" {
		t.Fatalf("short truncate = %q", got)
	}
	if got := truncate("hello world", 5); got != "hello" {
		t.Fatalf("long truncate = %q", got)
	}
}

func TestRepositoryLookups(t *testing.T) {
	mod, db := testModule(t, nil)
	mustRegister(t, mod.Service)

	if _, err := mod.Service.repo.FindByEmail("missing@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing email error = %v, want %v", err, ErrUserNotFound)
	}
	if _, err := mod.Service.repo.GetSession("missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want %v", err, ErrSessionNotFound)
	}

	user, err := mod.Service.repo.FindByEmail("ada@example.com")
	if err != nil {
		t.Fatalf("find by email: %v", err)
	}

	taken, err := mod.Service.repo.EmailTaken("ada@example.com", user.ID)
	if err != nil || taken {
		t.Fatalf("self email taken = %v, err=%v", taken, err)
	}
	taken, err = mod.Service.repo.UsernameTaken("ada", user.ID)
	if err != nil || taken {
		t.Fatalf("self username taken = %v, err=%v", taken, err)
	}

	closeDB(t, db)
	if _, err := mod.Service.Status(); err == nil {
		t.Fatal("expected status to fail on a closed database")
	}
}
