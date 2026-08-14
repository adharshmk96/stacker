package node

import "errors"

var (
	ErrNotFound      = errors.New("node not found")
	ErrNameTaken     = errors.New("a node with that name already exists")
	ErrInvalidSsh    = errors.New("ssh must be in user@host form")
	ErrSshKeyMissing = errors.New("the referenced ssh key does not exist")
	ErrLocalNode     = errors.New("the local node cannot be deleted")
	ErrCopyIDMissing = errors.New("sshpass and ssh-copy-id are required to install a key")
)
