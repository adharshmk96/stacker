package vps

import "errors"

var (
	ErrNotFound      = errors.New("vps not found")
	ErrNameTaken     = errors.New("a vps with that name already exists")
	ErrInvalidSsh    = errors.New("ssh must be in user@host form")
	ErrSshKeyMissing = errors.New("the referenced ssh key does not exist")
	ErrCopyIDMissing = errors.New("sshpass and ssh-copy-id are required to install a key")
)
