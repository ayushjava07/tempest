package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrSignalTimeout  = errors.New("signal: wait timed out")
	ErrSignalConsumed = errors.New("signal: signal already consumed")
	ErrSignalNotFound = errors.New("signal: signal not found")
	ErrInvalidSignal  = errors.New("signal: signal name and run ID cannot be empty")
)

// Signal represents an asynchronous notification or external approval payload.
type Signal struct {
	ID         string
	Namespace  string
	RunID      string
	Name       string
	Payload    []byte
	SentAt     time.Time
	ConsumedAt *time.Time
	ConsumedBy string
}

// Predicate defines a filter function on signal payloads.
type Predicate func(s *Signal) bool

// Manager coordinates signal dispatch and waiting between external actors and workflows.
type Manager struct {
	mu      sync.Mutex
	signals map[string][]*Signal      // key: ns:runID:name
	waiters map[string][]chan *Signal // key: ns:runID:name
}

func NewManager() *Manager {
	return &Manager{
		signals: make(map[string][]*Signal),
		waiters: make(map[string][]chan *Signal),
	}
}

func (m *Manager) channelKey(namespace, runID, name string) string {
	return fmt.Sprintf("%s:%s:%s", namespace, runID, name)
}

// Send broadcasts a signal to any waiting goroutine, or buffers it if no waiter is present.
func (m *Manager) Send(ctx context.Context, sig *Signal) error {
	if sig == nil || sig.RunID == "" || sig.Name == "" {
		return ErrInvalidSignal
	}
	if sig.SentAt.IsZero() {
		sig.SentAt = time.Now().UTC()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	k := m.channelKey(sig.Namespace, sig.RunID, sig.Name)

	// If active waiters exist, deliver to the first waiter
	waiterList := m.waiters[k]
	if len(waiterList) > 0 {
		ch := waiterList[0]
		m.waiters[k] = waiterList[1:]
		if len(m.waiters[k]) == 0 {
			delete(m.waiters, k)
		}
		ch <- sig
		return nil
	}

	// Buffer signal for subsequent waiter
	m.signals[k] = append(m.signals[k], sig)
	return nil
}

// Receive blocks until a signal is delivered, or until the context or timeout expires.
func (m *Manager) Receive(ctx context.Context, namespace, runID, name string, timeout time.Duration, predicate Predicate) (*Signal, error) {
	if runID == "" || name == "" {
		return nil, ErrInvalidSignal
	}

	k := m.channelKey(namespace, runID, name)

	m.mu.Lock()
	// Check buffered signals first
	buffered := m.signals[k]
	for i, sig := range buffered {
		if sig.ConsumedAt == nil && (predicate == nil || predicate(sig)) {
			// Found matching buffered signal!
			m.signals[k] = append(buffered[:i], buffered[i+1:]...)
			if len(m.signals[k]) == 0 {
				delete(m.signals, k)
			}
			now := time.Now().UTC()
			sig.ConsumedAt = &now
			m.mu.Unlock()
			return sig, nil
		}
	}

	// No buffered match; register waiter channel
	waitCh := make(chan *Signal, 1)
	m.waiters[k] = append(m.waiters[k], waitCh)
	m.mu.Unlock()

	cleanup := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		list := m.waiters[k]
		for i, ch := range list {
			if ch == waitCh {
				m.waiters[k] = append(list[:i], list[i+1:]...)
				if len(m.waiters[k]) == 0 {
					delete(m.waiters, k)
				}
				break
			}
		}
	}

	var timeoutCh <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutCh = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			cleanup()
			return nil, ctx.Err()
		case <-timeoutCh:
			cleanup()
			return nil, ErrSignalTimeout
		case sig := <-waitCh:
			if predicate == nil || predicate(sig) {
				now := time.Now().UTC()
				sig.ConsumedAt = &now
				return sig, nil
			}
			// Predicate rejected, re-buffer signal for someone else and re-register waiter
			m.mu.Lock()
			m.signals[k] = append(m.signals[k], sig)
			m.waiters[k] = append(m.waiters[k], waitCh)
			m.mu.Unlock()
		}
	}
}

// PendingCount returns the number of buffered unconsumed signals for the specified channel.
func (m *Manager) PendingCount(namespace, runID, name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.signals[m.channelKey(namespace, runID, name)])
}
