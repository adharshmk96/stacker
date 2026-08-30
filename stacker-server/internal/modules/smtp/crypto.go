package smtp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// keyFile is the name of the file under the server's key directory that holds
// the AES-256 key used to encrypt the SMTP password at rest. It lives beside
// the SSH host keys, which get the same 0700/0600 treatment.
const keyFile = "smtp.key"

// loadOrCreateKey reads the persisted encryption key, generating one on first
// use. Keeping it on disk rather than in the database means a copy of the
// sqlite file alone does not hand out the SMTP password.
func loadOrCreateKey(keyDir string) ([]byte, error) {
	path := filepath.Join(keyDir, keyFile)

	if data, err := os.ReadFile(path); err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("smtp: encryption key at %s is corrupt", path)
		}
		return data, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// encrypt seals plaintext with AES-256-GCM, returning a base64 string of
// nonce||ciphertext. An empty plaintext encrypts to an empty string so a
// cleared password stays represented as "".
func encrypt(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decrypt reverses encrypt. An empty ciphertext decrypts to "".
func decrypt(key []byte, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("smtp: ciphertext too short")
	}
	nonce, sealed := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
