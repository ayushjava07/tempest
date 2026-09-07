package providers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const (
	RedactedPlaceholder = "[REDACTED:SECRET]"
	MinMaskingLength    = 4
)

// SecretMaskingBridge tracks sensitive strings fetched from providers and redacts them from logs and traces.
type SecretMaskingBridge struct {
	mu             sync.RWMutex
	knownSecrets   map[string]struct{}
	sortedPatterns []string
	dirty          bool
}

// NewMaskingBridge creates a masking bridge.
func NewMaskingBridge() *SecretMaskingBridge {
	return &SecretMaskingBridge{
		knownSecrets:   make(map[string]struct{}),
		sortedPatterns: make([]string, 0),
	}
}

// RegisterSecret adds a sensitive value to the active masking dictionary if long enough.
func (b *SecretMaskingBridge) RegisterSecret(val []byte) {
	str := strings.TrimSpace(string(val))
	if len(str) < MinMaskingLength {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.knownSecrets[str]; !exists {
		b.knownSecrets[str] = struct{}{}
		b.dirty = true
	}
}

func (b *SecretMaskingBridge) rebuildPatternsLocked() {
	if !b.dirty {
		return
	}
	b.sortedPatterns = make([]string, 0, len(b.knownSecrets))
	for s := range b.knownSecrets {
		b.sortedPatterns = append(b.sortedPatterns, s)
	}
	// Sort longest first so longer secrets are matched and replaced before substrings
	sort.Slice(b.sortedPatterns, func(i, j int) bool {
		return len(b.sortedPatterns[i]) > len(b.sortedPatterns[j])
	})
	b.dirty = false
}

// MaskText scans input text and replaces all known secret strings with the redaction placeholder.
func (b *SecretMaskingBridge) MaskText(input string) string {
	if input == "" {
		return ""
	}

	b.mu.Lock()
	b.rebuildPatternsLocked()
	patterns := append([]string(nil), b.sortedPatterns...)
	b.mu.Unlock()

	res := input
	for _, secret := range patterns {
		if strings.Contains(res, secret) {
			res = strings.ReplaceAll(res, secret, RedactedPlaceholder)
		}
	}
	return res
}

// SecretCount returns the count of unique registered secrets.
func (b *SecretMaskingBridge) SecretCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.knownSecrets)
}

// MaskingSecretProvider wraps any SecretProvider and auto-registers retrieved secrets with the bridge.
type MaskingSecretProvider struct {
	backend SecretProvider
	bridge  *SecretMaskingBridge
}

// NewMaskingSecretProvider decorates a SecretProvider with automatic secret leak masking.
func NewMaskingSecretProvider(backend SecretProvider, bridge *SecretMaskingBridge) *MaskingSecretProvider {
	if bridge == nil {
		bridge = NewMaskingBridge()
	}
	return &MaskingSecretProvider{
		backend: backend,
		bridge:  bridge,
	}
}

func (m *MaskingSecretProvider) Name() string {
	return fmt.Sprintf("masked(%s)", m.backend.Name())
}

func (m *MaskingSecretProvider) Bridge() *SecretMaskingBridge {
	return m.bridge
}

func (m *MaskingSecretProvider) GetSecret(ctx context.Context, key string) (*Secret, error) {
	sec, err := m.backend.GetSecret(ctx, key)
	if err != nil {
		return nil, err
	}
	if sec != nil && len(sec.Value) > 0 {
		m.bridge.RegisterSecret(sec.Value)
	}
	return sec, nil
}

func (m *MaskingSecretProvider) PutSecret(ctx context.Context, key string, value []byte) error {
	m.bridge.RegisterSecret(value)
	return m.backend.PutSecret(ctx, key, value)
}

func (m *MaskingSecretProvider) DeleteSecret(ctx context.Context, key string) error {
	return m.backend.DeleteSecret(ctx, key)
}

func (m *MaskingSecretProvider) ListSecrets(ctx context.Context) ([]string, error) {
	return m.backend.ListSecrets(ctx)
}
