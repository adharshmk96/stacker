package project

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

// timeNow is a variable so tests can pin the clock without threading a clock
// through every call.
var timeNow = func() time.Time { return time.Now().UTC() }

var (
	// namePattern is what the create form already enforces client side.
	namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	// envNamePattern is stricter than namePattern: an environment name becomes
	// part of the swarm stack name, and docker only accepts these there.
	envNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	// hostLabel is one label of a hostname. A leading `*` is allowed so a
	// wildcard host can be routed.
	hostLabel = regexp.MustCompile(`^(\*|[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)$`)
	// serviceName matches a compose service key.
	serviceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// slugStrip collapses everything that cannot appear in a stack name.
	slugStrip = regexp.MustCompile(`[^a-z0-9]+`)
)

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand only fails catastrophically; nothing to recover to.
		panic(err)
	}
	return hex.EncodeToString(buf)
}

// slug renders a name as a docker-safe identifier. Docker accepts more than
// this, but stack names end up in service names, DNS names and Traefik router
// keys, and lowercase alphanumerics with dashes are legal in all of them.
func slug(value string) string {
	out := slugStrip.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	out = strings.Trim(out, "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	return out
}

// StackName is the swarm stack one environment deploys into.
//
// It is derived rather than stored so it stays in step with a rename, and it is
// prefixed so a stack stacker owns is never confused with one deployed by hand.
// Both halves are slugged and capped, which is what keeps the whole name inside
// the length docker allows for the service names built on top of it.
func StackName(project Project, env Environment) string {
	name := slug(project.Name)
	if name == "" {
		name = project.ID
	}
	envName := slug(env.Name)
	if envName == "" {
		envName = env.ID
	}
	return "stk-" + name + "-" + envName
}

// validHost checks a hostname the way Traefik's Host rule needs it: no scheme,
// no port, no path.
func validHost(value string) bool {
	if value == "" || len(value) > 253 || strings.HasSuffix(value, ".") {
		return false
	}
	if strings.ContainsAny(value, "/:@ \t") {
		return false
	}

	labels := strings.Split(value, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !hostLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// envMap flattens an environment's variables and secrets into the map handed to
// docker. Secrets are applied last so a secret wins a key collision — a value
// the user marked secret is the one they meant to keep.
func envMap(env Environment) map[string]string {
	values := make(map[string]string, len(env.Variables)+len(env.Secrets))
	for _, list := range [][]EnvVar{env.Variables, env.Secrets} {
		for _, item := range list {
			key := strings.TrimSpace(item.Key)
			if key == "" {
				continue
			}
			values[key] = item.Value
		}
	}
	return values
}

// redact returns the secrets with their values blanked, for responses. The
// server never sends a stored secret back: the browser has no use for the value
// and every response is one more place it could leak.
func redact(list []EnvVar) []EnvVar {
	out := make([]EnvVar, len(list))
	for i, item := range list {
		out[i] = EnvVar{Key: item.Key, Value: ""}
	}
	return out
}
