package providers

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

// cachedEntry stores an in-memory encrypted secret and its expiration deadline.
type cachedEntry struct {
	encrypted []byte
	metadata  SecretMetadata
	expiresAt time.Time
}

// CachedProvider wraps an underlying SecretProvider with encrypted in-memory caching and jittered TTLs.
type CachedProvider struct {
	backend       SecretProvider
	ttl           time.Duration
	jitterPercent float64
	mu            sync.RWMutex
	cache         map[string]cachedEntry
	cipherKey     []byte

	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

// NewCachedProvider initializes a caching wrapper around a secret provider.
func NewCachedProvider(backend SecretProvider, ttl time.Duration, jitterPercent float64, key []byte) (*CachedProvider, error) {
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("failed to generate cache encryption key: %w", err)
		}
	} else if len(key) != 32 {
		return nil, fmt.Errorf("cache encryption key must be 32 bytes")
	}

	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if jitterPercent < 0 || jitterPercent > 0.5 {
		jitterPercent = 0.1
	}

	return &CachedProvider{
		backend:       backend,
		ttl:           ttl,
		jitterPercent: jitterPercent,
		cache:         make(map[string]cachedEntry),
		cipherKey:     key,
	}, nil
}

func (c *CachedProvider) Name() string {
	return fmt.Sprintf("cached(%s)", c.backend.Name())
}

func (c *CachedProvider) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.cipherKey)
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
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *CachedProvider) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.cipherKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("cached ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

func (c *CachedProvider) calculateTTL() time.Duration {
	base := float64(c.ttl)
	jitterRange := base * c.jitterPercent
	if jitterRange <= 0 {
		return c.ttl
	}

	// Calculate random jitter offset within [-jitterRange, +jitterRange]
	maxBig := big.NewInt(int64(2 * jitterRange))
	n, err := rand.Int(rand.Reader, maxBig)
	if err != nil {
		return c.ttl
	}
	offset := float64(n.Int64()) - jitterRange
	return time.Duration(base + offset)
}

// GetSecret checks in-memory cache first, falling back to backend on miss or expiration.
func (c *CachedProvider) GetSecret(ctx context.Context, key string) (*Secret, error) {
	c.mu.RLock()
	entry, found := c.cache[key]
	c.mu.RUnlock()

	now := time.Now()
	if found && now.Before(entry.expiresAt) {
		plain, err := c.decrypt(entry.encrypted)
		if err == nil {
			c.hits.Add(1)
			return &Secret{
				Key:      key,
				Value:    plain,
				Metadata: entry.metadata,
			}, nil
		}
	}

	c.misses.Add(1)

	// Fetch from upstream backend
	secret, err := c.backend.GetSecret(ctx, key)
	if err != nil {
		return nil, err
	}

	// Encrypt and store in cache
	encrypted, err := c.encrypt(secret.Value)
	if err != nil {
		return nil, err
	}

	ttl := c.calculateTTL()
	expiresAt := now.Add(ttl)

	c.mu.Lock()
	c.cache[key] = cachedEntry{
		encrypted: encrypted,
		metadata:  secret.Metadata,
		expiresAt: expiresAt,
	}
	c.mu.Unlock()

	return secret, nil
}

// PutSecret writes through to backend and invalidates cache entry.
func (c *CachedProvider) PutSecret(ctx context.Context, key string, value []byte) error {
	if err := c.backend.PutSecret(ctx, key, value); err != nil {
		return err
	}
	c.Invalidate(key)
	return nil
}

// DeleteSecret deletes from backend and removes from cache.
func (c *CachedProvider) DeleteSecret(ctx context.Context, key string) error {
	if err := c.backend.DeleteSecret(ctx, key); err != nil {
		return err
	}
	c.Invalidate(key)
	return nil
}

// ListSecrets delegates to underlying backend.
func (c *CachedProvider) ListSecrets(ctx context.Context) ([]string, error) {
	return c.backend.ListSecrets(ctx)
}

// Invalidate purges an entry from memory cache.
func (c *CachedProvider) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.cache[key]; exists {
		delete(c.cache, key)
		c.evictions.Add(1)
	}
}

// InvalidateAll clears the entire memory cache.
func (c *CachedProvider) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictions.Add(uint64(len(c.cache)))
	c.cache = make(map[string]cachedEntry)
}

// Stats returns hit, miss, and eviction counts.
func (c *CachedProvider) Stats() (hits, misses, evictions uint64) {
	return c.hits.Load(), c.misses.Load(), c.evictions.Load()
}
