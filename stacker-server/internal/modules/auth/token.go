package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Session JWTs are HS256 signed by hand rather than through a library: the
// claim set is three fields and the key never leaves this process, so a
// dependency would buy nothing.

var errBadToken = errors.New("malformed token")

// claims is the JWT payload. `sid` is the whole point — the token asserts a
// session id, and the session row decides whether it is still valid.
type claims struct {
	SessionID string `json:"sid"`
	UserID    string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

var jwtHeader = base64URL([]byte(`{"alg":"HS256","typ":"JWT"}`))

// signToken renders a signed JWT for a session.
func signToken(secret []byte, c claims) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	body := jwtHeader + "." + base64URL(payload)
	return body + "." + base64URL(sign(secret, body)), nil
}

// parseToken verifies the signature and expiry and returns the claims. It says
// nothing about whether the session is still active — that is a database
// question, answered in Service.Authenticate.
func parseToken(secret []byte, token string) (claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims{}, errBadToken
	}

	body := parts[0] + "." + parts[1]
	expected := base64URL(sign(secret, body))
	// Constant time, so a wrong signature cannot be narrowed down by timing.
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return claims{}, errBadToken
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims{}, errBadToken
	}

	var c claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return claims{}, errBadToken
	}
	if time.Now().Unix() >= c.ExpiresAt {
		return claims{}, errBadToken
	}
	return c, nil
}

func sign(secret []byte, body string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(body))
	return mac.Sum(nil)
}

func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// newID returns a short random hex id, the same shape ssh keys and nodes use.
func newID() string {
	return randomHex(12)
}

// newResetToken returns the secret half of a password reset link. It is longer
// than an id because it travels outside the database.
func newResetToken() string {
	return randomHex(32)
}

// hashToken is the at-rest form of a reset token. A plain sha256 is right here
// (unlike for passwords): the input is 256 bits of entropy, so there is nothing
// to brute force and nothing to gain from a slow hash.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand only fails catastrophically; nothing sensible to recover to.
		panic(err)
	}
	return hex.EncodeToString(buf)
}
