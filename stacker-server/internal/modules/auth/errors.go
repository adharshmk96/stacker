package auth

import "errors"

var (
	ErrAlreadyRegistered  = errors.New("an account already exists; registration is closed")
	ErrInvalidCredentials = errors.New("incorrect email or password")
	ErrUnauthorized       = errors.New("not signed in")
	ErrUserNotFound       = errors.New("account not found")
	ErrEmailTaken         = errors.New("that email is already in use")
	ErrUsernameTaken      = errors.New("that username is already in use")
	ErrInvalidUsername    = errors.New("username may only contain letters, digits, dot, dash and underscore")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
	ErrSessionNotFound    = errors.New("session not found")
	ErrInvalidResetToken  = errors.New("this reset link is invalid or has expired")
)
