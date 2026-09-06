package timeout

import (
	"context"
	"time"
)

func After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func Run(ctx context.Context, d time.Duration, fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return fn(ctx)
}

func RunWithResult[T any](ctx context.Context, d time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return fn(ctx)
}

func OrTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

func OrDeadline(parent context.Context, t time.Time) (context.Context, context.CancelFunc) {
	return context.WithDeadline(parent, t)
}

type Timer struct {
	timer *time.Timer
	done  chan struct{}
}

func NewTimer(d time.Duration) *Timer {
	t := &Timer{
		timer: time.NewTimer(d),
		done:  make(chan struct{}),
	}
	go func() {
		select {
		case <-t.timer.C:
			close(t.done)
		case <-t.done:
		}
	}()
	return t
}

func (t *Timer) Stop() bool {
	stopped := t.timer.Stop()
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	return stopped
}

func (t *Timer) Reset(d time.Duration) {
	t.timer.Reset(d)
}

func AfterFunc(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}
