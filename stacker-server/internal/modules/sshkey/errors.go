package sshkey

import "errors"

var (
	ErrNotFound     = errors.New("ssh key not found")
	ErrNameTaken    = errors.New("an ssh key with that name already exists")
	ErrInvalidName  = errors.New("name may only contain letters, digits, dot, dash and underscore")
	ErrUnknownType  = errors.New("unsupported key type")
	ErrKeyInUse     = errors.New("ssh key is still used by one or more nodes")
	ErrDefaultKey   = errors.New("the default ssh key cannot be deleted")
	ErrKeyGenFailed = errors.New("failed to generate keypair")
)
