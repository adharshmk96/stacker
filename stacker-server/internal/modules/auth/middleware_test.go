package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The websocket fallback is what lets the browser authenticate a connection it
// cannot put a header on. It must apply to handshakes only.
func TestBearerTokenFromWebsocketSubprotocol(t *testing.T) {
	cases := []struct {
		name     string
		upgrade  string
		protocol string
		auth     string
		want     string
	}{
		{
			name:     "handshake carries the token as the second protocol",
			upgrade:  "websocket",
			protocol: "stacker.terminal, session-token",
			want:     "session-token",
		},
		{
			name:     "case of the upgrade header does not matter",
			upgrade:  "WebSocket",
			protocol: "stacker.terminal,session-token",
			want:     "session-token",
		},
		{
			name:     "a protocol list without a token yields nothing",
			upgrade:  "websocket",
			protocol: "stacker.terminal",
			want:     "",
		},
		{
			name:     "an ordinary request never looks at the protocol header",
			protocol: "stacker.terminal, session-token",
			want:     "",
		},
		{
			name:     "the authorization header still wins",
			upgrade:  "websocket",
			protocol: "stacker.terminal, from-protocol",
			auth:     "Bearer from-header",
			want:     "from-header",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/nodes/local/terminal", nil)
			if tc.upgrade != "" {
				req.Header.Set("Upgrade", tc.upgrade)
			}
			if tc.protocol != "" {
				req.Header.Set("Sec-WebSocket-Protocol", tc.protocol)
			}
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = req

			if got := bearerToken(c); got != tc.want {
				t.Errorf("bearerToken = %q, want %q", got, tc.want)
			}
		})
	}
}
