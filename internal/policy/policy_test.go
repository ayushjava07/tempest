package policy

import (
	"testing"
)

func TestEvaluator_AddRemove(t *testing.T) {
	e := NewEvaluator()
	p := &Policy{ID: "1", Name: "test", Statements: []Statement{{Effect: EffectAllow, Actions: []string{"*"}, Resources: []string{"*"}}}}
	e.AddPolicy(p)
	if e.PolicyCount() != 1 {
		t.Errorf("expected 1, got %d", e.PolicyCount())
	}
	e.RemovePolicy("1")
	if e.PolicyCount() != 0 {
		t.Error("expected 0")
	}
}

func TestEvaluator_AllowAll(t *testing.T) {
	e := NewEvaluator()
	p := &Policy{ID: "1", Statements: []Statement{{Effect: EffectAllow, Actions: []string{"*"}, Resources: []string{"*"}}}}
	e.AddPolicy(p)
	effect, err := e.Evaluate("user1", "read", "resource1")
	if err != nil {
		t.Fatal(err)
	}
	if effect != EffectAllow {
		t.Errorf("expected allow, got %s", effect)
	}
}

func TestEvaluator_DenyAll(t *testing.T) {
	e := NewEvaluator()
	p := &Policy{ID: "1", Statements: []Statement{{Effect: EffectDeny, Actions: []string{"*"}, Resources: []string{"*"}}}}
	e.AddPolicy(p)
	effect, err := e.Evaluate("user1", "read", "resource1")
	if err != nil {
		t.Fatal(err)
	}
	if effect != EffectDeny {
		t.Errorf("expected deny, got %s", effect)
	}
}

func TestEvaluator_ExplicitDeny(t *testing.T) {
	e := NewEvaluator()
	e.AddPolicy(&Policy{ID: "1", Statements: []Statement{{Effect: EffectAllow, Actions: []string{"read"}, Resources: []string{"*"}}}})
	e.AddPolicy(&Policy{ID: "2", Statements: []Statement{{Effect: EffectDeny, Actions: []string{"read"}, Resources: []string{"secret"}}}})
	effect, err := e.Evaluate("user1", "read", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if effect != EffectDeny {
		t.Errorf("expected deny, got %s", effect)
	}
}

func TestEvaluator_NoMatch(t *testing.T) {
	e := NewEvaluator()
	e.AddPolicy(&Policy{ID: "1", Statements: []Statement{{Effect: EffectAllow, Actions: []string{"write"}, Resources: []string{"*"}}}})
	effect, err := e.Evaluate("user1", "read", "resource1")
	if err == nil {
		t.Error("expected error")
	}
	if effect != EffectDeny {
		t.Errorf("expected deny, got %s", effect)
	}
}

func TestEvaluator_MultipleStatements(t *testing.T) {
	e := NewEvaluator()
	e.AddPolicy(&Policy{
		ID: "1",
		Statements: []Statement{
			{Effect: EffectAllow, Actions: []string{"read"}, Resources: []string{"public"}},
			{Effect: EffectAllow, Actions: []string{"write"}, Resources: []string{"private"}},
		},
	})
	effect, err := e.Evaluate("u", "read", "public")
	if err != nil || effect != EffectAllow {
		t.Error("expected allow for read public")
	}
	effect, err = e.Evaluate("u", "write", "private")
	if err != nil || effect != EffectAllow {
		t.Error("expected allow for write private")
	}
	effect, err = e.Evaluate("u", "read", "private")
	if err == nil || effect != EffectDeny {
		t.Error("expected deny for read private")
	}
}