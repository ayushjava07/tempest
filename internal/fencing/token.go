package fencing

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrStaleFencingToken = errors.New("fencing: stale fencing token rejected (zombie write defense)")
	ErrTokenExpired      = errors.New("fencing: fencing token lease has expired")
	ErrPreempted         = errors.New("fencing: lock preemption detected by higher fencing token")
)

// FencingToken represents an immutable authorization grant with a strictly monotonic sequence.
type FencingToken struct {
	Resource  string    `json:"resource"`
	Token     uint64    `json:"token"`
	Owner     string    `json:"owner"`
	Epoch     uint64    `json:"epoch"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (t FencingToken) String() string {
	return fmt.Sprintf("FencingToken[res=%s, token=%d, epoch=%d, owner=%s]", t.Resource, t.Token, t.Epoch, t.Owner)
}

// IsExpired returns true if the token validity window has passed.
func (t FencingToken) IsExpired() bool {
	return time.Now().UTC().After(t.ExpiresAt)
}

// TokenGenerator generates and tracks monotonically increasing fencing tokens across resources.
type TokenGenerator struct {
	mu           sync.RWMutex
	lastToken    map[string]uint64
	epochs       map[string]uint64
	activeTokens map[string]FencingToken
}

// NewTokenGenerator creates a fencing token generator.
func NewTokenGenerator() *TokenGenerator {
	return &TokenGenerator{
		lastToken:    make(map[string]uint64),
		epochs:       make(map[string]uint64),
		activeTokens: make(map[string]FencingToken),
	}
}

// AcquireToken advances the monotonic counter and issues a new fencing token for the resource.
func (g *TokenGenerator) AcquireToken(resource, owner string, ttl time.Duration) FencingToken {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.lastToken[resource]++
	tokNum := g.lastToken[resource]

	epoch := g.epochs[resource]
	if tokNum == 1 {
		epoch = 1
		g.epochs[resource] = 1
	}

	if ttl <= 0 {
		ttl = 30 * time.Second
	}

	now := time.Now().UTC()
	token := FencingToken{
		Resource:  resource,
		Token:     tokNum,
		Owner:     owner,
		Epoch:     epoch,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}

	g.activeTokens[resource] = token
	return token
}

// NextEpoch advances the generation epoch and resets sequence for a resource.
func (g *TokenGenerator) NextEpoch(resource string) uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.epochs[resource]++
	return g.epochs[resource]
}

// LatestToken retrieves the most recently issued token for a resource.
func (g *TokenGenerator) LatestToken(resource string) (FencingToken, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	tok, ok := g.activeTokens[resource]
	return tok, ok
}

// CurrentTokenNumber returns the raw sequence number for a resource.
func (g *TokenGenerator) CurrentTokenNumber(resource string) uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastToken[resource]
}
