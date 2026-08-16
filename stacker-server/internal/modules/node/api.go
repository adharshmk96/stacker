package node

import "context"

// This file is the seam other modules reach the docker daemons through.
//
// Running a docker command against a node means knowing whether it is local or
// remote, which key to use, how to quote for the remote shell and how to read
// docker's failures — all of which already lives in this package. Rather than
// duplicate any of it, modules that need docker (the swarm resource browser,
// for one) take a small interface satisfied by these three methods.

// Docker runs a docker subcommand on the given node and returns its output.
func (s *Service) Docker(ctx context.Context, item Node, args ...string) (string, error) {
	return s.rt.docker(ctx, item, args...)
}

// DockerInput is Docker with something piped to the command's stdin, for the
// handful of calls that take their payload that way — `docker stack deploy -c -`
// and `docker secret create name -`.
func (s *Service) DockerInput(ctx context.Context, item Node, stdin string, args ...string) (string, error) {
	return s.rt.dockerInput(ctx, item, stdin, args...)
}

// Manager returns the node running the swarm control plane. It is where every
// swarm-wide query and mutation is sent.
func (s *Service) Manager() (Node, error) {
	item, err := s.repo.Manager()
	if err != nil {
		return Node{}, err
	}
	s.decorate(&item)
	return item, nil
}

// InSwarm reports whether a node is one stacker has configured into the swarm,
// which is the test for "can docker commands be sent here".
func (item Node) InSwarm() bool { return item.SwarmRole != SwarmRoleNone }
