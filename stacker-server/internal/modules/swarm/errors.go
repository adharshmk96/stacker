package swarm

import "errors"

var (
	ErrUnknownResource = errors.New("unknown swarm resource")
	ErrUnknownAction   = errors.New("unknown action for this resource")
	ErrNoManager       = errors.New("there is no swarm manager yet — configure a node under Nodes first")
	ErrNodeRequired    = errors.New("this resource lives on a single node, so the node must be given")
	ErrUnknownNode     = errors.New("that node is not part of the swarm")
	ErrNameRequired    = errors.New("a name is required")
	ErrImageRequired   = errors.New("an image is required")
	ErrContentRequired = errors.New("some content is required")
	ErrReplicasNeeded  = errors.New("a replica count is required")
	ErrGlobalService   = errors.New("a global service runs one task per node and cannot be scaled")
)
