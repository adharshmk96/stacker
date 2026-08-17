package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignParseRoundTrip(t *testing.T) {
	secret := []byte("test-secret")
	want := claims{
		SessionID: "sess-1",
		UserID:    "user-1",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}

	token, err := signToken(secret, want)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}

	got, err := parseToken(secret, token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != want {
		t.Fatalf("claims = %+v, want %+v", got, want)
	}
}

func TestParseTokenRejectsBadJWT(t *testing.T) {
	secret := []byte("test-secret")
	valid, err := signToken(secret, claims{
		SessionID: "sid",
		UserID:    "uid",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	tampered := valid[:len(valid)-2] + "xx"

	payload := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	body := jwtHeader + "." + payload
	badJSON := body + "." + base64URL(sign(secret, body))

	invalidB64 := jwtHeader + ".!!!"
	badB64 := invalidB64 + "." + base64URL(sign(secret, invalidB64))

	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "two parts", token: "a.b"},
		{name: "four parts", token: "a.b.c.d"},
		{name: "wrong secret", token: mustSign([]byte("other-secret"), claims{
			SessionID: "sid", UserID: "uid", IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
		})},
		{name: "tampered signature", token: tampered},
		{name: "payload not json", token: badJSON},
		{name: "payload not base64", token: badB64},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseToken(secret, test.token)
			if !errors.Is(err, errBadToken) {
				t.Fatalf("error = %v, want %v", err, errBadToken)
			}
		})
	}
}

func TestParseTokenRejectsExpiry(t *testing.T) {
	secret := []byte("test-secret")
	token, err := signToken(secret, claims{
		SessionID: "sid",
		UserID:    "uid",
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Second).Unix(),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = parseToken(secret, token)
	if !errors.Is(err, errBadToken) {
		t.Fatalf("error = %v, want %v", err, errBadToken)
	}

	zeroExp, err := signToken(secret, claims{SessionID: "sid", UserID: "uid"})
	if err != nil {
		t.Fatalf("sign zero exp: %v", err)
	}
	_, err = parseToken(secret, zeroExp)
	if !errors.Is(err, errBadToken) {
		t.Fatalf("zero exp error = %v, want %v", err, errBadToken)
	}
}

func TestHashToken(t *testing.T) {
	sum := sha256.Sum256([]byte("reset-token"))
	want := hex.EncodeToString(sum[:])
	got := hashToken("reset-token")
	if got != want {
		t.Fatalf("hash = %q, want %q", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
}

func TestNewIDAndResetTokenLengths(t *testing.T) {
	id := newID()
	if len(id) != 24 {
		t.Fatalf("newID length = %d, want 24", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("newID is not hex: %v", err)
	}

	token := newResetToken()
	if len(token) != 64 {
		t.Fatalf("newResetToken length = %d, want 64", len(token))
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("newResetToken is not hex: %v", err)
	}

	if newID() == newID() {
		t.Fatal("newID returned the same value twice")
	}
	if newResetToken() == newResetToken() {
		t.Fatal("newResetToken returned the same value twice")
	}
}

func TestClaimsJSONTags(t *testing.T) {
	raw, err := json.Marshal(claims{SessionID: "s", UserID: "u", IssuedAt: 1, ExpiresAt: 2})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	for _, field := range []string{`"sid"`, `"sub"`, `"iat"`, `"exp"`} {
		if !strings.Contains(text, field) {
			t.Errorf("claims json missing %s: %s", field, text)
		}
	}
}

func mustSign(secret []byte, c claims) string {
	token, err := signToken(secret, c)
	if err != nil {
		panic(err)
	}
	return token
}
