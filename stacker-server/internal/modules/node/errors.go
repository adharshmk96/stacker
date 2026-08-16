package node

import "errors"

var (
	ErrNotFound          = errors.New("node not found")
	ErrNameTaken         = errors.New("a node with that name already exists")
	ErrInvalidSsh        = errors.New("ssh must be in user@host form")
	ErrSshKeyMissing     = errors.New("the referenced ssh key does not exist")
	ErrLocalNode         = errors.New("the local node cannot be deleted")
	ErrLocalSetupManaged = errors.New("the local swarm manager is configured by install.sh; rerun the installer")
	ErrCopyIDMissing     = errors.New("sshpass and ssh-copy-id are required to install a key")
	// ErrPasswordRequired is only reached when the key is not already installed:
	// a repeat install short-circuits before the password is looked at.
	ErrPasswordRequired = errors.New("a password is required to install the key on this host for the first time")

	// Swarm errors.
	ErrDockerMissing    = errors.New("docker is not installed on this node")
	ErrDockerNotRunning = errors.New("the docker daemon is not reachable on this node — is it running, and can this user talk to it?")
	ErrSwarmUnreachable = errors.New("the node could not be reached")
	// ErrSwarmCommand wraps a docker command that ran and failed; the wrapped
	// text is docker's own message, which is what the user needs to read.
	ErrSwarmCommand     = errors.New("the docker command failed")
	ErrAdvertiseAddr    = errors.New("no advertise address")
	ErrNoManager        = errors.New("there is no swarm manager yet — rerun install.sh on the host")
	ErrManagerUnhealthy = errors.New("the swarm manager is not reachable, so no node can join right now")
	ErrAlreadyInSwarm   = errors.New("this node already belongs to a swarm")
	ErrForeignSwarm     = errors.New("this node already belongs to a different swarm — make it leave that one before configuring it here")
	ErrNotInSwarm       = errors.New("this node is not part of the swarm")
	ErrNotWorker        = errors.New("only a worker can be promoted")
	ErrNotManagerRole   = errors.New("only a manager can be demoted")
	ErrLastManager      = errors.New("this is the only manager — promote another node before changing this one")
	ErrKeyNotVerified   = errors.New("ssh key authentication has not been verified for this node — test the connection first")
	ErrSwarmBusy        = errors.New("another swarm operation is already running for this node")
	ErrNodeInSwarm      = errors.New("this node is still part of the swarm — remove it from the swarm before deleting it")

	// Provisioning errors.
	ErrNoProvisionRun = errors.New("this node has not been configured in this session")
	ErrUnsupportedOS  = errors.New("stacker can only install docker on linux — install it yourself, then configure this node again")
	ErrLocalNotLinux  = errors.New("this machine is not linux, so stacker cannot install docker on it — install Docker Desktop, start it, then configure this node again")
	ErrSudoRequired   = errors.New("installing docker needs root: the login user must be root or have passwordless sudo")
	ErrCurlMissing    = errors.New("curl is missing and could not be installed, so the docker install script cannot be fetched")
	ErrDockerInstall  = errors.New("the docker install script failed")
)
