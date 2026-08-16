package node

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRespondSwarmDecoratesReachability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	checkedAt := time.Now()
	service := &Service{health: newHealthCache()}
	service.health.set(LocalID, health{
		State:   ReachabilityOnline,
		At:      checkedAt,
		Message: "This is the machine stacker runs on",
	})
	handler := &Handler{service: service}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	handler.respondSwarm(ctx, SwarmResult{Node: Node{ID: LocalID}}, nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response struct {
		Data SwarmResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.Node.Reachability != ReachabilityOnline {
		t.Errorf("reachability = %q, want %q", response.Data.Node.Reachability, ReachabilityOnline)
	}
	if response.Data.Node.ReachabilityMessage == "" {
		t.Error("reachability message is empty")
	}
	if response.Data.Node.ReachableCheckedAt == nil {
		t.Error("reachable checked at is nil")
	}
}

// ssh-copy-id always opens with an INFO banner on stderr, so the naive
// "first line of output" reading reported that banner for every failure and
// told the user nothing. These are real outputs from the failure modes that
// actually come up against a freshly rebuilt host.
func TestCopyIDError(t *testing.T) {
	fallback := errors.New("exit status 1")

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "host key changed is explained, not echoed",
			output: `/usr/bin/ssh-copy-id: INFO: Source of key(s) to be installed: "/tmp/k.pub"
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!`,
			want: "the host key has changed",
		},
		{
			name: "refused password is not reported as the INFO banner",
			output: `/usr/bin/ssh-copy-id: INFO: Source of key(s) to be installed: "/tmp/k.pub"
/usr/bin/ssh-copy-id: INFO: attempting to log in with the new key(s)
stacker@localhost: Permission denied (publickey,password).`,
			want: "the host refused the password",
		},
		{
			name: "an unrecognised error still surfaces its own text",
			output: `/usr/bin/ssh-copy-id: INFO: Source of key(s) to be installed: "/tmp/k.pub"
ssh: connect to host localhost port 2203: Connection refused`,
			want: "Connection refused",
		},
		{
			name:   "no usable output falls back to the process error",
			output: "",
			want:   "exit status 1",
		},
		{
			name:   "banner-only output falls back rather than returning the banner",
			output: `/usr/bin/ssh-copy-id: INFO: Source of key(s) to be installed: "/tmp/k.pub"`,
			want:   "exit status 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := copyIDError(tc.output, fallback)
			if !strings.Contains(got, tc.want) {
				t.Errorf("copyIDError() = %q, want it to contain %q", got, tc.want)
			}
			if strings.Contains(got, "INFO: Source of key(s)") {
				t.Errorf("copyIDError() leaked the INFO banner: %q", got)
			}
		})
	}
}
