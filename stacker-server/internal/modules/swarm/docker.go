package swarm

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// jsonLines is the format every list command is asked for: docker prints one
// JSON object per line, which streams cleanly over ssh and needs no column
// parsing. `--format json` would be shorter but only newer CLIs accept it,
// while `{{json .}}` has worked for as long as swarm has existed.
const jsonLines = "{{json .}}"

// listSpec describes how one resource is read from docker: the command to run,
// where to run it, and how to turn docker's fields into the row the UI renders.
type listSpec struct {
	scope Scope
	// argv is the docker subcommand, without the leading "docker".
	argv []string
	// row maps one decoded docker record onto a row. Returning false drops the
	// record — used to hide the rows docker reports but the list should not.
	row func(rec record) (Row, bool)
}

// record is one decoded `{{json .}}` object. Docker writes every field as a
// string, but a stray number or bool would break a map[string]string decode, so
// the values are read loosely and read back through get.
type record map[string]any

func (r record) get(key string) string {
	value, ok := r[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// date reads a docker timestamp and returns it as RFC3339 so the browser can
// parse it. Docker prints Go's default time layout, which `new Date()` does not
// understand; an unrecognised value is passed through rather than dropped.
func (r record) date(key string) string {
	value := r.get(key)
	if value == "" {
		return ""
	}

	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700 -0700",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return value
}

// dash is what an empty cell shows, so a column never renders as blank space
// the user has to guess the meaning of.
func dash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

var specs = map[Resource]listSpec{
	Stacks: {
		scope: ScopeSwarm,
		argv:  []string{"stack", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"name":         rec.get("Name"),
				"services":     rec.get("Services"),
				"orchestrator": dash(rec.get("Orchestrator")),
			}, true
		},
	},
	Services: {
		scope: ScopeSwarm,
		argv:  []string{"service", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"id":       rec.get("ID"),
				"name":     rec.get("Name"),
				"mode":     rec.get("Mode"),
				"replicas": rec.get("Replicas"),
				"image":    rec.get("Image"),
				"ports":    dash(rec.get("Ports")),
			}, true
		},
	},
	Networks: {
		scope: ScopeSwarm,
		argv:  []string{"network", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"id":        rec.get("ID"),
				"name":      rec.get("Name"),
				"driver":    rec.get("Driver"),
				"scope":     rec.get("Scope"),
				"createdAt": rec.date("CreatedAt"),
			}, true
		},
	},
	// Secrets and configs are the one pair docker reports relative times for
	// ("2 hours ago") rather than timestamps, so their dates are passed through
	// as the words docker chose.
	Secrets: {
		scope: ScopeSwarm,
		argv:  []string{"secret", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"id":        rec.get("ID"),
				"name":      rec.get("Name"),
				"createdAt": rec.get("CreatedAt"),
				"updatedAt": rec.get("UpdatedAt"),
			}, true
		},
	},
	Configs: {
		scope: ScopeSwarm,
		argv:  []string{"config", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"id":        rec.get("ID"),
				"name":      rec.get("Name"),
				"createdAt": rec.get("CreatedAt"),
				"updatedAt": rec.get("UpdatedAt"),
			}, true
		},
	},
	Containers: {
		scope: ScopeNode,
		// -a so stopped containers are listed too: a container that exited is
		// exactly the one the user came here to look at.
		argv: []string{"ps", "-a", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"id":     rec.get("ID"),
				"name":   rec.get("Names"),
				"image":  rec.get("Image"),
				"state":  rec.get("State"),
				"status": rec.get("Status"),
				"ports":  dash(rec.get("Ports")),
			}, true
		},
	},
	Images: {
		scope: ScopeNode,
		argv:  []string{"image", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			// An untagged layer left behind by a rebuild is noise in a list
			// meant to answer "what can run here".
			if rec.get("Repository") == "<none>" {
				return nil, false
			}
			return Row{
				"id":         rec.get("ID"),
				"repository": rec.get("Repository"),
				"tag":        rec.get("Tag"),
				"size":       rec.get("Size"),
				"createdAt":  rec.date("CreatedAt"),
			}, true
		},
	},
	Volumes: {
		scope: ScopeNode,
		argv:  []string{"volume", "ls", "--format", jsonLines},
		row: func(rec record) (Row, bool) {
			return Row{
				"name":       rec.get("Name"),
				"driver":     rec.get("Driver"),
				"mountpoint": rec.get("Mountpoint"),
				"scope":      rec.get("Scope"),
			}, true
		},
	},
	// Tasks are read per service rather than by a plain list command, so they
	// carry no argv; listTasks builds the call. The entry still exists so the
	// resource is recognised and its scope is known.
	Tasks: {scope: ScopeSwarm},
}

// taskRow maps one `docker service ps` record. Tasks are the one list whose
// state is worth splitting: docker reports a desired and a current state, and a
// task that is "Running" but desired "Shutdown" is mid-replacement.
func taskRow(rec record) Row {
	return Row{
		"id":      rec.get("ID"),
		"name":    rec.get("Name"),
		"node":    rec.get("Node"),
		"state":   strings.ToLower(firstWord(rec.get("CurrentState"))),
		"current": rec.get("CurrentState"),
		"desired": strings.ToLower(rec.get("DesiredState")),
		"image":   rec.get("Image"),
		"error":   dash(rec.get("Error")),
	}
}

// parseLines decodes docker's one-object-per-line output.
//
// A line that will not decode is skipped rather than failing the list: docker
// prints warnings ("WARNING: bridge-nf-call disabled") on the same stream, and
// one of those must not cost the user their table.
func parseLines(out string, spec listSpec) []Row {
	rows := []Row{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var rec record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if row, ok := spec.row(rec); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func firstWord(value string) string {
	if index := strings.IndexByte(value, ' '); index != -1 {
		return value[:index]
	}
	return value
}

// scaleArg renders the `service=count` argument `docker service scale` takes.
func scaleArg(name string, replicas int) string {
	return name + "=" + strconv.Itoa(replicas)
}
