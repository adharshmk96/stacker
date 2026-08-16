package auth

import "time"

// User is the single operator account. Stacker is a single-tenant tool, so
// registration is only open until the first user exists (see Service.Register).
type User struct {
	ID       string `gorm:"primaryKey;size:32" json:"id"`
	Email    string `gorm:"uniqueIndex;size:200;not null" json:"email"`
	Name     string `gorm:"size:120;not null" json:"name"`
	Username string `gorm:"uniqueIndex;size:80;not null" json:"username"`

	// PasswordHash is bcrypt and never leaves the server.
	PasswordHash string `gorm:"not null" json:"-"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Session is one signed-in device. The JWT handed to the browser carries only
// this row's id, so revoking the row signs that device out immediately — a
// stateless JWT could not be taken back before it expired.
type Session struct {
	ID     string `gorm:"primaryKey;size:32" json:"id"`
	UserID string `gorm:"index;size:32;not null" json:"userId"`

	UserAgent string `gorm:"size:400" json:"userAgent"`
	IP        string `gorm:"size:80" json:"ip"`

	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt time.Time  `json:"lastSeenAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

// Active reports whether the session may still authenticate a request.
func (s Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// PasswordReset is a single-use forgot-password grant. Only the hash of the
// token is stored, so a copy of the database does not hand out account access.
type PasswordReset struct {
	TokenHash string `gorm:"primaryKey;size:64"`
	UserID    string `gorm:"index;size:32;not null"`

	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// Secret persists server-side values that must survive a restart — today just
// the key the session JWTs are signed with. Regenerating it on every boot would
// sign everyone out whenever stacker restarts.
type Secret struct {
	Key   string `gorm:"primaryKey;size:60"`
	Value string `gorm:"not null"`
}

/* ---- request payloads ---- */

// RegisterRequest creates the very first account.
type RegisterRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Username string `json:"username" binding:"required,min=2,max=80"`
	Email    string `json:"email" binding:"required,email,max=200"`
	Password string `json:"password" binding:"required,min=8,max=200"`
}

type LoginRequest struct {
	// Identifier accepts either the email or the username.
	Identifier string `json:"identifier" binding:"required"`
	Password   string `json:"password" binding:"required"`
}

type UpdateProfileRequest struct {
	Name     string `json:"name" binding:"required,min=1,max=120"`
	Username string `json:"username" binding:"required,min=2,max=80"`
	Email    string `json:"email" binding:"required,email,max=200"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required,min=8,max=200"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8,max=200"`
}

/* ---- responses ---- */

// Status tells the UI whether to show the register screen or the login screen.
type Status struct {
	Registered bool `json:"registered"`
}

// LoginResult is what a successful login or registration answers with.
type LoginResult struct {
	Token     string    `json:"token"`
	User      User      `json:"user"`
	SessionID string    `json:"sessionId"`
	ExpiresAt time.Time `json:"expiresAt"`
}
