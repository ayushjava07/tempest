package expr

import (
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestExpr_BasicEvaluations(t *testing.T) {
	env := map[string]interface{}{
		"status":      "FAILED",
		"retry_count": 2,
		"threshold":   5.5,
		"region":      "us-east-1",
		"user": map[string]interface{}{
			"role":    "admin",
			"active":  true,
			"retries": 0,
		},
	}

	tests := []struct {
		expr     string
		expected bool
	}{
		{`$.status == "FAILED"`, true},
		{`$.status == "SUCCEEDED"`, false},
		{`$.retry_count < 3`, true},
		{`$.retry_count >= 5`, false},
		{`$.retry_count + 1 == 3`, true},
		{`$.user.role == "admin" && $.user.active == true`, true},
		{`$.user.role == "guest" || $.status == "FAILED"`, true},
		{`!($.retry_count >= 5)`, true},
		{`$.region in ["us-east-1", "eu-west-1"]`, true},
		{`$.region in ["ap-south-1", "eu-central-1"]`, false},
		{`"us-east" in $.region`, true},
		{`$.threshold > 5 && $.threshold < 6`, true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			res, err := EvaluateBool(tt.expr, env)
			if err != nil {
				t.Fatalf("EvaluateBool(%q) failed: %v", tt.expr, err)
			}
			if res != tt.expected {
				t.Fatalf("EvaluateBool(%q) = %v, expected %v", tt.expr, res, tt.expected)
			}
		})
	}
}

func TestExpr_ArithmeticAndPrecedence(t *testing.T) {
	val, err := Evaluate(`2 + 3 * 4`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.(float64) != 14.0 {
		t.Fatalf("expected 14.0, got %v", val)
	}

	val, err = Evaluate(`(2 + 3) * 4`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val.(float64) != 20.0 {
		t.Fatalf("expected 20.0, got %v", val)
	}
}

func TestExpr_DivisionByZero(t *testing.T) {
	_, err := Evaluate(`10 / 0`, nil)
	if err == nil || err != ErrDivisionByZero {
		t.Fatalf("expected ErrDivisionByZero, got: %v", err)
	}
}

func TestExpr_SyntaxErrors(t *testing.T) {
	invalidExprs := []string{
		`"unterminated string`,
		`(1 + 2`,
		`1 + + 2`,
		`[1, 2,`,
	}

	for _, expr := range invalidExprs {
		t.Run("syntax_"+expr, func(t *testing.T) {
			_, err := Evaluate(expr, nil)
			if err == nil {
				t.Fatalf("expected error for invalid expr %q, got nil", expr)
			}
		})
	}
}

func TestExpr_Concurrency(t *testing.T) {
	env := map[string]interface{}{
		"code": 200,
		"mode": "fast",
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ok, err := EvaluateBool(`$.code == 200 && $.mode == "fast"`, env)
				if err != nil || !ok {
					t.Errorf("evaluation failed concurrently: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}
