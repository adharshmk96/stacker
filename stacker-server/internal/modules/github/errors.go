package github

import "errors"

var (
	ErrNotFound           = errors.New("GitHub App not found")
	ErrInvalidName        = errors.New("app name may contain letters, numbers, spaces, dots, underscores, and hyphens")
	ErrInvalidBaseURL     = errors.New("base URL must be an http or https origin")
	ErrInvalidCallback    = errors.New("invalid or expired GitHub callback")
	ErrNotInstalled       = errors.New("GitHub App is not installed")
	ErrInvalidExistingApp = errors.New("GitHub App ID, installation ID, private key, and webhook secret are required")
)
