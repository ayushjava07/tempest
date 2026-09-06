package bloomfilter

import (
	"fmt"
	"sync"
	"testing"
)

func TestBloomFilter_AddContains(t *testing.T) {
	bf := New(1024, 3)
	bf.Add([]byte("hello"))
	if !bf.Contains([]byte("hello")) {
		t.Error("expected contains")
	}
	if bf.Contains([]byte("world")) {
		t.Error("expected not contains")
	}
}

func TestBloomFilter_MultipleItems(t *testing.T) {
	bf := New(1024, 3)
	items := []string{"apple", "banana", "cherry", "date"}
	for _, item := range items {
		bf.Add([]byte(item))
	}
	for _, item := range items {
		if !bf.Contains([]byte(item)) {
			t.Errorf("expected contains %s", item)
		}
	}
}

func TestBloomFilter_Count(t *testing.T) {
	bf := New(1024, 3)
	bf.Add([]byte("a"))
	bf.Add([]byte("b"))
	bf.Add([]byte("c"))
	if bf.Added() != 3 {
		t.Errorf("expected 3, got %d", bf.Added())
	}
}

func TestBloomFilter_DuplicateAdd(t *testing.T) {
	bf := New(1024, 3)
	bf.Add([]byte("x"))
	bf.Add([]byte("x"))
	if bf.Added() != 2 {
		t.Error("expected count 2 for two Add calls")
	}
}

func TestBloomFilter_FalsePositive(t *testing.T) {
	bf := New(1024, 3)
	for i := 0; i < 100; i++ {
		bf.Add([]byte(fmt.Sprintf("item-%d", i)))
	}
	falsePositives := 0
	for i := 0; i < 1000; i++ {
		if bf.Contains([]byte(fmt.Sprintf("not-added-%d", i))) {
			falsePositives++
		}
	}
	fpr := float64(falsePositives) / 1000.0
	if fpr > 0.5 {
		t.Errorf("FPR too high: %f", fpr)
	}
}

func TestBloomFilter_Reset(t *testing.T) {
	bf := New(1024, 3)
	bf.Add([]byte("data"))
	bf.Reset()
	if bf.Added() != 0 {
		t.Error("expected 0 after reset")
	}
	if bf.Contains([]byte("data")) {
		t.Error("expected not contains after reset")
	}
}

func TestBloomFilter_FillRatio(t *testing.T) {
	bf := New(1024, 3)
	bf.Add([]byte("a"))
	fr := bf.FillRatio()
	if fr <= 0 || fr > 1 {
		t.Errorf("expected fill ratio 0-1, got %f", fr)
	}
}

func TestBloomFilter_Size(t *testing.T) {
	bf := New(512, 3)
	if bf.Size() != 512 {
		t.Errorf("expected 512, got %d", bf.Size())
	}
}

func TestBloomFilter_Concurrent(t *testing.T) {
	bf := New(1024, 3)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bf.Add([]byte(fmt.Sprintf("item-%d", n)))
			bf.Contains([]byte(fmt.Sprintf("item-%d", n)))
		}(i)
	}
	wg.Wait()
	if bf.Added() != 100 {
		t.Errorf("expected 100, got %d", bf.Added())
	}
}

func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	bf := New(1024, 3)
	fpr := bf.FalsePositiveRate()
	if fpr < 0 || fpr > 1 {
		t.Errorf("expected FPR 0-1, got %f", fpr)
	}
}
