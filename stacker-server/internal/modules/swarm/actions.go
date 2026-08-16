package swarm

import (
	"fmt"
	"strings"
)

// actionSpec is one row action: the docker command it becomes, the line the
// toast shows afterwards, and whether the output is meant for the user.
type actionSpec struct {
	build   func(req ActionRequest) ([]string, error)
	message func(req ActionRequest) string
	// reads marks the actions whose whole point is the output — logs, inspect,
	// a config's content. Everything else is a mutation and answers with a
	// message only.
	reads bool
}

// simple is the common case: a fixed command with the row's id appended.
//
// The message is a format string when it names the row and a plain sentence
// otherwise, so `%s` decides rather than a second parameter.
func simple(message string, argv ...string) actionSpec {
	return actionSpec{
		build: func(req ActionRequest) ([]string, error) {
			// Copied rather than appended to: `argv` is shared by every call
			// this spec ever serves.
			return append(append([]string{}, argv...), req.ID), nil
		},
		message: func(req ActionRequest) string {
			if !strings.Contains(message, "%s") {
				return message
			}
			return fmt.Sprintf(message, req.ID)
		},
	}
}

// reader is `simple` for the actions that exist to show their output.
func reader(message string, argv ...string) actionSpec {
	spec := simple(message, argv...)
	spec.reads = true
	return spec
}

// actions is what each resource's row menu can do, keyed by the action key the
// UI sends. An action missing here is refused — the table is the whole of what
// stacker will run against a docker daemon.
var actions = map[Resource]map[string]actionSpec{
	Stacks: {
		"services": reader("Services in %s", "stack", "services"),
		"tasks":    reader("Tasks in %s", "stack", "ps", "--no-trunc"),
		"remove":   simple("Removed the stack %s", "stack", "rm"),
	},
	Services: {
		"logs":     reader("Logs for %s", "service", "logs", "--tail", logTail, "--timestamps", "--no-task-ids"),
		"inspect":  reader("%s", "service", "inspect", "--pretty"),
		"tasks":    reader("Tasks for %s", "service", "ps", "--no-trunc"),
		"update":   simple("Forced an update of %s", "service", "update", "--detach=true", "--force"),
		"rollback": simple("Rolled %s back to its previous spec", "service", "rollback", "--detach=true"),
		"remove":   simple("Removed the service %s", "service", "rm"),
		"scale": {
			build: func(req ActionRequest) ([]string, error) {
				if req.Replicas == nil {
					return nil, ErrReplicasNeeded
				}
				return []string{"service", "scale", "--detach=true", scaleArg(req.ID, *req.Replicas)}, nil
			},
			message: func(req ActionRequest) string {
				return fmt.Sprintf("Scaling %s to %d replicas", req.ID, *req.Replicas)
			},
		},
	},
	Tasks: {
		// A task id is a valid argument to `service logs`, which is how a
		// single replica's output is read.
		"logs":    reader("Logs for this task", "service", "logs", "--tail", logTail, "--timestamps"),
		"inspect": reader("Task %s", "inspect"),
	},
	Containers: {
		"logs":    reader("Logs for %s", "logs", "--tail", logTail, "--timestamps"),
		"inspect": reader("Container %s", "inspect"),
		"start":   simple("Started %s", "start"),
		"stop":    simple("Stopped %s", "stop"),
		"restart": simple("Restarted %s", "restart"),
		// -f because the list offers Remove on a running container too, and
		// refusing after the user confirmed would just be a second click.
		"remove": simple("Removed the container %s", "rm", "-f"),
	},
	Images: {
		"inspect": reader("Image %s", "image", "inspect"),
		"pull":    simple("Pulled %s", "image", "pull"),
		"remove":  simple("Removed the image %s", "image", "rm"),
	},
	Volumes: {
		"inspect": reader("Volume %s", "volume", "inspect"),
		"remove":  simple("Removed the volume %s", "volume", "rm"),
	},
	Networks: {
		"inspect": reader("Network %s", "network", "inspect"),
		"remove":  simple("Removed the network %s", "network", "rm"),
	},
	Secrets: {
		// Only the metadata: docker will not read a secret's value back, which
		// is the point of storing it as a secret.
		"inspect": reader("Secret %s", "secret", "inspect", "--pretty"),
		"remove":  simple("Removed the secret %s", "secret", "rm"),
	},
	Configs: {
		"inspect": reader("Config %s", "config", "inspect", "--pretty"),
		// Unlike a secret, a config's content is readable, and reading it is
		// the main reason to open one.
		"view":   reader("Content of %s", "config", "inspect", "--format", "{{ printf \"%s\" .Spec.Data }}"),
		"remove": simple("Removed the config %s", "config", "rm"),
	},
}
