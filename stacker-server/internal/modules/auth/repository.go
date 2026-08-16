package auth

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Repository is the only place this module touches the database.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

/* ---- users ---- */

func (r *Repository) CountUsers() (int64, error) {
	var count int64
	err := r.db.Model(&User{}).Count(&count).Error
	return count, err
}

func (r *Repository) GetUser(id string) (User, error) {
	var user User
	err := r.db.First(&user, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

// FindByIdentifier resolves the login field, which accepts either form. Emails
// are stored normalized, so the address half is matched case-insensitively —
// nobody expects `Me@Example.com` to be a different login from `me@example.com`.
func (r *Repository) FindByIdentifier(identifier string) (User, error) {
	var user User
	err := r.db.First(&user, "email = ? OR username = ?", normalizeEmail(identifier), identifier).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (r *Repository) FindByEmail(email string) (User, error) {
	var user User
	err := r.db.First(&user, "email = ?", email).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

// EmailTaken and UsernameTaken exclude the given user so an update that keeps
// the current value does not collide with itself.
func (r *Repository) EmailTaken(email, exceptID string) (bool, error) {
	var count int64
	err := r.db.Model(&User{}).Where("email = ? AND id <> ?", email, exceptID).Count(&count).Error
	return count > 0, err
}

func (r *Repository) UsernameTaken(username, exceptID string) (bool, error) {
	var count int64
	err := r.db.Model(&User{}).Where("username = ? AND id <> ?", username, exceptID).Count(&count).Error
	return count > 0, err
}

func (r *Repository) CreateUser(user *User) error {
	return r.db.Create(user).Error
}

func (r *Repository) SaveUser(user *User) error {
	return r.db.Save(user).Error
}

/* ---- sessions ---- */

func (r *Repository) CreateSession(session *Session) error {
	return r.db.Create(session).Error
}

func (r *Repository) GetSession(id string) (Session, error) {
	var session Session
	err := r.db.First(&session, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, ErrSessionNotFound
	}
	return session, err
}

// ListSessions returns the sessions still worth showing: revoked and expired
// ones are noise once they can no longer sign anyone in.
func (r *Repository) ListSessions(userID string, now time.Time) ([]Session, error) {
	var sessions []Session
	err := r.db.
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).
		Order("last_seen_at desc").
		Find(&sessions).Error
	return sessions, err
}

func (r *Repository) TouchSession(id string, at time.Time) error {
	return r.db.Model(&Session{}).Where("id = ?", id).Update("last_seen_at", at).Error
}

func (r *Repository) RevokeSession(id, userID string, at time.Time) error {
	res := r.db.Model(&Session{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", at)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// RevokeOtherSessions signs every device out but the one making the request —
// what a password change should do to a possibly stolen session.
func (r *Repository) RevokeOtherSessions(userID, keepID string, at time.Time) error {
	return r.db.Model(&Session{}).
		Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, keepID).
		Update("revoked_at", at).Error
}

func (r *Repository) RevokeAllSessions(userID string, at time.Time) error {
	return r.db.Model(&Session{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", at).Error
}

/* ---- password resets ---- */

func (r *Repository) CreateReset(reset *PasswordReset) error {
	return r.db.Create(reset).Error
}

func (r *Repository) GetReset(tokenHash string) (PasswordReset, error) {
	var reset PasswordReset
	err := r.db.First(&reset, "token_hash = ?", tokenHash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PasswordReset{}, ErrInvalidResetToken
	}
	return reset, err
}

func (r *Repository) MarkResetUsed(tokenHash string, at time.Time) error {
	return r.db.Model(&PasswordReset{}).
		Where("token_hash = ?", tokenHash).
		Update("used_at", at).Error
}

// InvalidateResets burns any outstanding links for a user, so a reset mail sent
// before a password change cannot be replayed afterwards.
func (r *Repository) InvalidateResets(userID string, at time.Time) error {
	return r.db.Model(&PasswordReset{}).
		Where("user_id = ? AND used_at IS NULL", userID).
		Update("used_at", at).Error
}

/* ---- secrets ---- */

// SecretOrCreate reads a persisted secret, generating and storing it the first
// time it is asked for.
func (r *Repository) SecretOrCreate(key string, generate func() string) (string, error) {
	var secret Secret
	err := r.db.First(&secret, "key = ?", key).Error
	if err == nil {
		return secret.Value, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	secret = Secret{Key: key, Value: generate()}
	if err := r.db.Create(&secret).Error; err != nil {
		return "", err
	}
	return secret.Value, nil
}
