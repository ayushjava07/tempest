package signal

import (
	"os"
	"testing"
	"time"
)

func TestCatch(t *testing.T) {
	ctx := Catch(os.Interrupt)
	select {
	case <-ctx.Done():
		t.Error("expected not done")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestNotify(t *testing.T) {
	ch := Notify(os.Interrupt)
	select {
	case <-ch:
		t.Error("expected not received")
	case <-time.After(10 * time.Millisecond):
	}
}

func TestIgnore(t *testing.T) {
	Ignore(os.Interrupt)
}

func TestReset(t *testing.T) {
	Reset(os.Interrupt)
}

func TestWaitForShutdown(t *testing.T) {
	ch := make(chan struct{})
	go func() {
		time.Sleep(10 * time.Millisecond)
		close(ch)
	}()
	select {
	case <-ch:
	case <-time.After(50 * time.Millisecond):
	}
}