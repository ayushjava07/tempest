package deltadiff

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidPointer = errors.New("deltadiff: invalid JSON pointer syntax")
	ErrPathNotFound   = errors.New("deltadiff: path not found in target structure")
	ErrPatchFailed    = errors.New("deltadiff: patch operation failed")
	ErrTestFailed     = errors.New("deltadiff: patch test operation failed condition")
)

// OpType represents an RFC 6902 JSON Patch operation.
type OpType string

const (
	OpAdd     OpType = "add"
	OpRemove  OpType = "remove"
	OpReplace OpType = "replace"
	OpMove    OpType = "move"
	OpCopy    OpType = "copy"
	OpTest    OpType = "test"
)

// PatchOperation defines a single discrete mutation on a JSON data structure.
type PatchOperation struct {
	Op    OpType `json:"op"`
	Path  string `json:"path"`
	From  string `json:"from,omitempty"`
	Value any    `json:"value,omitempty"`
}

// Patch encapsulates an ordered sequence of patch operations.
type Patch []PatchOperation

// MarshalJSON returns the JSON encoding of the patch.
func (p Patch) MarshalJSON() ([]byte, error) {
	return json.Marshal([]PatchOperation(p))
}

// UnmarshalJSON parses a JSON patch document.
func (p *Patch) UnmarshalJSON(data []byte) error {
	var ops []PatchOperation
	if err := json.Unmarshal(data, &ops); err != nil {
		return err
	}
	for i, op := range ops {
		if !isValidOp(op.Op) {
			return fmt.Errorf("invalid op at index %d: %q", i, op.Op)
		}
		if !strings.HasPrefix(op.Path, "/") && op.Path != "" {
			return fmt.Errorf("%w: path %q must begin with '/'", ErrInvalidPointer, op.Path)
		}
	}
	*p = ops
	return nil
}

func isValidOp(op OpType) bool {
	switch op {
	case OpAdd, OpRemove, OpReplace, OpMove, OpCopy, OpTest:
		return true
	default:
		return false
	}
}

// ParsePointer tokens splits an RFC 6901 JSON pointer into unescaped path segments.
func ParsePointer(ptr string) ([]string, error) {
	if ptr == "" {
		return []string{}, nil
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, fmt.Errorf("%w: pointer %q must start with '/'", ErrInvalidPointer, ptr)
	}

	rawTokens := strings.Split(ptr[1:], "/")
	tokens := make([]string, len(rawTokens))
	for i, tok := range rawTokens {
		// RFC 6901 unescaping: ~1 -> /, ~0 -> ~
		tok = strings.ReplaceAll(tok, "~1", "/")
		tok = strings.ReplaceAll(tok, "~0", "~")
		tokens[i] = tok
	}
	return tokens, nil
}

// FormatPointer tokens joins path segments into an RFC 6901 JSON pointer.
func FormatPointer(tokens ...string) string {
	if len(tokens) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, tok := range tokens {
		sb.WriteString("/")
		escaped := strings.ReplaceAll(tok, "~", "~0")
		escaped = strings.ReplaceAll(escaped, "/", "~1")
		sb.WriteString(escaped)
	}
	return sb.String()
}
