package errors

import (
	stderrors "errors"
	"fmt"
)

type Class string

const (
	ClassNotFound         Class = "not_found"
	ClassAlreadyExists    Class = "already_exists"
	ClassInvalidArgument  Class = "invalid_argument"
	ClassConflict         Class = "conflict"
	ClassUnauthenticated  Class = "unauthenticated"
	ClassPermissionDenied Class = "permission_denied"
	ClassInternal         Class = "internal"
	ClassUnavailable      Class = "unavailable"
	ClassDeadlineExceeded Class = "deadline_exceeded"
	ClassCanceled         Class = "canceled"
)

var (
	ErrNotFound        = stderrors.New("not found")
	ErrAlreadyExists   = stderrors.New("already exists")
	ErrInvalidArgument = stderrors.New("invalid argument")
	ErrConflict        = stderrors.New("conflict")
	ErrUnauthenticated = stderrors.New("unauthenticated")
	ErrPermissionDenied = stderrors.New("permission denied")
	ErrInternal        = stderrors.New("internal error")
	ErrUnavailable     = stderrors.New("unavailable")
	ErrDeadlineExceeded = stderrors.New("deadline exceeded")
	ErrCanceled        = stderrors.New("canceled")
)

type Classified struct {
	class Class
	err   error
}

func (c *Classified) Error() string { return c.err.Error() }
func (c *Classified) Unwrap() error { return c.err }
func (c *Classified) Class() Class  { return c.class }

func Of(class Class, err error) *Classified {
	return &Classified{class: class, err: err}
}

func Classify(err error) Class {
	var c *Classified
	if stderrors.As(err, &c) {
		return c.class
	}
	switch {
	case stderrors.Is(err, ErrNotFound):
		return ClassNotFound
	case stderrors.Is(err, ErrAlreadyExists):
		return ClassAlreadyExists
	case stderrors.Is(err, ErrInvalidArgument):
		return ClassInvalidArgument
	case stderrors.Is(err, ErrConflict):
		return ClassConflict
	case stderrors.Is(err, ErrUnauthenticated):
		return ClassUnauthenticated
	case stderrors.Is(err, ErrPermissionDenied):
		return ClassPermissionDenied
	case stderrors.Is(err, ErrInternal):
		return ClassInternal
	case stderrors.Is(err, ErrUnavailable):
		return ClassUnavailable
	case stderrors.Is(err, ErrDeadlineExceeded):
		return ClassDeadlineExceeded
	case stderrors.Is(err, ErrCanceled):
		return ClassCanceled
	default:
		return ClassInternal
	}
}

func NotFoundError(err error) *Classified        { return Of(ClassNotFound, err) }
func AlreadyExistsError(err error) *Classified   { return Of(ClassAlreadyExists, err) }
func InvalidArgumentError(err error) *Classified { return Of(ClassInvalidArgument, err) }
func ConflictError(err error) *Classified        { return Of(ClassConflict, err) }
func UnauthenticatedError(err error) *Classified { return Of(ClassUnauthenticated, err) }
func PermissionDeniedError(err error) *Classified { return Of(ClassPermissionDenied, err) }
func InternalError(err error) *Classified        { return Of(ClassInternal, err) }
func UnavailableError(err error) *Classified     { return Of(ClassUnavailable, err) }
func DeadlineExceededError(err error) *Classified { return Of(ClassDeadlineExceeded, err) }
func CanceledError(err error) *Classified        { return Of(ClassCanceled, err) }

func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

func As(err error, target any) bool {
	return stderrors.As(err, target)
}

func Errorf(class Class, format string, args ...any) *Classified {
	return Of(class, fmt.Errorf(format, args...))
}
