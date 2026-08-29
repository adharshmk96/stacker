package auth

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// sessionTTL is how long a signed-in device stays valid without signing in
	// again. Long, because stacker is an operator tool run on trusted machines.
	sessionTTL = 30 * 24 * time.Hour
	// resetTTL is deliberately short — the link sits in the server log.
	resetTTL = 1 * time.Hour

	jwtSecretKey = "jwt_secret"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ResetDataFunc wipes every table and re-seeds the empty database. It is
// injected rather than imported so this module does not have to know about the
// other modules it is about to erase.
type ResetDataFunc func() error

// MailSender delivers outbound email when SMTP is configured.
type MailSender interface {
	Enabled() bool
	Send(to, subject, body string) error
}

// Service owns accounts, sessions and password resets.
type Service struct {
	repo      *Repository
	secret    []byte
	resetData ResetDataFunc
	mail      MailSender
	log       *slog.Logger
}

// NewService loads (or creates on first run) the signing secret.
func NewService(repo *Repository, resetData ResetDataFunc, mail MailSender, log *slog.Logger) (*Service, error) {
	secret, err := repo.SecretOrCreate(jwtSecretKey, func() string { return randomHex(32) })
	if err != nil {
		return nil, err
	}

	return &Service{repo: repo, secret: []byte(secret), resetData: resetData, mail: mail, log: log}, nil
}

// Status reports whether the install has an account yet.
func (s *Service) Status() (Status, error) {
	count, err := s.repo.CountUsers()
	if err != nil {
		return Status{}, err
	}
	return Status{Registered: count > 0}, nil
}

// Register creates the first and only account. Once one exists the endpoint is
// closed — this is a single-operator tool, not a signup page.
func (s *Service) Register(req RegisterRequest, agent, ip string) (LoginResult, error) {
	count, err := s.repo.CountUsers()
	if err != nil {
		return LoginResult{}, err
	}
	if count > 0 {
		return LoginResult{}, ErrAlreadyRegistered
	}

	username := strings.TrimSpace(req.Username)
	if !usernamePattern.MatchString(username) {
		return LoginResult{}, ErrInvalidUsername
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return LoginResult{}, err
	}

	user := User{
		ID:           newID(),
		Name:         strings.TrimSpace(req.Name),
		Username:     username,
		Email:        normalizeEmail(req.Email),
		PasswordHash: string(hash),
	}
	if err := s.repo.CreateUser(&user); err != nil {
		return LoginResult{}, err
	}

	s.log.Info("account registered", "id", user.ID, "email", user.Email)
	return s.startSession(user, agent, ip)
}

// Login verifies the password and opens a session.
func (s *Service) Login(req LoginRequest, agent, ip string) (LoginResult, error) {
	user, err := s.repo.FindByIdentifier(strings.TrimSpace(req.Identifier))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Same error either way, so the response cannot be used to find out
			// which accounts exist.
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	return s.startSession(user, agent, ip)
}

// Authenticate resolves a bearer token to its user and session. It is the one
// place the token's session id is checked against the database, which is what
// makes revocation immediate.
func (s *Service) Authenticate(token string) (User, Session, error) {
	c, err := parseToken(s.secret, token)
	if err != nil {
		return User{}, Session{}, ErrUnauthorized
	}

	session, err := s.repo.GetSession(c.SessionID)
	if err != nil {
		return User{}, Session{}, ErrUnauthorized
	}

	now := time.Now()
	if !session.Active(now) || session.UserID != c.UserID {
		return User{}, Session{}, ErrUnauthorized
	}

	user, err := s.repo.GetUser(session.UserID)
	if err != nil {
		return User{}, Session{}, ErrUnauthorized
	}

	// Best-effort: a failed touch must not fail the request it rode in on.
	if now.Sub(session.LastSeenAt) > time.Minute {
		if err := s.repo.TouchSession(session.ID, now); err != nil {
			s.log.Warn("could not record session activity", "session", session.ID, "error", err)
		}
	}

	return user, session, nil
}

// Logout revokes the session the caller is using.
func (s *Service) Logout(session Session) error {
	return s.repo.RevokeSession(session.ID, session.UserID, time.Now())
}

func (s *Service) Sessions(userID string) ([]Session, error) {
	return s.repo.ListSessions(userID, time.Now())
}

// RevokeSession signs one other device out.
func (s *Service) RevokeSession(userID, sessionID string) error {
	return s.repo.RevokeSession(sessionID, userID, time.Now())
}

// UpdateProfile changes the account's identity fields, leaving credentials alone.
func (s *Service) UpdateProfile(userID string, req UpdateProfileRequest) (User, error) {
	user, err := s.repo.GetUser(userID)
	if err != nil {
		return User{}, err
	}

	username := strings.TrimSpace(req.Username)
	if !usernamePattern.MatchString(username) {
		return User{}, ErrInvalidUsername
	}
	email := normalizeEmail(req.Email)

	taken, err := s.repo.EmailTaken(email, userID)
	if err != nil {
		return User{}, err
	}
	if taken {
		return User{}, ErrEmailTaken
	}

	taken, err = s.repo.UsernameTaken(username, userID)
	if err != nil {
		return User{}, err
	}
	if taken {
		return User{}, ErrUsernameTaken
	}

	user.Name = strings.TrimSpace(req.Name)
	user.Username = username
	user.Email = email

	if err := s.repo.SaveUser(&user); err != nil {
		return User{}, err
	}
	return user, nil
}

// ChangePassword rotates the password and signs every other device out — if the
// reason for the change is a leaked password, leaving those sessions alive would
// defeat the point.
func (s *Service) ChangePassword(userID, currentSessionID string, req ChangePasswordRequest) error {
	user, err := s.repo.GetUser(userID)
	if err != nil {
		return err
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)) != nil {
		return ErrInvalidCredentials
	}

	return s.setPassword(user, req.NewPassword, currentSessionID)
}

// ForgotPassword mints a reset link. Stacker has no mail transport, so the link
// goes to the server log — the operator reads it out of `docker logs`.
//
// The result is the same whether or not the address matched, so the endpoint
// cannot be used to test which address owns the install.
func (s *Service) ForgotPassword(req ForgotPasswordRequest, baseURL string) error {
	user, err := s.repo.FindByEmail(normalizeEmail(req.Email))
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			s.log.Info("password reset requested for an unknown address", "email", req.Email)
			return nil
		}
		return err
	}

	token := newResetToken()
	reset := PasswordReset{
		TokenHash: hashToken(token),
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(resetTTL),
	}
	if err := s.repo.CreateReset(&reset); err != nil {
		return err
	}

	link := strings.TrimSuffix(baseURL, "/") + "/reset-password?token=" + token
	subject := "Reset your Stacker password"
	body := "Open this link to set a new password (expires in " + resetTTL.String() + "):\n\n" + link

	if s.mail != nil && s.mail.Enabled() {
		if err := s.mail.Send(user.Email, subject, body); err != nil {
			s.log.Error("could not send password reset email", "email", user.Email, "error", err)
			return err
		}
		s.log.Info("password reset email sent", "email", user.Email)
		return nil
	}

	s.log.Info("password reset link issued — open it to set a new password",
		"email", user.Email,
		"expiresIn", resetTTL.String(),
		"link", link,
	)
	return nil
}

// ResetPassword consumes a link from ForgotPassword and sets a new password.
// Every session is revoked: whoever asked for the reset has lost access to the
// old ones by definition.
func (s *Service) ResetPassword(req ResetPasswordRequest) error {
	reset, err := s.repo.GetReset(hashToken(strings.TrimSpace(req.Token)))
	if err != nil {
		return err
	}
	if reset.UsedAt != nil || time.Now().After(reset.ExpiresAt) {
		return ErrInvalidResetToken
	}

	user, err := s.repo.GetUser(reset.UserID)
	if err != nil {
		return ErrInvalidResetToken
	}

	if err := s.setPassword(user, req.Password, ""); err != nil {
		return err
	}

	return s.repo.MarkResetUsed(reset.TokenHash, time.Now())
}

// ResetAllData empties the database — nodes, keys, and the account itself — and
// leaves the install back at first-run, where /register is open again.
func (s *Service) ResetAllData() error {
	if s.resetData == nil {
		return nil
	}

	s.log.Warn("resetting all data: every node, key and account is being erased")
	if err := s.resetData(); err != nil {
		return err
	}

	// The wipe took the signing secret with it, so re-read it: leaving the old
	// key in memory would keep pre-reset tokens verifiable.
	secret, err := s.repo.SecretOrCreate(jwtSecretKey, func() string { return randomHex(32) })
	if err != nil {
		return err
	}
	s.secret = []byte(secret)

	s.log.Warn("all data reset")
	return nil
}

// setPassword hashes and stores a new password, then clears the sessions and
// pending reset links it invalidates.
func (s *Service) setPassword(user User, password, keepSessionID string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hash)
	if err := s.repo.SaveUser(&user); err != nil {
		return err
	}

	now := time.Now()
	if keepSessionID == "" {
		err = s.repo.RevokeAllSessions(user.ID, now)
	} else {
		err = s.repo.RevokeOtherSessions(user.ID, keepSessionID, now)
	}
	if err != nil {
		return err
	}

	if err := s.repo.InvalidateResets(user.ID, now); err != nil {
		return err
	}

	s.log.Info("password changed", "user", user.ID)
	return nil
}

// startSession opens a session row and signs a token naming it.
func (s *Service) startSession(user User, agent, ip string) (LoginResult, error) {
	now := time.Now()
	session := Session{
		ID:         newID(),
		UserID:     user.ID,
		UserAgent:  truncate(agent, 400),
		IP:         truncate(ip, 80),
		LastSeenAt: now,
		ExpiresAt:  now.Add(sessionTTL),
	}
	if err := s.repo.CreateSession(&session); err != nil {
		return LoginResult{}, err
	}

	token, err := signToken(s.secret, claims{
		SessionID: session.ID,
		UserID:    user.ID,
		IssuedAt:  now.Unix(),
		ExpiresAt: session.ExpiresAt.Unix(),
	})
	if err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		Token:     token,
		User:      user,
		SessionID: session.ID,
		ExpiresAt: session.ExpiresAt,
	}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
