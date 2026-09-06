package ringbuffer

import (
	"sync"
	"testing"
)

func TestRingBuffer_PushPop(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	v, ok := rb.Pop()
	if !ok || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
	v, ok = rb.Pop()
	if !ok || v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := New[int](2)
	if !rb.Push(1) {
		t.Error("expected success")
	}
	if !rb.Push(2) {
		t.Error("expected success")
	}
	if rb.Push(3) {
		t.Error("expected overflow")
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := New[int](3)
	_, ok := rb.Pop()
	if ok {
		t.Error("expected empty")
	}
}

func TestRingBuffer_Len(t *testing.T) {
	rb := New[int](5)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	if rb.Len() != 3 {
		t.Errorf("expected 3, got %d", rb.Len())
	}
}

func TestRingBuffer_Peek(t *testing.T) {
	rb := New[int](3)
	rb.Push(42)
	v, ok := rb.Peek()
	if !ok || v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if rb.Len() != 1 {
		t.Error("peek should not remove")
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Clear()
	if rb.Len() != 0 {
		t.Error("expected empty after clear")
	}
}

func TestRingBuffer_Items(t *testing.T) {
	rb := New[int](3)
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	items := rb.Items()
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if items[0] != 1 || items[1] != 2 || items[2] != 3 {
		t.Error("wrong order")
	}
}

func TestRingBuffer_WrapAround(t *testing.T) {
	rb := New[int](2)
	rb.Push(1)
	rb.Pop()
	rb.Push(2)
	rb.Push(3)
	items := rb.Items()
	if len(items) != 2 || items[0] != 2 || items[1] != 3 {
		t.Error("expected [2,3] after wrap")
	}
}

func TestRingBuffer_Concurrent(t *testing.T) {
	rb := New[int](100)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rb.Push(n)
			rb.Pop()
		}(i)
	}
	wg.Wait()
}

func TestRingBuffer_FullEmpty(t *testing.T) {
	rb := New[int](2)
	if !rb.Empty() {
		t.Error("expected empty")
	}
	rb.Push(1)
	rb.Push(2)
	if !rb.Full() {
		t.Error("expected full")
	}
}
