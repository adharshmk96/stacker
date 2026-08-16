package sshkey

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

const rsaBits = 4096

const defaultKeyName = "stacker-default"

// Service owns keypair generation and the key folder on disk. The private half
// only ever leaves this package as a file path.
type Service struct {
	repo   *Repository
	keyDir string
	log    *slog.Logger
}

func NewService(repo *Repository, keyDir string, log *slog.Logger) *Service {
	return &Service{repo: repo, keyDir: keyDir, log: log}
}

func (s *Service) List() ([]SshKey, error) {
	return s.repo.List()
}

func (s *Service) Get(id string) (SshKey, error) {
	return s.repo.Get(id)
}

// EnsureDefault creates the install-wide key on first run. The database flag,
// rather than its display name, is the contract that protects it.
func (s *Service) EnsureDefault() (SshKey, error) {
	key, err := s.repo.GetDefault()
	if err == nil {
		return key, nil
	}
	if err != ErrNotFound {
		return SshKey{}, err
	}

	key, err = s.create(CreateRequest{Name: defaultKeyName, Type: KeyTypeEd25519}, true)
	if err == ErrNameTaken {
		key, err = s.Create(CreateRequest{Name: defaultKeyName + "-key", Type: KeyTypeEd25519})
		if err == nil {
			key.IsDefault = true
			err = s.repo.Save(&key)
		}
	}
	return key, err
}

// PrivateKeyPath resolves the on-disk private key for a stored key. Other
// modules (the node key install) use this to hand `-i <path>` to ssh.
func (s *Service) PrivateKeyPath(id string) (string, error) {
	key, err := s.repo.Get(id)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(key.PrivateKeyPath); err != nil {
		return "", fmt.Errorf("private key file missing for %s: %w", key.Name, err)
	}
	return key.PrivateKeyPath, nil
}

// Create generates a fresh keypair, writes both halves to the key folder and
// records the public half in the database.
func (s *Service) Create(req CreateRequest) (SshKey, error) {
	return s.create(req, false)
}

func (s *Service) create(req CreateRequest, isDefault bool) (SshKey, error) {
	name := strings.TrimSpace(req.Name)
	if err := validateName(name); err != nil {
		return SshKey{}, err
	}

	taken, err := s.repo.ExistsByName(name)
	if err != nil {
		return SshKey{}, err
	}
	if taken {
		return SshKey{}, ErrNameTaken
	}

	id := newID()
	privatePEM, publicKey, err := generate(req.Type, name)
	if err != nil {
		return SshKey{}, err
	}

	privatePath := filepath.Join(s.keyDir, fmt.Sprintf("%s_%s", id, name))
	publicLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + " " + name

	if err := os.WriteFile(privatePath, privatePEM, 0o600); err != nil {
		return SshKey{}, err
	}
	if err := os.WriteFile(privatePath+".pub", []byte(publicLine+"\n"), 0o644); err != nil {
		os.Remove(privatePath)
		return SshKey{}, err
	}

	key := SshKey{
		ID:             id,
		Name:           name,
		Type:           req.Type,
		PublicKey:      publicLine,
		Fingerprint:    fingerprint(publicKey),
		PrivateKeyPath: privatePath,
		IsDefault:      isDefault,
	}

	if err := s.repo.Create(&key); err != nil {
		// Don't leave orphaned key files behind a failed insert.
		os.Remove(privatePath)
		os.Remove(privatePath + ".pub")
		return SshKey{}, err
	}

	s.log.Info("ssh key created", "id", key.ID, "name", key.Name, "type", key.Type)
	return key, nil
}

// Delete removes the record and both key files. A key still referenced by a node
// is refused — deleting it would strand that host.
func (s *Service) Delete(id string) error {
	key, err := s.repo.Get(id)
	if err != nil {
		return err
	}
	if key.IsDefault {
		return ErrDefaultKey
	}

	used, err := s.repo.UsedByNodeCount(id)
	if err != nil {
		return err
	}
	if used > 0 {
		return ErrKeyInUse
	}

	if err := s.repo.Delete(id); err != nil {
		return err
	}

	// Files are best-effort: the record is already gone, so a leftover file is
	// worth a log line but not a failed request.
	for _, path := range []string{key.PrivateKeyPath, key.PrivateKeyPath + ".pub"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			s.log.Warn("could not remove key file", "path", path, "error", err)
		}
	}

	s.log.Info("ssh key deleted", "id", id, "name", key.Name)
	return nil
}

// Rotate replaces the protected default keypair while retaining its id so node
// references stay valid. Existing hosts must receive the new public key.
func (s *Service) Rotate(id string) (SshKey, error) {
	key, err := s.repo.Get(id)
	if err != nil {
		return SshKey{}, err
	}
	if !key.IsDefault {
		return SshKey{}, ErrDefaultKey
	}

	privatePEM, publicKey, err := generate(key.Type, key.Name)
	if err != nil {
		return SshKey{}, err
	}
	publicLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + " " + key.Name
	if err := os.WriteFile(key.PrivateKeyPath, privatePEM, 0o600); err != nil {
		return SshKey{}, err
	}
	if err := os.WriteFile(key.PrivateKeyPath+".pub", []byte(publicLine+"\n"), 0o644); err != nil {
		return SshKey{}, err
	}

	key.PublicKey = publicLine
	key.Fingerprint = fingerprint(publicKey)
	if err := s.repo.Save(&key); err != nil {
		return SshKey{}, err
	}
	s.log.Warn("default ssh key rotated", "id", key.ID)
	return key, nil
}

// generate produces the OpenSSH-format private key bytes and the public key.
func generate(keyType KeyType, comment string) ([]byte, ssh.PublicKey, error) {
	switch keyType {
	case KeyTypeEd25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, ErrKeyGenFailed
		}
		block, err := ssh.MarshalPrivateKey(priv, comment)
		if err != nil {
			return nil, nil, ErrKeyGenFailed
		}
		sshPub, err := ssh.NewPublicKey(pub)
		if err != nil {
			return nil, nil, ErrKeyGenFailed
		}
		return pem.EncodeToMemory(block), sshPub, nil

	case KeyTypeRSA:
		priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
		if err != nil {
			return nil, nil, ErrKeyGenFailed
		}
		block, err := ssh.MarshalPrivateKey(priv, comment)
		if err != nil {
			return nil, nil, ErrKeyGenFailed
		}
		sshPub, err := ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return nil, nil, ErrKeyGenFailed
		}
		return pem.EncodeToMemory(block), sshPub, nil

	default:
		return nil, nil, ErrUnknownType
	}
}
