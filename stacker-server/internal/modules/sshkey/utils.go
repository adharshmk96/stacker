package sshkey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateName keeps names safe to use as a filename — the key files are stored
// as <name>/<id> on disk and the name ends up in the public key comment.
func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || !namePattern.MatchString(name) {
		return ErrInvalidName
	}
	return nil
}

// newID returns a short random hex id.
func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand only fails catastrophically; nothing sensible to recover to.
		panic(err)
	}
	return hex.EncodeToString(buf)
}

// fingerprint renders the SHA256 fingerprint the way `ssh-keygen -lf` does:
// base64, no padding, prefixed with "SHA256:".
func fingerprint(pub ssh.PublicKey) string {
	sum := sha256.Sum256(pub.Marshal())
	return "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(sum[:]), "=")
}
