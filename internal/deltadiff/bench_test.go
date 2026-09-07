package deltadiff

import (
	"fmt"
	"testing"
)

func generateLargeMap(items int, mutateKey string) map[string]any {
	m := make(map[string]any, items)
	for i := 0; i < items; i++ {
		k := fmt.Sprintf("entity_%04d", i)
		m[k] = map[string]any{
			"id":      float64(i),
			"name":    fmt.Sprintf("Item %d", i),
			"status":  "ACTIVE",
			"counter": float64(i * 10),
		}
	}
	if mutateKey != "" {
		m[mutateKey] = map[string]any{
			"id":      float64(9999),
			"name":    "Mutated Item",
			"status":  "CHANGED",
			"counter": float64(99990),
		}
	}
	return m
}

func TestFastDiffCorrectness(t *testing.T) {
	src := generateLargeMap(100, "")
	dst := generateLargeMap(100, "entity_0050")

	patchStandard, err := Diff(src, dst)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	patchFast, err := FastDiff(src, dst)
	if err != nil {
		t.Fatalf("FastDiff failed: %v", err)
	}

	if len(patchFast) == 0 {
		t.Fatalf("FastDiff should find mutations")
	}

	// Apply FastDiff patch to src and verify equals dst
	applied, err := Apply(src, patchFast)
	if err != nil {
		t.Fatalf("Apply FastDiff patch failed: %v", err)
	}

	patchAfter, _ := Diff(applied, dst)
	if len(patchAfter) != 0 {
		t.Fatalf("FastDiff patch did not produce exact match, remaining diffs: %v", patchAfter)
	}

	_ = patchStandard
}

func BenchmarkFastDiffIdenticalTrees(b *testing.B) {
	data := generateLargeMap(200, "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = FastDiff(data, data)
	}
}

func BenchmarkStandardDiffIdenticalTrees(b *testing.B) {
	data := generateLargeMap(200, "")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Diff(data, data)
	}
}
