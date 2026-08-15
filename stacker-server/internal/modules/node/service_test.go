package node

import (
	"errors"
	"strings"
	"testing"
)

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
