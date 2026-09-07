package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSecretNotFound      = errors.New("vault: secret not found")
	ErrProviderUnavailable = errors.New("vault: secret provider unavailable")
	ErrAccessDenied        = errors.New("vault: secret access denied")
	ErrInvalidSecretKey    = errors.New("vault: invalid secret key name")
)

// SecretMetadata annotates a secret with versioning and expiration data.
type SecretMetadata struct {
	Key       string            `json:"key"`
	Version   int               `json:"version"`
	CreatedAt time.Time         `json:"created_at"`
	ExpiresAt *time.Time        `json:"expires_at,omitempty"`
	Custom    map[string]string `json:"custom,omitempty"`
}

// Secret represents a securely retrieved secret payload.
type Secret struct {
	Key      string         `json:"key"`
	Value    []byte         `json:"value"`
	Metadata SecretMetadata `json:"metadata"`
}

// SecretProvider defines the abstraction implemented by backends (Env, File, HashiCorp Vault, AWS KMS).
type SecretProvider interface {
	Name() string
	GetSecret(ctx context.Context, key string) (*Secret, error)
	PutSecret(ctx context.Context, key string, value []byte) error
	DeleteSecret(ctx context.Context, key string) error
	ListSecrets(ctx context.Context) ([]string, error)
}

// ProviderRegistry coordinates multiple registered secret providers with fallback order.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]SecretProvider
	order     []string
}

// NewRegistry creates a provider registry.
func NewRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]SecretProvider),
		order:     make([]string, 0),
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(p SecretProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	if _, exists := r.providers[name]; !exists {
		r.order = append(r.order, name)
	}
	r.providers[name] = p
}

// GetProvider retrieves a provider by name.
func (r *ProviderRegistry) GetProvider(name string) (SecretProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// ResolveSecret searches registered providers in priority order until the key is found.
func (r *ProviderRegistry) ResolveSecret(ctx context.Context, key string) (*Secret, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, name := range r.order {
		p := r.providers[name]
		sec, err := p.GetSecret(ctx, key)
		if err == nil && sec != nil {
			return sec, name, nil
		}
		if err != nil && !errors.Is(err, ErrSecretNotFound) {
			// Non-notfound error encountered
			continue
		}
	}

	return nil, "", fmt.Errorf("%w: key %q not found in any provider", ErrSecretNotFound, key)
}
