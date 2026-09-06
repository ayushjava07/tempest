package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	ErrKeyNotFound       = errors.New("vault: key version not found on keyring")
	ErrInvalidKeyLength  = errors.New("vault: AES-256 key must be exactly 32 bytes")
	ErrSecretNotFound    = errors.New("vault: secret path not found")
	ErrCiphertextCorrupt = errors.New("vault: ciphertext authentication tag mismatch")
)

// EncryptedSecret represents an encrypted payload tagged with its encryption key version.
type EncryptedSecret struct {
	Path       string
	KeyVersion uint32
	Nonce      []byte
	Ciphertext []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// KeyRing maintains multiple key versions to facilitate zero-downtime rotation.
type KeyRing struct {
	mu        sync.RWMutex
	keys      map[uint32][]byte
	activeVer uint32
}

func NewKeyRing(initialKey []byte) (*KeyRing, error) {
	if len(initialKey) != 32 {
		return nil, ErrInvalidKeyLength
	}
	kr := &KeyRing{
		keys:      make(map[uint32][]byte),
		activeVer: 1,
	}
	kr.keys[1] = append([]byte(nil), initialKey...)
	return kr, nil
}

// Rotate adds a new primary key version for subsequent encryptions.
func (kr *KeyRing) Rotate(newKey []byte) (uint32, error) {
	if len(newKey) != 32 {
		return 0, ErrInvalidKeyLength
	}

	kr.mu.Lock()
	defer kr.mu.Unlock()

	kr.activeVer++
	kr.keys[kr.activeVer] = append([]byte(nil), newKey...)
	return kr.activeVer, nil
}

// ActiveKey returns the currently active encryption key and its version.
func (kr *KeyRing) ActiveKey() (uint32, []byte) {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	return kr.activeVer, append([]byte(nil), kr.keys[kr.activeVer]...)
}

// GetKey retrieves a specific key version for decryption.
func (kr *KeyRing) GetKey(version uint32) ([]byte, error) {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	k, ok := kr.keys[version]
	if !ok {
		return nil, ErrKeyNotFound
	}
	return append([]byte(nil), k...), nil
}

// Vault manages encryption, decryption, and storage of workflow secrets.
type Vault struct {
	mu      sync.RWMutex
	keyRing *KeyRing
	secrets map[string]*EncryptedSecret
}

func NewVault(keyRing *KeyRing) *Vault {
	return &Vault{
		keyRing: keyRing,
		secrets: make(map[string]*EncryptedSecret),
	}
}

// Put encrypts and stores a secret at the designated path using the active key.
func (v *Vault) Put(ctx context.Context, path string, plaintext []byte) (*EncryptedSecret, error) {
	if path == "" {
		return nil, errors.New("vault: secret path cannot be empty")
	}

	ver, key := v.keyRing.ActiveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("read nonce: %w", err)
	}

	// Encrypt with path as authenticated additional data (AAD)
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(path))

	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now().UTC()
	sec := &EncryptedSecret{
		Path:       path,
		KeyVersion: ver,
		Nonce:      nonce,
		Ciphertext: ciphertext,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if existing, ok := v.secrets[path]; ok {
		sec.CreatedAt = existing.CreatedAt
	}

	v.secrets[path] = sec
	return sec, nil
}

// Get decrypts the secret stored at path.
func (v *Vault) Get(ctx context.Context, path string) ([]byte, error) {
	v.mu.RLock()
	sec, ok := v.secrets[path]
	v.mu.RUnlock()

	if !ok {
		return nil, ErrSecretNotFound
	}

	key, err := v.keyRing.GetKey(sec.KeyVersion)
	if err != nil {
		return nil, fmt.Errorf("retrieve key v%d: %w", sec.KeyVersion, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, sec.Nonce, sec.Ciphertext, []byte(path))
	if err != nil {
		return nil, ErrCiphertextCorrupt
	}

	return plaintext, nil
}

// Reencrypt re-encrypts an existing secret with the current active key version.
func (v *Vault) Reencrypt(ctx context.Context, path string) error {
	plaintext, err := v.Get(ctx, path)
	if err != nil {
		return err
	}
	_, err = v.Put(ctx, path, plaintext)
	return err
}

// RotateAndReencrypt rotates the keyring with newKey and re-encrypts all stored secrets.
func (v *Vault) RotateAndReencrypt(ctx context.Context, newKey []byte) (int, error) {
	_, err := v.keyRing.Rotate(newKey)
	if err != nil {
		return 0, fmt.Errorf("rotate key: %w", err)
	}

	v.mu.RLock()
	var paths []string
	for p := range v.secrets {
		paths = append(paths, p)
	}
	v.mu.RUnlock()

	reencrypted := 0
	for _, p := range paths {
		if err := v.Reencrypt(ctx, p); err != nil {
			return reencrypted, fmt.Errorf("re-encrypt %s: %w", p, err)
		}
		reencrypted++
	}

	return reencrypted, nil
}
