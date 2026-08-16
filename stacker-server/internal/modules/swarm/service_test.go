package swarm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"stacker/internal/modules/node"
)

// fakeNodes stands in for the node module: a fixed roster, and canned output
// keyed by the docker command it is asked to run.
type fakeNodes struct {
	roster  []node.Node
	replies map[string]string
	fails   map[string]error
	// calls records every command run, so a test can assert on what was sent
	// rather than only on what came back.
	calls []string
}

func (f *fakeNodes) List() ([]node.Node, error) { return f.roster, nil }

func (f *fakeNodes) Manager() (node.Node, error) {
	for _, item := range f.roster {
		if item.SwarmRole == node.SwarmRoleManager {
			return item, nil
		}
	}
	return node.Node{}, errors.New("no manager")
}

func (f *fakeNodes) Docker(_ context.Context, item node.Node, args ...string) (string, error) {
	key := item.Name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)

	if err, ok := f.fails[key]; ok {
		return "", err
	}
	return f.replies[key], nil
}

func (f *fakeNodes) DockerInput(_ context.Context, item node.Node, stdin string, args ...string) (string, error) {
	f.calls = append(f.calls, item.Name+" "+strings.Join(args, " ")+" <<"+stdin)
	return "", nil
}

func newService(nodes *fakeNodes) *Service {
	return NewService(nodes, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func manager(name string) node.Node {
	return node.Node{ID: name, Name: name, SwarmRole: node.SwarmRoleManager, SwarmNodeID: "docker-" + name}
}

func worker(name string) node.Node {
	return node.Node{ID: name, Name: name, SwarmRole: node.SwarmRoleWorker, SwarmNodeID: "docker-" + name}
}

func TestListSwarmScopedAsksTheManager(t *testing.T) {
	nodes := &fakeNodes{
		roster: []node.Node{manager("local"), worker("edge-01")},
		replies: map[string]string{
			"local service ls --format {{json .}}": `{"ID":"a1","Name":"web","Mode":"replicated","Replicas":"2/2","Image":"nginx","Ports":""}`,
		},
	}

	result, err := newService(nodes).List(context.Background(), Services, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(result.Rows))
	}
	if result.Rows[0]["name"] != "web" {
		t.Errorf("name = %q, want web", result.Rows[0]["name"])
	}
	// An empty port list must still render as something.
	if result.Rows[0]["ports"] != "—" {
		t.Errorf("ports = %q, want an em dash", result.Rows[0]["ports"])
	}
	// A swarm-wide list is one call, to the manager, and never to a worker.
	for _, call := range nodes.calls {
		if strings.HasPrefix(call, "edge-01 ") {
			t.Errorf("asked a worker for a swarm-wide list: %q", call)
		}
	}
}

func TestListNodeScopedFansOutAndStampsTheNode(t *testing.T) {
	const argv = " ps -a --format {{json .}}"

	nodes := &fakeNodes{
		roster: []node.Node{manager("local"), worker("edge-01")},
		replies: map[string]string{
			"local" + argv:   `{"ID":"c1","Names":"web.1","Image":"nginx","State":"running","Status":"Up 2 days"}`,
			"edge-01" + argv: `{"ID":"c2","Names":"web.2","Image":"nginx","State":"exited","Status":"Exited (1)"}`,
		},
	}

	result, err := newService(nodes).List(context.Background(), Containers, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(result.Rows))
	}

	seen := map[string]string{}
	for _, row := range result.Rows {
		seen[row["name"]] = row["node"]
		if row["nodeId"] == "" {
			t.Errorf("row %q carries no node id", row["name"])
		}
	}
	if seen["web.1"] != "local" || seen["web.2"] != "edge-01" {
		t.Errorf("rows stamped with the wrong nodes: %v", seen)
	}
}

func TestListNodeScopedNarrowsToOneNode(t *testing.T) {
	const argv = " ps -a --format {{json .}}"

	nodes := &fakeNodes{
		roster: []node.Node{manager("local"), worker("edge-01")},
		replies: map[string]string{
			"edge-01" + argv: `{"ID":"c2","Names":"web.2","Image":"nginx","State":"running","Status":"Up"}`,
		},
	}

	result, err := newService(nodes).List(context.Background(), Containers, "edge-01")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(result.Rows))
	}
	// Narrowing must mean not reaching out to the other hosts at all.
	if len(nodes.calls) != 1 {
		t.Errorf("made %d calls, want 1: %v", len(nodes.calls), nodes.calls)
	}
}

// One unreachable node has to cost its own rows and nothing else.
func TestListNodeScopedKeepsRowsWhenANodeFails(t *testing.T) {
	const argv = " ps -a --format {{json .}}"

	nodes := &fakeNodes{
		roster: []node.Node{manager("local"), worker("edge-01")},
		replies: map[string]string{
			"local" + argv: `{"ID":"c1","Names":"web.1","Image":"nginx","State":"running","Status":"Up"}`,
		},
		fails: map[string]error{
			"edge-01" + argv: fmt.Errorf("%w: %s", node.ErrSwarmCommand, "connection refused"),
		},
	}

	result, err := newService(nodes).List(context.Background(), Containers, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want the one healthy node's", len(result.Rows))
	}
	if len(result.Errors) != 1 || result.Errors[0].Node != "edge-01" {
		t.Fatalf("errors = %+v, want one for edge-01", result.Errors)
	}
	// The wrapper adds nothing the user can act on; docker's own line does.
	if result.Errors[0].Message != "connection refused" {
		t.Errorf("message = %q, want docker's own line", result.Errors[0].Message)
	}
}

// A node that has never been configured has no docker worth asking.
func TestListSkipsNodesOutsideTheSwarm(t *testing.T) {
	nodes := &fakeNodes{
		roster: []node.Node{manager("local"), {ID: "new", Name: "new", SwarmRole: node.SwarmRoleNone}},
	}

	result, err := newService(nodes).List(context.Background(), Containers, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Nodes) != 1 || result.Nodes[0].Name != "local" {
		t.Fatalf("nodes = %+v, want only the configured one", result.Nodes)
	}
	for _, call := range nodes.calls {
		if strings.HasPrefix(call, "new ") {
			t.Errorf("asked an unconfigured node for containers: %q", call)
		}
	}
}

func TestListWithoutAManagerExplainsItself(t *testing.T) {
	_, err := newService(&fakeNodes{}).List(context.Background(), Services, "")
	if !errors.Is(err, ErrNoManager) {
		t.Fatalf("err = %v, want ErrNoManager", err)
	}
}

// Docker has no "list every task" command, so tasks are read per service — and
// with no services there is nothing to ask for.
func TestListTasksWithNoServicesMakesNoTaskCall(t *testing.T) {
	nodes := &fakeNodes{
		roster:  []node.Node{manager("local")},
		replies: map[string]string{"local service ls --format {{.Name}}": ""},
	}

	result, err := newService(nodes).List(context.Background(), Tasks, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("got %d rows, want none", len(result.Rows))
	}
	if len(nodes.calls) != 1 {
		t.Errorf("calls = %v, want only the service list", nodes.calls)
	}
}

// A task's node is docker's hostname, which need not be what stacker calls it.
func TestListTasksRewritesTheNodeToStackersName(t *testing.T) {
	nodes := &fakeNodes{
		roster: []node.Node{{
			ID: "n1", Name: "edge-01", SwarmRole: node.SwarmRoleManager, SwarmNodeID: "abc123",
		}},
		replies: map[string]string{
			"edge-01 service ls --format {{.Name}}":                 "web",
			"edge-01 service ps --no-trunc --format {{json .}} web": `{"ID":"t1","Name":"web.1","Node":"ip-10-0-0-4","DesiredState":"Running","CurrentState":"Running 2 hours ago","Error":""}`,
			"edge-01 node ls --format {{.ID}}\t{{.Hostname}}":       "abc123\tip-10-0-0-4",
		},
	}

	result, err := newService(nodes).List(context.Background(), Tasks, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(result.Rows))
	}
	if got := result.Rows[0]["node"]; got != "edge-01" {
		t.Errorf("node = %q, want stacker's name", got)
	}
	if got := result.Rows[0]["state"]; got != "running" {
		t.Errorf("state = %q, want the first word of the current state, lowercased", got)
	}
}

func TestActionBuildsTheDockerCommand(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local")}}

	result, err := newService(nodes).Action(context.Background(), Services,
		ActionRequest{Action: "remove", ID: "web"})
	if err != nil {
		t.Fatalf("Action: %v", err)
	}
	if result.Message != "Removed the service web" {
		t.Errorf("message = %q", result.Message)
	}
	if want := "local service rm web"; nodes.calls[0] != want {
		t.Errorf("ran %q, want %q", nodes.calls[0], want)
	}
}

// The action table is the whole of what stacker will run; anything else is a
// client asking for something that was never offered.
func TestActionRefusesAnythingOutsideTheTable(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local")}}

	_, err := newService(nodes).Action(context.Background(), Services,
		ActionRequest{Action: "exec", ID: "web"})
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
	if len(nodes.calls) != 0 {
		t.Errorf("ran %v, want nothing", nodes.calls)
	}
}

func TestScaleNeedsAReplicaCount(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local")}}

	_, err := newService(nodes).Action(context.Background(), Services,
		ActionRequest{Action: "scale", ID: "web"})
	if !errors.Is(err, ErrReplicasNeeded) {
		t.Fatalf("err = %v, want ErrReplicasNeeded", err)
	}
}

func TestScaleSendsTheTarget(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local")}}
	three := 3

	if _, err := newService(nodes).Action(context.Background(), Services,
		ActionRequest{Action: "scale", ID: "web", Replicas: &three}); err != nil {
		t.Fatalf("Action: %v", err)
	}
	if want := "local service scale --detach=true web=3"; nodes.calls[0] != want {
		t.Errorf("ran %q, want %q", nodes.calls[0], want)
	}
}

// A container lives on one host, so acting on one without saying which host is
// a request that cannot be carried out — not one to guess at.
func TestNodeScopedActionNeedsANode(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local")}}

	_, err := newService(nodes).Action(context.Background(), Containers,
		ActionRequest{Action: "restart", ID: "c1"})
	if !errors.Is(err, ErrNodeRequired) {
		t.Fatalf("err = %v, want ErrNodeRequired", err)
	}
}

func TestNodeScopedActionRunsOnThatNode(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local"), worker("edge-01")}}

	if _, err := newService(nodes).Action(context.Background(), Containers,
		ActionRequest{Action: "restart", ID: "c1", Node: "edge-01"}); err != nil {
		t.Fatalf("Action: %v", err)
	}
	if want := "edge-01 restart c1"; nodes.calls[0] != want {
		t.Errorf("ran %q, want %q", nodes.calls[0], want)
	}
}

// A secret's value must never reach a process list or a shell history.
func TestCreateSecretSendsTheValueOnStdin(t *testing.T) {
	nodes := &fakeNodes{roster: []node.Node{manager("local")}}

	if _, err := newService(nodes).Create(context.Background(), Secrets,
		CreateRequest{Name: "db_password", Content: "hunter2"}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	call := nodes.calls[0]
	if !strings.HasPrefix(call, "local secret create db_password - <<") {
		t.Fatalf("ran %q", call)
	}
	if !strings.HasSuffix(call, "<<hunter2") {
		t.Errorf("the value did not go in on stdin: %q", call)
	}
}

func TestCreateChecksItsRequiredFields(t *testing.T) {
	service := newService(&fakeNodes{roster: []node.Node{manager("local")}})

	if _, err := service.Create(context.Background(), Services, CreateRequest{Name: "web"}); !errors.Is(err, ErrImageRequired) {
		t.Errorf("err = %v, want ErrImageRequired", err)
	}
	if _, err := service.Create(context.Background(), Stacks, CreateRequest{Content: "services: {}"}); !errors.Is(err, ErrNameRequired) {
		t.Errorf("err = %v, want ErrNameRequired", err)
	}
}

func TestUnknownResourceIsRefused(t *testing.T) {
	_, err := newService(&fakeNodes{}).List(context.Background(), Resource("widgets"), "")
	if !errors.Is(err, ErrUnknownResource) {
		t.Fatalf("err = %v, want ErrUnknownResource", err)
	}
}

// Docker prints warnings on the same stream as the JSON it was asked for, and
// one of those must not cost the user their table.
func TestParseLinesSkipsNonJSONOutput(t *testing.T) {
	out := "WARNING: bridge-nf-call-iptables is disabled\n" +
		`{"Name":"monitoring","Services":"4","Orchestrator":"Swarm"}` + "\n"

	rows := parseLines(out, specs[Stacks])
	if len(rows) != 1 || rows[0]["name"] != "monitoring" {
		t.Fatalf("rows = %+v", rows)
	}
}

// Docker prints Go's default time layout, which browsers cannot parse.
func TestRecordDateNormalisesDockerTimestamps(t *testing.T) {
	rec := record{"CreatedAt": "2026-08-14 12:00:00 +0000 UTC", "Other": "not a date"}

	if got, want := rec.date("CreatedAt"), "2026-08-14T12:00:00Z"; got != want {
		t.Errorf("date = %q, want %q", got, want)
	}
	// An unrecognised value is passed through rather than dropped.
	if got := rec.date("Other"); got != "not a date" {
		t.Errorf("date = %q, want the value untouched", got)
	}
}

// An untagged layer left behind by a rebuild is noise in a list meant to
// answer "what can run here".
func TestImageListDropsDanglingLayers(t *testing.T) {
	out := `{"Repository":"<none>","Tag":"<none>","ID":"a1","Size":"10MB"}` + "\n" +
		`{"Repository":"nginx","Tag":"alpine","ID":"b2","Size":"40MB"}`

	rows := parseLines(out, specs[Images])
	if len(rows) != 1 || rows[0]["repository"] != "nginx" {
		t.Fatalf("rows = %+v", rows)
	}
}
