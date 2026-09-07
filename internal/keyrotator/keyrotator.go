package keyrotator

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrKeyNotFound     = errors.New("keyrotator: key not found")
	ErrKeyRevoked      = errors.New("keyrotator: key has been cryptographically revoked/destroyed")
	ErrDecryptionFail  = errors.New("keyrotator: authentication or decryption failed")
	ErrNoActiveKey     = errors.New("keyrotator: no active encryption key configured")
	ErrCiphertextShort = errors.New("keyrotator: ciphertext payload too short")
)

type KeyState int

const (
	KeyActive KeyState = iota
	KeyRetired
	KeyRevoked
)

// DataKey represents a versioned data encryption key.
type DataKey struct {
	ID         string
	Version    uint32
	State      KeyState
	RawKey     []byte // AES-256 (32 bytes)
	CreatedAt  time.Time
	RevokedAt  time.Time
	UsageCount atomic.Uint64
}

// Envelope is the self-contained encrypted payload structure.
type Envelope struct {
	Version    uint32 `json:"version"`
	KeyID      string `json:"key_id"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// MarshalBinary serializes envelope to wire format.
func (e *Envelope) MarshalBinary() []byte {
	keyIDBytes := []byte(e.KeyID)
	keyIDLen := uint16(len(keyIDBytes))
	nonceLen := uint16(len(e.Nonce))

	buf := make([]byte, 4+2+len(keyIDBytes)+2+len(e.Nonce)+len(e.Ciphertext))
	binary.BigEndian.PutUint32(buf[0:4], e.Version)
	binary.BigEndian.PutUint16(buf[4:6], keyIDLen)
	copy(buf[6:6+keyIDLen], keyIDBytes)

	offset := 6 + keyIDLen
	binary.BigEndian.PutUint16(buf[offset:offset+2], nonceLen)
	copy(buf[offset+2:offset+2+nonceLen], e.Nonce)

	offset += 2 + nonceLen
	copy(buf[offset:], e.Ciphertext)
	return buf
}

// UnmarshalBinary parses binary envelope.
func UnmarshalBinary(data []byte) (*Envelope, error) {
	if len(data) < 8 {
		return nil, ErrCiphertextShort
	}

	ver := binary.BigEndian.Uint32(data[0:4])
	keyIDLen := int(binary.BigEndian.Uint16(data[4:6]))
	if len(data) < 6+keyIDLen+2 {
		return nil, ErrCiphertextShort
	}

	keyID := string(data[6 : 6+keyIDLen])
	offset := 6 + keyIDLen
	nonceLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	if len(data) < offset+2+nonceLen {
		return nil, ErrCiphertextShort
	}

	offset += 2
	nonce := make([]byte, nonceLen)
	copy(nonce, data[offset:offset+nonceLen])

	offset += nonceLen
	ciphertext := make([]byte, len(data)-offset)
	copy(ciphertext, data[offset:])

	return &Envelope{
		Version:    ver,
		KeyID:      keyID,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// RotationPolicy controls automatic key lifecycle.
type RotationPolicy struct {
	MaxOperations uint64
	MaxAge        time.Duration
}

// Rotator manages the cryptographic envelope lifecycle.
type Rotator struct {
	mu          sync.RWMutex
	masterKEK   []byte // Key Encryption Key
	keys        map[string]*DataKey
	activeKeyID string
	nextVersion uint32
	policy      RotationPolicy
}

// New creates a new Rotator initialized with a 32-byte master key.
func New(masterKEK []byte, policy RotationPolicy) (*Rotator, error) {
	if len(masterKEK) != 32 {
		// Derive 32 bytes via SHA-256 if arbitrary size passed
		h := sha256.Sum256(masterKEK)
		masterKEK = h[:]
	}

	r := &Rotator{
		masterKEK:   append([]byte(nil), masterKEK...),
		keys:        make(map[string]*DataKey),
		policy:      policy,
		nextVersion: 1,
	}

	// Create initial active key
	if _, err := r.Rotate(); err != nil {
		return nil, err
	}

	return r, nil
}

// Rotate creates a new active DEK and marks the previous one as retired.
func (r *Rotator) Rotate() (*DataKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}

	keyIDBytes := make([]byte, 12)
	_, _ = rand.Read(keyIDBytes)
	keyID := fmt.Sprintf("key-v%d-%s", r.nextVersion, hex.EncodeToString(keyIDBytes))

	// Retire old active key
	if r.activeKeyID != "" {
		if old, ok := r.keys[r.activeKeyID]; ok && old.State == KeyActive {
			old.State = KeyRetired
		}
	}

	dk := &DataKey{
		ID:        keyID,
		Version:   r.nextVersion,
		State:     KeyActive,
		RawKey:    raw,
		CreatedAt: time.Now().UTC(),
	}

	r.nextVersion++
	r.keys[keyID] = dk
	r.activeKeyID = keyID

	return dk, nil
}

// Encrypt seals plaintext using the current active DEK.
func (r *Rotator) Encrypt(plaintext, aad []byte) (*Envelope, error) {
	r.mu.RLock()
	activeID := r.activeKeyID
	dk, ok := r.keys[activeID]
	r.mu.RUnlock()

	if !ok || dk.State != KeyActive {
		return nil, ErrNoActiveKey
	}

	block, err := aes.NewCipher(dk.RawKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	ops := dk.UsageCount.Add(1)

	// Check if policy requires auto-rotation
	if r.policy.MaxOperations > 0 && ops >= r.policy.MaxOperations {
		go func() {
			_, _ = r.Rotate()
		}()
	}

	return &Envelope{
		Version:    dk.Version,
		KeyID:      dk.ID,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// Decrypt opens an encrypted envelope.
func (r *Rotator) Decrypt(env *Envelope, aad []byte) ([]byte, error) {
	if env == nil {
		return nil, errors.New("keyrotator: envelope is nil")
	}

	r.mu.RLock()
	dk, ok := r.keys[env.KeyID]
	r.mu.RUnlock()

	if !ok {
		return nil, ErrKeyNotFound
	}
	if dk.State == KeyRevoked {
		return nil, ErrKeyRevoked
	}

	block, err := aes.NewCipher(dk.RawKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, env.Nonce, env.Ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFail, err)
	}

	return plaintext, nil
}

// ReEncrypt decrypts an older envelope and re-seals it under the latest active DEK.
func (r *Rotator) ReEncrypt(env *Envelope, aad []byte) (*Envelope, bool, error) {
	r.mu.RLock()
	activeID := r.activeKeyID
	r.mu.RUnlock()

	// If already on the active key, no migration needed
	if env.KeyID == activeID {
		return env, false, nil
	}

	plaintext, err := r.Decrypt(env, aad)
	if err != nil {
		return nil, false, err
	}

	newEnv, err := r.Encrypt(plaintext, aad)
	if err != nil {
		return nil, false, err
	}

	return newEnv, true, nil
}

// RevokeKey securely destroys a key, rendering all its ciphertexts permanently undecryptable.
func (r *Rotator) RevokeKey(keyID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dk, ok := r.keys[keyID]
	if !ok {
		return ErrKeyNotFound
	}

	dk.State = KeyRevoked
	dk.RevokedAt = time.Now().UTC()
	// Zero out raw key material in memory
	for i := range dk.RawKey {
		dk.RawKey[i] = 0
	}

	if r.activeKeyID == keyID {
		r.activeKeyID = ""
	}

	return nil
}

// ActiveKeyID returns the ID of the current encryption key.
func (r *Rotator) ActiveKeyID() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeKeyID
}
