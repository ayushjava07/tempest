package deltadiff

import (
	"encoding/json"
	"hash/fnv"
)

// SubtreeHash computes a 64-bit FNV-1a hash of an object to enable instant unchanged branch skipping.
func SubtreeHash(val any) uint64 {
	if val == nil {
		return 0
	}
	h := fnv.New64a()
	b, err := json.Marshal(val)
	if err != nil {
		return 1
	}
	h.Write(b)
	return h.Sum64()
}

// FastDiff executes structural diffing while pruning identical subtrees using hash checks.
func FastDiff(source, target any) (Patch, error) {
	// Root level check
	if source == nil && target == nil {
		return nil, nil
	}

	hSrc := SubtreeHash(source)
	hDst := SubtreeHash(target)
	if hSrc == hDst {
		// Entire tree is identical, return empty patch instantly
		return Patch{}, nil
	}

	var patch Patch
	fastDiffRecursive("", source, target, &patch)
	return patch, nil
}

func fastDiffRecursive(path string, src, dst any, patch *Patch) {
	// If both non-nil, test hash equivalence to skip unchanged subtree
	if src != nil && dst != nil {
		hSrc := SubtreeHash(src)
		hDst := SubtreeHash(dst)
		if hSrc == hDst {
			return
		}
	}

	diffRecursive(path, src, dst, patch)
}
