package deltadiff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// Apply applies an RFC 6902 JSON patch to a Go object structure.
func Apply(target any, patch Patch) (any, error) {
	current := target

	for i, op := range patch {
		var err error
		current, err = applyOperation(current, op)
		if err != nil {
			return nil, fmt.Errorf("patch failed at operation %d (%s %s): %w", i, op.Op, op.Path, err)
		}
	}

	return current, nil
}

// ApplyJSON applies a patch to a JSON document and returns the mutated JSON bytes.
func ApplyJSON(targetJSON []byte, patch Patch) ([]byte, error) {
	var target any
	if len(targetJSON) > 0 {
		if err := json.Unmarshal(targetJSON, &target); err != nil {
			return nil, fmt.Errorf("failed to parse target JSON: %w", err)
		}
	}

	result, err := Apply(target, patch)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func applyOperation(target any, op PatchOperation) (any, error) {
	tokens, err := ParsePointer(op.Path)
	if err != nil {
		return nil, err
	}

	switch op.Op {
	case OpTest:
		val, err := getValue(target, tokens)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(val, op.Value) {
			return nil, fmt.Errorf("%w: at %s expected %v, got %v", ErrTestFailed, op.Path, op.Value, val)
		}
		return target, nil

	case OpAdd:
		return addValue(target, tokens, op.Value)

	case OpRemove:
		return removeValue(target, tokens)

	case OpReplace:
		// Replace is equivalent to remove then add
		t, err := removeValue(target, tokens)
		if err != nil {
			return nil, err
		}
		return addValue(t, tokens, op.Value)

	case OpCopy:
		fromTokens, err := ParsePointer(op.From)
		if err != nil {
			return nil, err
		}
		val, err := getValue(target, fromTokens)
		if err != nil {
			return nil, err
		}
		return addValue(target, tokens, val)

	case OpMove:
		fromTokens, err := ParsePointer(op.From)
		if err != nil {
			return nil, err
		}
		val, err := getValue(target, fromTokens)
		if err != nil {
			return nil, err
		}
		t, err := removeValue(target, fromTokens)
		if err != nil {
			return nil, err
		}
		return addValue(t, tokens, val)

	default:
		return nil, fmt.Errorf("%w: unknown op %q", ErrPatchFailed, op.Op)
	}
}

func getValue(target any, tokens []string) (any, error) {
	if len(tokens) == 0 {
		return target, nil
	}

	curr := target
	for _, tok := range tokens {
		switch node := curr.(type) {
		case map[string]any:
			v, exists := node[tok]
			if !exists {
				return nil, fmt.Errorf("%w: key %q", ErrPathNotFound, tok)
			}
			curr = v
		case []any:
			idx, err := strconv.Atoi(tok)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf("%w: invalid array index %q", ErrPathNotFound, tok)
			}
			curr = node[idx]
		default:
			return nil, fmt.Errorf("%w: cannot traverse into primitive", ErrPathNotFound)
		}
	}
	return curr, nil
}

func addValue(target any, tokens []string, val any) (any, error) {
	if len(tokens) == 0 {
		return val, nil
	}

	if target == nil {
		target = make(map[string]any)
	}

	parentTokens := tokens[:len(tokens)-1]
	lastToken := tokens[len(tokens)-1]

	parent, err := getValue(target, parentTokens)
	if err != nil {
		return nil, err
	}

	switch node := parent.(type) {
	case map[string]any:
		node[lastToken] = val
		return target, nil

	case []any:
		if lastToken == "-" {
			// Append to array
			parentTokensParent, _ := getParentSliceContainer(target, parentTokens)
			if parentTokensParent != nil {
				// update slice in parent
			}
			node = append(node, val)
			return updateSliceInParent(target, parentTokens, node)
		}

		idx, err := strconv.Atoi(lastToken)
		if err != nil || idx < 0 || idx > len(node) {
			return nil, fmt.Errorf("%w: array insert index %q out of range", ErrPatchFailed, lastToken)
		}

		// Insert at idx
		newSlice := make([]any, 0, len(node)+1)
		newSlice = append(newSlice, node[:idx]...)
		newSlice = append(newSlice, val)
		newSlice = append(newSlice, node[idx:]...)
		return updateSliceInParent(target, parentTokens, newSlice)

	default:
		return nil, fmt.Errorf("%w: cannot add to non-container", ErrPatchFailed)
	}
}

func removeValue(target any, tokens []string) (any, error) {
	if len(tokens) == 0 {
		return nil, nil
	}

	parentTokens := tokens[:len(tokens)-1]
	lastToken := tokens[len(tokens)-1]

	parent, err := getValue(target, parentTokens)
	if err != nil {
		return nil, err
	}

	switch node := parent.(type) {
	case map[string]any:
		if _, exists := node[lastToken]; !exists {
			return nil, fmt.Errorf("%w: key %q not found", ErrPathNotFound, lastToken)
		}
		delete(node, lastToken)
		return target, nil

	case []any:
		idx, err := strconv.Atoi(lastToken)
		if err != nil || idx < 0 || idx >= len(node) {
			return nil, fmt.Errorf("%w: remove index %q out of range", ErrPathNotFound, lastToken)
		}
		newSlice := append(node[:idx], node[idx+1:]...)
		return updateSliceInParent(target, parentTokens, newSlice)

	default:
		return nil, fmt.Errorf("%w: cannot remove from non-container", ErrPatchFailed)
	}
}

func getParentSliceContainer(target any, tokens []string) (any, error) {
	return getValue(target, tokens)
}

func updateSliceInParent(root any, parentTokens []string, newSlice []any) (any, error) {
	if len(parentTokens) == 0 {
		return newSlice, nil
	}

	grandParentTokens := parentTokens[:len(parentTokens)-1]
	parentKey := parentTokens[len(parentTokens)-1]

	grandParent, err := getValue(root, grandParentTokens)
	if err != nil {
		return nil, err
	}

	switch gp := grandParent.(type) {
	case map[string]any:
		gp[parentKey] = newSlice
		return root, nil
	case []any:
		idx, err := strconv.Atoi(parentKey)
		if err != nil {
			return nil, err
		}
		gp[idx] = newSlice
		return root, nil
	default:
		return nil, fmt.Errorf("unexpected grandparent type")
	}
}
