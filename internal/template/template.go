package template

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrUnclosedDelimiter = errors.New("template: unclosed delimiter '{{'")
	ErrInvalidExpression = errors.New("template: invalid expression syntax")
	ErrKeyNotFound       = errors.New("template: key not found in context")
)

// Context provides variables available during template evaluation.
type Context struct {
	Input map[string]any
	Steps map[string]map[string]any
	RunID string
	Env   map[string]string
}

// Engine parses and renders templates containing {{ variable | filter }} expressions.
type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

// Render evaluates a template string against the provided Context.
func (e *Engine) Render(tmpl string, ctx Context) (string, error) {
	var b strings.Builder
	idx := 0

	for idx < len(tmpl) {
		start := strings.Index(tmpl[idx:], "{{")
		if start == -1 {
			b.WriteString(tmpl[idx:])
			break
		}
		start += idx
		b.WriteString(tmpl[idx:start])

		end := strings.Index(tmpl[start+2:], "}}")
		if end == -1 {
			return "", ErrUnclosedDelimiter
		}
		end += start + 2

		expr := strings.TrimSpace(tmpl[start+2 : end])
		val, err := e.evalExpression(expr, ctx)
		if err != nil {
			return "", err
		}
		b.WriteString(val)

		idx = end + 2
	}

	return b.String(), nil
}

// RenderObject deep-evaluates strings within a JSON-compatible map or slice structure.
func (e *Engine) RenderObject(val any, ctx Context) (any, error) {
	switch v := val.(type) {
	case string:
		// If string is exactly "{{ expr }}", return typed value rather than converting to string
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 {
			expr := strings.TrimSpace(trimmed[2 : len(trimmed)-2])
			raw, err := e.evalRawExpression(expr, ctx)
			if err == nil {
				return raw, nil
			}
		}
		return e.Render(v, ctx)
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			renderedKey, err := e.Render(k, ctx)
			if err != nil {
				return nil, err
			}
			renderedVal, err := e.RenderObject(item, ctx)
			if err != nil {
				return nil, err
			}
			out[renderedKey] = renderedVal
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			renderedItem, err := e.RenderObject(item, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = renderedItem
		}
		return out, nil
	default:
		return val, nil
	}
}

func (e *Engine) evalExpression(expr string, ctx Context) (string, error) {
	val, err := e.evalRawExpression(expr, ctx)
	if err != nil {
		return "", err
	}
	return formatValue(val), nil
}

func (e *Engine) evalRawExpression(expr string, ctx Context) (any, error) {
	// Parse pipeline: expr | filter1 | filter2
	parts := strings.Split(expr, "|")
	rawKey := strings.TrimSpace(parts[0])

	val, err := resolveVariable(rawKey, ctx)
	if err != nil && len(parts) == 1 {
		return nil, err
	}

	for _, pipe := range parts[1:] {
		filter := strings.TrimSpace(pipe)
		val = applyFilter(filter, val)
	}

	return val, nil
}

func resolveVariable(path string, ctx Context) (any, error) {
	if path == "run.id" {
		return ctx.RunID, nil
	}

	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil, ErrInvalidExpression
	}

	switch parts[0] {
	case "input":
		return lookupMap(ctx.Input, parts[1:])
	case "env":
		if len(parts) != 2 {
			return nil, ErrInvalidExpression
		}
		v, ok := ctx.Env[parts[1]]
		if !ok {
			return nil, fmt.Errorf("%w: env %s", ErrKeyNotFound, parts[1])
		}
		return v, nil
	case "steps":
		if len(parts) < 3 {
			return nil, ErrInvalidExpression
		}
		stepID := parts[1]
		stepData, ok := ctx.Steps[stepID]
		if !ok {
			return nil, fmt.Errorf("%w: step %s", ErrKeyNotFound, stepID)
		}
		return lookupMap(stepData, parts[2:])
	default:
		return nil, fmt.Errorf("%w: unknown root identifier %s", ErrKeyNotFound, parts[0])
	}
}

func lookupMap(data map[string]any, keys []string) (any, error) {
	if len(keys) == 0 {
		return data, nil
	}
	var current any = data
	for _, k := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: expected map at %s", ErrKeyNotFound, k)
		}
		val, exists := m[k]
		if !exists {
			return nil, fmt.Errorf("%w: %s", ErrKeyNotFound, k)
		}
		current = val
	}
	return current, nil
}

func applyFilter(filter string, val any) any {
	if strings.HasPrefix(filter, "default(") && strings.HasSuffix(filter, ")") {
		def := filter[8 : len(filter)-1]
		def = strings.Trim(def, "\"'")
		if val == nil || val == "" {
			return def
		}
		return val
	}

	switch filter {
	case "upper":
		return strings.ToUpper(fmt.Sprint(val))
	case "lower":
		return strings.ToLower(fmt.Sprint(val))
	case "trim":
		return strings.TrimSpace(fmt.Sprint(val))
	case "json":
		b, err := json.Marshal(val)
		if err != nil {
			return "{}"
		}
		return string(b)
	default:
		return val
	}
}

func formatValue(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}
