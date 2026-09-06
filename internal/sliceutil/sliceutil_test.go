package sliceutil

import (
	"strconv"
	"testing"
)

func TestContains(t *testing.T) {
	if !Contains([]int{1, 2, 3}, 2) {
		t.Error("expected contains")
	}
	if Contains([]int{1, 2, 3}, 4) {
		t.Error("expected not contains")
	}
}

func TestIndex(t *testing.T) {
	if idx := Index([]string{"a", "b", "c"}, "b"); idx != 1 {
		t.Errorf("expected 1, got %d", idx)
	}
	if idx := Index([]int{1, 2}, 3); idx != -1 {
		t.Error("expected -1")
	}
}

func TestUnique(t *testing.T) {
	result := Unique([]int{1, 2, 2, 3, 1})
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}
}

func TestFilter(t *testing.T) {
	result := Filter([]int{1, 2, 3, 4, 5}, func(v int) bool { return v%2 == 0 })
	if len(result) != 2 || result[0] != 2 || result[1] != 4 {
		t.Error("unexpected filter result")
	}
}

func TestMap(t *testing.T) {
	result := Map([]int{1, 2, 3}, func(v int) string { return strconv.Itoa(v) })
	if len(result) != 3 || result[0] != "1" {
		t.Error("unexpected map result")
	}
}

func TestReduce(t *testing.T) {
	sum := Reduce([]int{1, 2, 3}, 0, func(acc, v int) int { return acc + v })
	if sum != 6 {
		t.Errorf("expected 6, got %d", sum)
	}
}

func TestAny(t *testing.T) {
	if !Any([]int{1, 2, 3}, func(v int) bool { return v == 2 }) {
		t.Error("expected true")
	}
	if Any([]int{1, 2, 3}, func(v int) bool { return v == 4 }) {
		t.Error("expected false")
	}
}

func TestAll(t *testing.T) {
	if !All([]int{2, 4, 6}, func(v int) bool { return v%2 == 0 }) {
		t.Error("expected true")
	}
	if All([]int{2, 3, 6}, func(v int) bool { return v%2 == 0 }) {
		t.Error("expected false")
	}
}

func TestChunk(t *testing.T) {
	result := Chunk([]int{1, 2, 3, 4, 5}, 2)
	if len(result) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(result))
	}
}

func TestFlatten(t *testing.T) {
	result := Flatten([][]int{{1, 2}, {3, 4}})
	if len(result) != 4 {
		t.Errorf("expected 4, got %d", len(result))
	}
}

func TestReverse(t *testing.T) {
	result := Reverse([]int{1, 2, 3})
	if result[0] != 3 || result[2] != 1 {
		t.Error("expected reversed")
	}
}

func TestDrop(t *testing.T) {
	result := Drop([]int{1, 2, 3, 4}, 2)
	if len(result) != 2 || result[0] != 3 {
		t.Error("unexpected drop result")
	}
}

func TestTake(t *testing.T) {
	result := Take([]int{1, 2, 3, 4}, 2)
	if len(result) != 2 || result[1] != 2 {
		t.Error("unexpected take result")
	}
}

func TestGroupBy(t *testing.T) {
	groups := GroupBy([]int{1, 2, 3, 4, 5, 6}, func(v int) string {
		if v%2 == 0 {
			return "even"
		}
		return "odd"
	})
	if len(groups["even"]) != 3 || len(groups["odd"]) != 3 {
		t.Error("unexpected grouping")
	}
}

func TestDrop_ExceedsLength(t *testing.T) {
	result := Drop([]int{1}, 5)
	if result != nil {
		t.Error("expected nil")
	}
}

func TestTake_ExceedsLength(t *testing.T) {
	result := Take([]int{1, 2}, 10)
	if len(result) != 2 {
		t.Error("expected full slice")
	}
}
