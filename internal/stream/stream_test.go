package stream

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipeline_AddProcess(t *testing.T) {
	p := NewPipeline[int]()
	var steps []string
	p.Add(func(ctx context.Context, v int) error {
		steps = append(steps, "a")
		return nil
	})
	p.Add(func(ctx context.Context, v int) error {
		steps = append(steps, "b")
		return nil
	})
	if err := p.Process(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0] != "a" || steps[1] != "b" {
		t.Error("unexpected steps")
	}
}

func TestPipeline_Error(t *testing.T) {
	p := NewPipeline[int]()
	p.Add(func(ctx context.Context, v int) error {
		return fmt.Errorf("fail")
	})
	p.Add(func(ctx context.Context, v int) error {
		return nil
	})
	err := p.Process(context.Background(), 1)
	if err == nil {
		t.Error("expected error")
	}
}

func TestPipeline_Batch(t *testing.T) {
	p := NewPipeline[int]()
	var count atomic.Int32
	p.Add(func(ctx context.Context, v int) error {
		count.Add(1)
		return nil
	})
	if err := p.ProcessBatch(context.Background(), []int{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if count.Load() != 3 {
		t.Errorf("expected 3, got %d", count.Load())
	}
}

func TestBuffer_PushPop(t *testing.T) {
	b := NewBuffer[int](5)
	if err := b.Push(1); err != nil {
		t.Fatal(err)
	}
	if err := b.Push(2); err != nil {
		t.Fatal(err)
	}
	v, err := b.Pop()
	if err != nil || v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
	v, err = b.Pop()
	if err != nil || v != 2 {
		t.Errorf("expected 2, got %d", v)
	}
}

func TestBuffer_Capacity(t *testing.T) {
	b := NewBuffer[int](2)
	b.Push(1)
	b.Push(2)
	done := make(chan error, 1)
	go func() {
		done <- b.Push(3)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("expected full error")
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBuffer_Close(t *testing.T) {
	b := NewBuffer[int](5)
	b.Push(1)
	b.Close()
	v, err := b.Pop()
	if err != nil {
		t.Fatalf("expected item before close error, got err: %v", err)
	}
	if v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
	_, err = b.Pop()
	if err == nil {
		t.Error("expected error after close and empty")
	}
	err = b.Push(2)
	if err == nil {
		t.Error("expected push error after close")
	}
}

func TestFanOut_Send(t *testing.T) {
	f := NewFanOut[int](3, 5)
	if err := f.Send(42); err != nil {
		t.Fatal(err)
	}
	ch1, _ := f.Channel(0)
	ch2, _ := f.Channel(1)
	v1 := <-ch1
	v2 := <-ch2
	if v1 != 42 || v2 != 42 {
		t.Errorf("expected 42, got %d, %d", v1, v2)
	}
}

func TestFanOut_Close(t *testing.T) {
	f := NewFanOut[int](2, 5)
	f.Send(1)
	f.Close()
	ch, _ := f.Channel(0)
	v, ok := <-ch
	if !ok && v != 1 {
		t.Error("expected to receive 1 then closed")
	}
	v, ok = <-ch
	if ok {
		t.Error("expected closed channel")
	}
}