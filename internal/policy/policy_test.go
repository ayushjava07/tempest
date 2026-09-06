package policy

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestPolicy_ExplicitDenyOverridesAllow(t *testing.T) {
	engine := NewEngine()

	// Policy 1: Allow all actions on workflows/*
	err := engine.Register(Policy{
		ID:   "allow-workflows",
		Name: "Allow Workflows",
		Statements: []Statement{
			{
				ID:         "stmt-allow",
				Effect:     EffectAllow,
				Principals: []string{"role:developer"},
				Actions:    []string{"workflow:*"},
				Resources:  []string{"workflows/*"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Policy 2: Explicitly Deny deletion
	err = engine.Register(Policy{
		ID:   "deny-deletions",
		Name: "Deny Deletions",
		Statements: []Statement{
			{
				ID:         "stmt-deny",
				Effect:     EffectDeny,
				Principals: []string{"role:developer"},
				Actions:    []string{"workflow:delete"},
				Resources:  []string{"workflows/*"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx := context.Background()

	// Should be allowed to read
	dec, err := engine.Evaluate(ctx, Request{
		Principal: "role:developer",
		Action:    "workflow:read",
		Resource:  "workflows/my-workflow",
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if !dec.Allowed || dec.Effect != EffectAllow {
		t.Errorf("expected ALLOW for read, got %v (%s)", dec.Effect, dec.Reason)
	}

	// Should be explicitly DENIED to delete
	dec, err = engine.Evaluate(ctx, Request{
		Principal: "role:developer",
		Action:    "workflow:delete",
		Resource:  "workflows/my-workflow",
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if dec.Allowed || dec.Effect != EffectDeny {
		t.Errorf("expected DENY for delete, got %v (%s)", dec.Effect, dec.Reason)
	}
}

func TestPolicy_DefaultDeny(t *testing.T) {
	engine := NewEngine()

	// Register policy for a different principal
	_ = engine.Register(Policy{
		ID: "operator-only",
		Statements: []Statement{
			{
				ID:         "s1",
				Effect:     EffectAllow,
				Principals: []string{"role:operator"},
				Actions:    []string{"*"},
				Resources:  []string{"*"},
			},
		},
	})

	dec, err := engine.Evaluate(context.Background(), Request{
		Principal: "user:stranger",
		Action:    "workflow:create",
		Resource:  "workflows/test",
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if dec.Allowed || dec.Effect != EffectDeny {
		t.Errorf("expected default DENY, got %v", dec)
	}
}

func TestPolicy_Conditions(t *testing.T) {
	engine := NewEngine()

	err := engine.Register(Policy{
		ID: "prod-restricted",
		Statements: []Statement{
			{
				ID:         "stmt-cond",
				Effect:     EffectAllow,
				Principals: []string{"user:alice"},
				Actions:    []string{"run:trigger"},
				Resources:  []string{"runs/prod/*"},
				Conditions: map[string][]string{
					"mfa_verified": {"true"},
					"ip_subnet":    {"10.0.*"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ctx := context.Background()

	// Case 1: All conditions met
	dec, err := engine.Evaluate(ctx, Request{
		Principal: "user:alice",
		Action:    "run:trigger",
		Resource:  "runs/prod/wf-1",
		Attributes: map[string]string{
			"mfa_verified": "true",
			"ip_subnet":    "10.0.1.5",
		},
	})
	if err != nil || !dec.Allowed {
		t.Errorf("expected ALLOW when conditions match, got %v (%v)", dec, err)
	}

	// Case 2: Missing mfa_verified
	dec, err = engine.Evaluate(ctx, Request{
		Principal: "user:alice",
		Action:    "run:trigger",
		Resource:  "runs/prod/wf-1",
		Attributes: map[string]string{
			"mfa_verified": "false",
			"ip_subnet":    "10.0.1.5",
		},
	})
	if err != nil || dec.Allowed {
		t.Errorf("expected DENY when MFA false, got %v", dec)
	}

	// Case 3: Out of subnet
	dec, err = engine.Evaluate(ctx, Request{
		Principal: "user:alice",
		Action:    "run:trigger",
		Resource:  "runs/prod/wf-1",
		Attributes: map[string]string{
			"mfa_verified": "true",
			"ip_subnet":    "192.168.1.1",
		},
	})
	if err != nil || dec.Allowed {
		t.Errorf("expected DENY when subnet mismatch, got %v", dec)
	}
}

func TestPolicy_ValidationAndRemoval(t *testing.T) {
	engine := NewEngine()

	// Empty ID
	err := engine.Register(Policy{ID: ""})
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("expected ErrInvalidPolicy, got %v", err)
	}

	// No statements
	err = engine.Register(Policy{ID: "p1", Statements: nil})
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("expected ErrInvalidPolicy for empty statements, got %v", err)
	}

	// Removal
	_ = engine.Register(Policy{
		ID: "p-remove",
		Statements: []Statement{
			{ID: "s1", Effect: EffectAllow, Actions: []string{"*"}, Resources: []string{"*"}},
		},
	})
	if len(engine.List()) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(engine.List()))
	}
	if err := engine.Remove("p-remove"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if len(engine.List()) != 0 {
		t.Fatalf("expected 0 policies after remove, got %d", len(engine.List()))
	}
}

func TestPolicyCache(t *testing.T) {
	cache := NewPolicyCache(50 * time.Millisecond)

	req := Request{
		Principal: "user:alice",
		Action:    "workflow:read",
		Resource:  "workflows/etl",
		Attributes: map[string]string{
			"env": "prod",
		},
	}

	_, ok := cache.Get(req)
	if ok {
		t.Fatal("expected cache miss initially")
	}

	cache.Set(req, Decision{Allowed: true, Effect: EffectAllow})

	dec, ok := cache.Get(req)
	if !ok || !dec.Allowed {
		t.Fatalf("expected cache hit with allowed=true")
	}

	// Wait for TTL expiration
	time.Sleep(70 * time.Millisecond)
	_, ok = cache.Get(req)
	if ok {
		t.Fatal("expected cache expiration after TTL")
	}
}

func TestPolicy_Concurrency(t *testing.T) {
	engine := NewEngine()

	_ = engine.Register(Policy{
		ID: "concurrent-policy",
		Statements: []Statement{
			{
				ID:         "s1",
				Effect:     EffectAllow,
				Principals: []string{"user:*"},
				Actions:    []string{"workflow:*"},
				Resources:  []string{"workflows/*"},
			},
		},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := Request{
				Principal: fmt.Sprintf("user:worker-%d", idx),
				Action:    "workflow:read",
				Resource:  fmt.Sprintf("workflows/job-%d", idx),
			}
			dec, err := engine.Evaluate(context.Background(), req)
			if err != nil || !dec.Allowed {
				t.Errorf("evaluation failed in worker %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}
