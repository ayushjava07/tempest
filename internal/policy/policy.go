package policy

import (
	"fmt"
	"sync"
)

type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

type Statement struct {
	Effect    Effect   `json:"effect"`
	Actions   []string `json:"actions"`
	Resources []string `json:"resources"`
	Conditions map[string]any `json:"conditions,omitempty"`
}

type Policy struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Version    string       `json:"version"`
	Statements []Statement  `json:"statements"`
}

type Evaluator struct {
	mu       sync.RWMutex
	policies map[string]*Policy
}

func NewEvaluator() *Evaluator {
	return &Evaluator{
		policies: make(map[string]*Policy),
	}
}

func (e *Evaluator) AddPolicy(p *Policy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[p.ID] = p
}

func (e *Evaluator) RemovePolicy(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.policies, id)
}

func (e *Evaluator) Evaluate(principal, action, resource string) (Effect, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var hasAllow, hasDeny bool
	for _, p := range e.policies {
		for _, stmt := range p.Statements {
			if !match(stmt.Actions, action) {
				continue
			}
			if !match(stmt.Resources, resource) {
				continue
			}
			if stmt.Effect == EffectDeny {
				hasDeny = true
			} else if stmt.Effect == EffectAllow {
				hasAllow = true
			}
		}
	}
	if hasDeny {
		return EffectDeny, nil
	}
	if hasAllow {
		return EffectAllow, nil
	}
	return EffectDeny, fmt.Errorf("no matching policy")
}

func match(patterns []string, value string) bool {
	for _, p := range patterns {
		if p == "*" || p == value {
			return true
		}
	}
	return false
}

func (e *Evaluator) PolicyCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.policies)
}