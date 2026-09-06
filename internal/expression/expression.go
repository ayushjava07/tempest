package expression

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidExpression = errors.New("invalid expression syntax")
	ErrVariableNotFound  = errors.New("variable not found in context")
	ErrTypeMismatch      = errors.New("type mismatch in comparison")
)

// Evaluator evaluates boolean and comparison expressions against a key-value context.
type Evaluator struct{}

// New creates a new Evaluator.
func New() *Evaluator {
	return &Evaluator{}
}

// Evaluate evaluates a string expression against a context map.
// Supported expressions:
// - "true", "false"
// - "a == b", "a != b"
// - "x > y", "x < y", "x >= y", "x <= y" (numeric)
// - expressions can use ${var.name} syntax
func (e *Evaluator) Evaluate(expr string, ctx map[string]interface{}) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil // empty condition defaults to true
	}

	// Resolve all variables in the expression
	resolved, err := e.resolveVariables(expr, ctx)
	if err != nil {
		return false, err
	}

	resolved = strings.TrimSpace(resolved)
	if strings.EqualFold(resolved, "true") {
		return true, nil
	}
	if strings.EqualFold(resolved, "false") {
		return false, nil
	}

	// Handle binary comparisons
	ops := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range ops {
		idx := strings.Index(resolved, op)
		if idx != -1 {
			left := strings.TrimSpace(resolved[:idx])
			right := strings.TrimSpace(resolved[idx+len(op):])
			return e.compare(left, op, right)
		}
	}

	return false, fmt.Errorf("%w: %s", ErrInvalidExpression, expr)
}

func (e *Evaluator) resolveVariables(expr string, ctx map[string]interface{}) (string, error) {
	result := expr
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			return "", fmt.Errorf("%w: unmatched ${", ErrInvalidExpression)
		}
		end += start

		varPath := result[start+2 : end]
		val, err := e.lookupVar(varPath, ctx)
		if err != nil {
			return "", err
		}

		result = result[:start] + fmt.Sprintf("%v", val) + result[end+1:]
	}
	return result, nil
}

func (e *Evaluator) lookupVar(path string, ctx map[string]interface{}) (interface{}, error) {
	parts := strings.Split(path, ".")
	var curr interface{} = ctx

	for _, part := range parts {
		m, ok := curr.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrVariableNotFound, path)
		}
		val, exists := m[part]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrVariableNotFound, path)
		}
		curr = val
	}
	return curr, nil
}

func (e *Evaluator) compare(left, op, right string) (bool, error) {
	// Strip optional quotes
	left = strings.Trim(left, `"'`)
	right = strings.Trim(right, `"'`)

	// Try numeric comparison
	leftNum, errLeft := strconv.ParseFloat(left, 64)
	rightNum, errRight := strconv.ParseFloat(right, 64)

	if errLeft == nil && errRight == nil {
		switch op {
		case "==":
			return leftNum == rightNum, nil
		case "!=":
			return leftNum != rightNum, nil
		case ">":
			return leftNum > rightNum, nil
		case "<":
			return leftNum < rightNum, nil
		case ">=":
			return leftNum >= rightNum, nil
		case "<=":
			return leftNum <= rightNum, nil
		}
	}

	// String comparison
	switch op {
	case "==":
		return left == right, nil
	case "!=":
		return left != right, nil
	case ">":
		return left > right, nil
	case "<":
		return left < right, nil
	case ">=":
		return left >= right, nil
	case "<=":
		return left <= right, nil
	}

	return false, fmt.Errorf("%w: unsupported operator %s", ErrInvalidExpression, op)
}
