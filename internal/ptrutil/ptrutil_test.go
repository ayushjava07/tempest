package ptrutil

import (
	"testing"
)

func TestOf(t *testing.T) {
	p := Of(42)
	if *p != 42 {
		t.Errorf("expected 42, got %d", *p)
	}
}

func TestOf_String(t *testing.T) {
	p := Of("hello")
	if *p != "hello" {
		t.Errorf("expected hello, got %s", *p)
	}
}

func TestDeref(t *testing.T) {
	x := 42
	if Deref(&x, 0) != 42 {
		t.Error("expected 42")
	}
	if Deref[int](nil, 99) != 99 {
		t.Error("expected default 99")
	}
}

func TestEqual(t *testing.T) {
	a := Of(1)
	b := Of(1)
	if !Equal(a, b) {
		t.Error("expected equal")
	}
	c := Of(2)
	if Equal(a, c) {
		t.Error("expected not equal")
	}
	if Equal(a, nil) {
		t.Error("expected not equal with nil")
	}
	if Equal[int](nil, nil) {
	}
}

func TestCopy(t *testing.T) {
	x := 42
	p := Copy(&x)
	if *p != 42 {
		t.Error("expected copy")
	}
	x = 99
	if *p != 42 {
		t.Error("expected independent copy")
	}
}

func TestCopy_Nil(t *testing.T) {
	p := Copy[int](nil)
	if p != nil {
		t.Error("expected nil")
	}
}

func TestIsNil(t *testing.T) {
	if !IsNil(nil) {
		t.Error("expected nil")
	}
	x := 42
	if IsNil(&x) {
		t.Error("expected not nil")
	}
}
