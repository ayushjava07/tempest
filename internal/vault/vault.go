package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
)

type Vault struct {
	mu      sync.RWMutex
	secrets map[string][]byte
	key     []byte
}

func New(key []byte) *Vault {
	if len(key) != 32 {
		panic("vault key must be 32 bytes")
	}
	return &Vault{
		secrets: make(map[string][]byte),
		key:     key,
	}
}

func (v *Vault) Store(ctx context.Context, name string, value []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	encrypted, err := v.encrypt(value)
	if err != nil {
		return err
	}
	v.secrets[name] = encrypted
	return nil
}

func (v *Vault) Retrieve(ctx context.Context, name string) ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	encrypted, ok := v.secrets[name]
	if !ok {
		return nil, fmt.Errorf("secret not found: %s", name)
	}
	return v.decrypt(encrypted)
}

func (v *Vault) Delete(ctx context.Context, name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.secrets, name)
	return nil
}

func (v *Vault) List(ctx context.Context) ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	names := make([]string, 0, len(v.secrets))
	for n := range v.secrets {
		names = append(names, n)
	}
	return names, nil
}

func (v *Vault) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
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
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (v *Vault) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

func EncodeKey(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func DecodeKey(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}