package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	contextUser    = "auth.user"
	contextSession = "auth.session"
)

// RequireAuth rejects a request that does not carry a live session. Every module
// but /api/auth's public endpoints is mounted behind it.
func (m *Module) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": ErrUnauthorized.Error()})
			return
		}

		user, session, err := m.Service.Authenticate(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": ErrUnauthorized.Error()})
			return
		}

		c.Set(contextUser, user)
		c.Set(contextSession, session)
		c.Next()
	}
}

// bearerToken reads the token from the Authorization header, falling back to
// the websocket subprotocol list.
//
// The fallback exists because a browser cannot set headers on a websocket: the
// only field it controls is `Sec-WebSocket-Protocol`, so clients send
// `<name>, <token>` there and the endpoint echoes the name back to complete the
// handshake. Nothing else about the check changes — it is the same session
// token, read from the one place the browser can put it.
func bearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if len(header) > 7 && strings.EqualFold(header[:7], "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return websocketToken(c)
}

// websocketToken pulls the token out of the subprotocol list, which is only
// looked at on an actual websocket handshake.
func websocketToken(c *gin.Context) string {
	if !strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return ""
	}

	parts := strings.Split(c.GetHeader("Sec-WebSocket-Protocol"), ",")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// CurrentUser returns the user RequireAuth put on the context.
func CurrentUser(c *gin.Context) (User, bool) {
	value, ok := c.Get(contextUser)
	if !ok {
		return User{}, false
	}
	user, ok := value.(User)
	return user, ok
}

// CurrentSession returns the session RequireAuth put on the context.
func CurrentSession(c *gin.Context) (Session, bool) {
	value, ok := c.Get(contextSession)
	if !ok {
		return Session{}, false
	}
	session, ok := value.(Session)
	return session, ok
}
