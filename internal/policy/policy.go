package policy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidPolicy   = errors.New("policy: invalid policy definition")
	ErrDuplicatePolicy = errors.New("policy: policy already exists")
	ErrPolicyNotFound  = errors.New("policy: policy not found")
)

// Effect defines the authorization decision of a policy statement.
type Effect string

const (
	EffectAllow Effect = "ALLOW"
	EffectDeny  Effect = "DENY"
)

// Decision represents the final evaluation result of an access request.
type Decision struct {
	Allowed   bool
	Effect    Effect
	Reason    string
	MatchedID string
}

// Statement defines a single access control rule with principals, actions, resources, and conditions.
type Statement struct {
	ID         string
	Effect     Effect
	Principals []string            // e.g. ["user:*", "role:operator", "*"]
	Actions    []string            // e.g. ["workflow:create", "run:*", "admin:*"]
	Resources  []string            // e.g. ["workflows/prod/*", "runs/*"]
	Conditions map[string][]string // e.g. {"environment": ["production"], "owner": ["alice"]}
}

// Policy is a named bundle of statements.
type Policy struct {
	ID          string
	Name        string
	Description string
	Statements  []Statement
	CreatedAt   time.Time
}

// Request encapsulates the context of an access evaluation request.
type Request struct {
	Principal  string            // e.g. "user:alice" or "role:operator"
	Action     string            // e.g. "workflow:create"
	Resource   string            // e.g. "workflows/prod/etl-pipeline"
	Attributes map[string]string // e.g. {"environment": "production", "owner": "alice"}
}

// Engine evaluates access control policies against incoming requests.
type Engine struct {
	mu       sync.RWMutex
	policies map[string]Policy
}

// NewEngine constructs a new empty policy evaluation engine.
func NewEngine() *Engine {
	return &Engine{
		policies: make(map[string]Policy),
	}
}

// Register adds or updates a policy definition.
func (e *Engine) Register(p Policy) error {
	if p.ID == "" {
		return fmt.Errorf("%w: policy ID cannot be empty", ErrInvalidPolicy)
	}
	if len(p.Statements) == 0 {
		return fmt.Errorf("%w: policy must contain at least one statement", ErrInvalidPolicy)
	}

	for _, stmt := range p.Statements {
		if stmt.Effect != EffectAllow && stmt.Effect != EffectDeny {
			return fmt.Errorf("%w: statement effect must be ALLOW or DENY", ErrInvalidPolicy)
		}
		if len(stmt.Actions) == 0 {
			return fmt.Errorf("%w: statement must specify at least one action", ErrInvalidPolicy)
		}
		if len(stmt.Resources) == 0 {
			return fmt.Errorf("%w: statement must specify at least one resource", ErrInvalidPolicy)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	e.policies[p.ID] = p
	return nil
}

// Remove deletes a policy by ID.
func (e *Engine) Remove(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, exists := e.policies[id]; !exists {
		return ErrPolicyNotFound
	}
	delete(e.policies, id)
	return nil
}

// List returns all registered policies.
func (e *Engine) List() []Policy {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Policy, 0, len(e.policies))
	for _, p := range e.policies {
		out = append(out, p)
	}
	return out
}

// Evaluate determines whether a request is authorized.
// Precedence rules:
// 1. Explicit DENY overrides any ALLOW.
// 2. An ALLOW must match for the request to succeed.
// 3. Default is DENY (closed-world assumption).
func (e *Engine) Evaluate(ctx context.Context, req Request) (Decision, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	hasAllow := false
	allowReason := ""
	allowMatchedID := ""

	for _, p := range e.policies {
		for _, stmt := range p.Statements {
			if !matchPrincipal(stmt.Principals, req.Principal) {
				continue
			}
			if !matchPatternList(stmt.Actions, req.Action) {
				continue
			}
			if !matchPatternList(stmt.Resources, req.Resource) {
				continue
			}
			if !matchConditions(stmt.Conditions, req.Attributes) {
				continue
			}

			// Match found! Check effect:
			if stmt.Effect == EffectDeny {
				return Decision{
					Allowed:   false,
					Effect:    EffectDeny,
					Reason:    fmt.Sprintf("explicit deny by policy %s (statement %s)", p.ID, stmt.ID),
					MatchedID: stmt.ID,
				}, nil
			}

			if stmt.Effect == EffectAllow {
				hasAllow = true
				allowReason = fmt.Sprintf("allowed by policy %s (statement %s)", p.ID, stmt.ID)
				allowMatchedID = stmt.ID
			}
		}
	}

	if hasAllow {
		return Decision{
			Allowed:   true,
			Effect:    EffectAllow,
			Reason:    allowReason,
			MatchedID: allowMatchedID,
		}, nil
	}

	return Decision{
		Allowed:   false,
		Effect:    EffectDeny,
		Reason:    "default deny: no matching allow statement",
		MatchedID: "",
	}, nil
}

func matchPrincipal(principals []string, target string) bool {
	if len(principals) == 0 {
		return true // Empty principals implies any caller
	}
	for _, p := range principals {
		if p == "*" || p == target {
			return true
		}
		if matchGlob(p, target) {
			return true
		}
	}
	return false
}

func matchPatternList(patterns []string, target string) bool {
	for _, pat := range patterns {
		if pat == "*" || pat == target {
			return true
		}
		if matchGlob(pat, target) {
			return true
		}
	}
	return false
}

func matchGlob(pattern, val string) bool {
	// Support glob matching via path.Match or wildcards
	matched, err := path.Match(pattern, val)
	if err == nil && matched {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.HasPrefix(val, prefix) {
			return true
		}
	}
	return false
}

func matchConditions(conds map[string][]string, attrs map[string]string) bool {
	if len(conds) == 0 {
		return true
	}
	for key, allowedVals := range conds {
		val, exists := attrs[key]
		if !exists {
			return false
		}
		match := false
		for _, av := range allowedVals {
			if av == "*" || av == val || matchGlob(av, val) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// PolicyCache caches evaluation decisions with TTL to reduce policy engine latency.
type PolicyCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	decision  Decision
	expiresAt time.Time
}

func NewPolicyCache(ttl time.Duration) *PolicyCache {
	if ttl <= 0 {
		ttl = 1 * time.Minute
	}
	return &PolicyCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *PolicyCache) makeKey(req Request) string {
	var b strings.Builder
	b.WriteString(req.Principal)
	b.WriteByte('|')
	b.WriteString(req.Action)
	b.WriteByte('|')
	b.WriteString(req.Resource)
	for k, v := range req.Attributes {
		b.WriteByte('|')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
	}
	return b.String()
}

func (c *PolicyCache) Get(req Request) (Decision, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[c.makeKey(req)]
	if !ok || time.Now().After(entry.expiresAt) {
		return Decision{}, false
	}
	return entry.decision, true
}

func (c *PolicyCache) Set(req Request, dec Decision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[c.makeKey(req)] = cacheEntry{
		decision:  dec,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *PolicyCache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]cacheEntry)
}
