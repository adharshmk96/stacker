package node

import (
	"crypto/rand"
	"encoding/hex"
	"os"
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

// localName is the display name for the seeded local node: the machine's
// hostname, or a neutral fallback when the OS will not give one up.
func localName() string {
	host, err := os.Hostname()
	if host = strings.TrimSpace(host); err != nil || host == "" {
		return "This machine"
	}
	return host
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
