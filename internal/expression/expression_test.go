package expression

import (
	"testing"
)

func TestEvaluator_LiteralBooleans(t *testing.T) {
	eval := New()

	res, err := eval.Evaluate("true", nil)
	if err != nil || !res {
		t.Errorf("expected true, got %v (err: %v)", res, err)
	}

	res, err = eval.Evaluate("false", nil)
	if err != nil || res {
		t.Errorf("expected false, got %v (err: %v)", res, err)
	}

	res, err = eval.Evaluate("", nil)
	if err != nil || !res {
		t.Errorf("empty expression should default to true")
	}
}

func TestEvaluator_NumericComparisons(t *testing.T) {
	eval := New()

	tests := []struct {
		expr     string
		expected bool
	}{
		{"10 == 10", true},
		{"10 != 10", false},
		{"15 > 10", true},
		{"5 < 10", true},
		{"10 >= 10", true},
		{"9 <= 10", true},
		{"5 > 10", false},
	}

	for _, tc := range tests {
		res, err := eval.Evaluate(tc.expr, nil)
		if err != nil {
			t.Errorf("expr '%s' returned error: %v", tc.expr, err)
		}
		if res != tc.expected {
			t.Errorf("expr '%s': expected %v, got %v", tc.expr, tc.expected, res)
		}
	}
}

func TestEvaluator_ContextVariables(t *testing.T) {
	eval := New()
	ctx := map[string]interface{}{
		"step": map[string]interface{}{
			"status": "success",
			"code":   200,
		},
		"retries": 3,
	}

	res, err := eval.Evaluate("${step.status} == 'success'", ctx)
	if err != nil || !res {
		t.Errorf("expected true for step.status == success, got %v (err: %v)", res, err)
	}

	res, err = eval.Evaluate("${step.code} == 200", ctx)
	if err != nil || !res {
		t.Errorf("expected true for step.code == 200, got %v (err: %v)", res, err)
	}

	res, err = eval.Evaluate("${retries} < 5", ctx)
	if err != nil || !res {
		t.Errorf("expected true for retries < 5, got %v (err: %v)", res, err)
	}

	_, err = eval.Evaluate("${missing.var} == 1", ctx)
	if err == nil {
		t.Errorf("expected error for missing variable, got nil")
	}
}
