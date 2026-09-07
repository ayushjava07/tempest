package deltadiff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// Diff compares source and target data structures and produces an RFC 6902 JSON patch.
func Diff(source, target any) (Patch, error) {
	var patch Patch
	diffRecursive("", source, target, &patch)
	return patch, nil
}

// DiffJSON deserializes two JSON payloads and generates the structural delta patch.
func DiffJSON(sourceJSON, targetJSON []byte) (Patch, error) {
	var src any
	var dst any

	if len(sourceJSON) > 0 {
		if err := json.Unmarshal(sourceJSON, &src); err != nil {
			return nil, fmt.Errorf("failed to parse source JSON: %w", err)
		}
	}
	if len(targetJSON) > 0 {
		if err := json.Unmarshal(targetJSON, &dst); err != nil {
			return nil, fmt.Errorf("failed to parse target JSON: %w", err)
		}
	}

	return Diff(src, dst)
}

func diffRecursive(path string, src, dst any, patch *Patch) {
	if reflect.DeepEqual(src, dst) {
		return
	}

	// Case 1: Nil transitions
	if src == nil && dst != nil {
		*patch = append(*patch, PatchOperation{Op: OpAdd, Path: path, Value: dst})
		return
	}
	if src != nil && dst == nil {
		*patch = append(*patch, PatchOperation{Op: OpRemove, Path: path})
		return
	}

	// Case 2: Object (Map) comparisons
	srcMap, srcIsMap := src.(map[string]any)
	dstMap, dstIsMap := dst.(map[string]any)

	if srcIsMap && dstIsMap {
		diffMaps(path, srcMap, dstMap, patch)
		return
	}

	// Case 3: Array (Slice) comparisons
	srcSlice, srcIsSlice := src.([]any)
	dstSlice, dstIsSlice := dst.([]any)

	if srcIsSlice && dstIsSlice {
		diffSlices(path, srcSlice, dstSlice, patch)
		return
	}

	// Case 4: Primitive value replacement
	*patch = append(*patch, PatchOperation{Op: OpReplace, Path: path, Value: dst})
}

func diffMaps(path string, src, dst map[string]any, patch *Patch) {
	// 1. Check for removals (in src but missing in dst)
	srcKeys := make([]string, 0, len(src))
	for k := range src {
		srcKeys = append(srcKeys, k)
	}
	sort.Strings(srcKeys)

	for _, k := range srcKeys {
		subPath := path + "/" + k
		if _, exists := dst[k]; !exists {
			*patch = append(*patch, PatchOperation{Op: OpRemove, Path: subPath})
		}
	}

	// 2. Check for additions and modifications (in dst)
	dstKeys := make([]string, 0, len(dst))
	for k := range dst {
		dstKeys = append(dstKeys, k)
	}
	sort.Strings(dstKeys)

	for _, k := range dstKeys {
		subPath := path + "/" + k
		dstVal := dst[k]
		srcVal, exists := src[k]

		if !exists {
			*patch = append(*patch, PatchOperation{Op: OpAdd, Path: subPath, Value: dstVal})
		} else {
			diffRecursive(subPath, srcVal, dstVal, patch)
		}
	}
}

func diffSlices(path string, src, dst []any, patch *Patch) {
	minLen := len(src)
	if len(dst) < minLen {
		minLen = len(dst)
	}

	// Common prefix elements
	for i := 0; i < minLen; i++ {
		subPath := fmt.Sprintf("%s/%d", path, i)
		diffRecursive(subPath, src[i], dst[i], patch)
	}

	// Elements added to dst
	if len(dst) > len(src) {
		for i := len(src); i < len(dst); i++ {
			subPath := fmt.Sprintf("%s/%d", path, i)
			*patch = append(*patch, PatchOperation{Op: OpAdd, Path: subPath, Value: dst[i]})
		}
	}

	// Elements removed from src (remove in reverse order to preserve indexing)
	if len(src) > len(dst) {
		for i := len(src) - 1; i >= len(dst); i-- {
			subPath := fmt.Sprintf("%s/%d", path, i)
			*patch = append(*patch, PatchOperation{Op: OpRemove, Path: subPath})
		}
	}
}
