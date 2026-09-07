package expression

import (
	"fmt"
	"strconv"
	"strings"
)

type Evaluator struct {
	variables map[string]any
}

func New() *Evaluator {
	return &Evaluator{
		variables: make(map[string]any),
	}
}

func (e *Evaluator) SetVariable(name string, value any) {
	e.variables[name] = value
}

func (e *Evaluator) GetVariable(name string) (any, bool) {
	v, ok := e.variables[name]
	return v, ok
}

func (e *Evaluator) Evaluate(expr string) (any, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}
	if strings.HasPrefix(expr, "$") {
		name := expr[1:]
		if v, ok := e.variables[name]; ok {
			return v, nil
		}
		return nil, fmt.Errorf("variable not found: %s", name)
	}
	if v, err := strconv.Atoi(expr); err == nil {
		return v, nil
	}
	if v, err := strconv.ParseFloat(expr, 64); err == nil {
		return v, nil
	}
	if expr == "true" || expr == "false" {
		return expr == "true", nil
	}
	if len(expr) >= 2 && expr[0] == '"' && expr[len(expr)-1] == '"' {
		return expr[1 : len(expr)-1], nil
	}
	return nil, fmt.Errorf("unsupported expression: %s", expr)
}

func (e *Evaluator) EvaluateBool(expr string) (bool, error) {
	v, err := e.Evaluate(expr)
	if err != nil {
		return false, err
	}
	switch val := v.(type) {
	case bool:
		return val, nil
	case int:
		return val != 0, nil
	case float64:
		return val != 0, nil
	case string:
		return val != "", nil
	default:
		return false, fmt.Errorf("cannot convert to bool: %T", v)
	}
}

func (e *Evaluator) EvaluateInt(expr string) (int, error) {
	v, err := e.Evaluate(expr)
	if err != nil {
		return 0, err
	}
	switch val := v.(type) {
	case int:
		return val, nil
	case float64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, fmt.Errorf("cannot convert to int: %T", v)
	}
}