package errors

import (
	"fmt"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		want Class
	}{
		{NotFoundError(ErrNotFound), ClassNotFound},
		{AlreadyExistsError(ErrAlreadyExists), ClassAlreadyExists},
		{ConflictError(ErrConflict), ClassConflict},
		{UnauthenticatedError(ErrUnauthenticated), ClassUnauthenticated},
		{PermissionDeniedError(ErrPermissionDenied), ClassPermissionDenied},
		{InternalError(ErrInternal), ClassInternal},
		{UnavailableError(ErrUnavailable), ClassUnavailable},
		{DeadlineExceededError(ErrDeadlineExceeded), ClassDeadlineExceeded},
		{InvalidArgumentError(ErrInvalidArgument), ClassInvalidArgument},
		{CanceledError(ErrCanceled), ClassCanceled},
		{fmt.Errorf("unknown"), ClassInternal},
	}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("Classify(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestIs(t *testing.T) {
	err := NotFoundError(ErrNotFound)
	if !Is(err, ErrNotFound) {
		t.Error("expected Is(err, ErrNotFound) to be true")
	}
}

func TestOf(t *testing.T) {
	err := Of(ClassInternal, fmt.Errorf("boom"))
	if err.Class() != ClassInternal {
		t.Errorf("expected ClassInternal, got %v", err.Class())
	}
	if err.Error() != "boom" {
		t.Errorf("expected 'boom', got %q", err.Error())
	}
}

func TestErrorf(t *testing.T) {
	err := Errorf(ClassInvalidArgument, "bad %s", "input")
	if err.Class() != ClassInvalidArgument {
		t.Errorf("expected ClassInvalidArgument, got %v", err.Class())
	}
	if err.Error() != "bad input" {
		t.Errorf("expected 'bad input', got %q", err.Error())
	}
}

func TestUnwrap(t *testing.T) {
	inner := fmt.Errorf("inner")
	outer := Of(ClassInternal, inner)
	if !Is(outer, inner) {
		t.Error("expected Unwrap to work")
	}
}
