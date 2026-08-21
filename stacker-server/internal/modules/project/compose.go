package project

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// This file turns a user's compose file into something `docker stack deploy`
// will accept, without editing the file itself.
//
// Everything stacker adds — the proxy network, the environment's variables, the
// rollout settings, the image tags for services that are built rather than
// pulled — goes into a second compose file passed after the first. Compose
// merges the two, so the user's file stays exactly as it is in their repository
// and the diff stacker applied is one readable artifact in the run's log.

// composeFile is the part of a compose file this module has to understand. Every
// other key is irrelevant here and is left alone by the override.
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image string `yaml:"image"`
	// Build is `any` because compose allows both a string and a mapping there,
	// and all this needs to know is whether it is present.
	Build  any `yaml:"build"`
	Deploy *struct {
		Replicas  *int `yaml:"replicas"`
		Placement *struct {
			Constraints []string `yaml:"constraints"`
		} `yaml:"placement"`
	} `yaml:"deploy"`
}

// composeSpec is a parsed compose file plus the answers the deploy needs from
// it, worked out once.
type composeSpec struct {
	// Names are the service keys, sorted, so generated output is stable.
	Names []string
	// Builds are the services compose has to build because they declare a
	// build context. They are the reason a deploy runs `docker compose build`.
	Builds []string
	// PinnedReplicas are the services that set their own replica count. The
	// environment's replica setting is not applied to them.
	PinnedReplicas map[string]bool
	// PinnedPlacement is the same for placement constraints.
	PinnedPlacement map[string]bool
	// NeedsImageTag are the built services with no `image:` of their own. Stack
	// deploy ignores `build` and requires an image, so stacker names one.
	NeedsImageTag map[string]bool
}

// parseCompose reads a compose file. It is deliberately tolerant: only the
// `services` block is inspected, so a file using keys stacker has never heard of
// still parses.
func parseCompose(content string) (composeSpec, error) {
	var file composeFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return composeSpec{}, fmt.Errorf("%w: %s", ErrComposeInvalid, err)
	}
	if len(file.Services) == 0 {
		return composeSpec{}, ErrNoServices
	}

	spec := composeSpec{
		PinnedReplicas:  map[string]bool{},
		PinnedPlacement: map[string]bool{},
		NeedsImageTag:   map[string]bool{},
	}
	for name, service := range file.Services {
		if !serviceName.MatchString(name) {
			return composeSpec{}, fmt.Errorf("%w: %q is not a usable service name", ErrComposeInvalid, name)
		}
		spec.Names = append(spec.Names, name)

		if service.Build != nil {
			spec.Builds = append(spec.Builds, name)
			if strings.TrimSpace(service.Image) == "" {
				spec.NeedsImageTag[name] = true
			}
		}
		if service.Deploy != nil {
			if service.Deploy.Replicas != nil {
				spec.PinnedReplicas[name] = true
			}
			if service.Deploy.Placement != nil && len(service.Deploy.Placement.Constraints) > 0 {
				spec.PinnedPlacement[name] = true
			}
		}
	}
	sort.Strings(spec.Names)
	sort.Strings(spec.Builds)
	return spec, nil
}

// Has reports whether the compose file declares a service, which is how a
// domain's target is checked before anything is deployed.
func (s composeSpec) Has(name string) bool {
	for _, candidate := range s.Names {
		if candidate == name {
			return true
		}
	}
	return false
}

// imageTag names a locally built image for one deployment. A deployment ID is
// part of the tag so Swarm sees a changed service specification after a rebuild
// instead of keeping tasks that refer to an unchanged `:latest` tag.
func imageTag(stack, service, deploymentID string) string {
	return "stacker/" + stack + "/" + service + ":" + deploymentID
}

// overrideOptions is everything the override needs that is not in the compose
// file itself.
type overrideOptions struct {
	Stack string
	// BuildImageTags overrides every locally built service. Docker Swarm ignores
	// build directives, so this must be the same tag Docker Compose built and it
	// must change for each deployment to make Swarm replace the task.
	BuildImageTags map[string]string
	// Network is the attachable overlay Traefik sits on. Every service that a
	// domain points at has to join it, or the proxy cannot resolve the service
	// name it is asked to forward to.
	Network string
	// ProxyServices are the services with at least one domain. Only they join
	// the proxy network — a database has no reason to be reachable from it.
	ProxyServices map[string]bool
	Env           map[string]string
	Deploy        DeploySettings
}

// buildOverride renders the compose file stacker adds on top of the user's.
//
// It is built as plain maps and marshalled, rather than assembled as text: a
// value that needs quoting (a password with a `#` in it, a constraint with a
// colon) is then the YAML library's problem and not a class of bug that only
// shows up on someone's production deploy.
func buildOverride(spec composeSpec, opts overrideOptions) (string, error) {
	services := map[string]any{}

	// Sorted keys from the spec, so the same project renders the same file every
	// time and a diff in the log means something actually changed.
	keys := make([]string, 0, len(opts.Env))
	for key := range opts.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, name := range spec.Names {
		service := map[string]any{}

		if len(keys) > 0 {
			environment := make(map[string]any, len(keys))
			for _, key := range keys {
				environment[key] = opts.Env[key]
			}
			service["environment"] = environment
		}

		if tag := opts.BuildImageTags[name]; tag != "" {
			service["image"] = tag
		} else if spec.NeedsImageTag[name] {
			service["image"] = imageTag(opts.Stack, name, "latest")
		}

		// `default` is listed explicitly: naming a network in an override
		// replaces the implicit default, and dropping it would cut the services
		// off from each other.
		if opts.ProxyServices[name] {
			service["networks"] = []string{"default", "proxy"}
		}

		deploy := map[string]any{}
		if !spec.PinnedReplicas[name] && opts.Deploy.Replicas > 0 {
			deploy["replicas"] = opts.Deploy.Replicas
		}
		if !spec.PinnedPlacement[name] && strings.TrimSpace(opts.Deploy.Placement) != "" {
			deploy["placement"] = map[string]any{
				"constraints": []string{strings.TrimSpace(opts.Deploy.Placement)},
			}
		}
		deploy["update_config"] = updateConfig(opts.Deploy)
		if opts.Deploy.AutoRollback {
			// Swarm only rolls back on its own when the update itself is judged
			// to have failed, which is why the monitor window matters: a task
			// that dies after it is set has already been accepted.
			deploy["rollback_config"] = map[string]any{"order": "start-first"}
		}
		service["deploy"] = deploy

		services[name] = service
	}

	doc := map[string]any{
		"services": services,
		"networks": map[string]any{
			// External: the network is created by install.sh as part of the
			// stacker stack, and a project must never own or remove it.
			"proxy": map[string]any{"external": true, "name": opts.Network},
		},
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// updateConfig maps the environment's strategy onto swarm's update settings.
//
// `rolling` starts the replacement task before stopping the old one, so a single
// replica service stays up across a deploy; `recreate` stops first, which is
// what a service holding an exclusive lock or a host port needs.
func updateConfig(settings DeploySettings) map[string]any {
	order := "start-first"
	if settings.Strategy == StrategyRecreate {
		order = "stop-first"
	}

	config := map[string]any{
		"order":       order,
		"parallelism": 1,
	}
	if settings.HealthGraceSec > 0 {
		// The window swarm watches a new task in before accepting it, and the
		// window auto-rollback is decided within.
		config["monitor"] = fmt.Sprintf("%ds", settings.HealthGraceSec)
	}
	if settings.AutoRollback {
		config["failure_action"] = "rollback"
	}
	return config
}
