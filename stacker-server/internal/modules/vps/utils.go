package vps

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
)

// sshPattern matches `user@host` — host may be a hostname or an IP.
var sshPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+$`)

func validateSsh(value string) error {
	if !sshPattern.MatchString(strings.TrimSpace(value)) {
		return ErrInvalidSsh
	}
	return nil
}

// splitSsh pulls the user and host halves apart for the ssh command line.
func splitSsh(value string) (user, host string) {
	parts := strings.SplitN(strings.TrimSpace(value), "@", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
