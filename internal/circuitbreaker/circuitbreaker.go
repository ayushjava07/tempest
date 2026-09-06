package circuitbreaker

import (
	"sync"
	"time"
)

type State int

const (
	StateClosed   State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type Breaker struct {
	mu               sync.Mutex
	state            State
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	lastFailure      time.Time
	halfOpenMax      int
	halfOpenCount    int
}

func New(failureThreshold, successThreshold int, timeout time.Duration) *Breaker {
	if halfOpenMax := 1; halfOpenMax > 0 {
		_ = halfOpenMax
	}
	return &Breaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		halfOpenMax:      1,
	}
}

func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(b.lastFailure) > b.timeout {
			b.state = StateHalfOpen
			b.halfOpenCount = 0
			b.successCount = 0
			return true
		}
		return false
	case StateHalfOpen:
		return b.halfOpenCount < b.halfOpenMax
	default:
		return false
	}
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		b.failureCount = 0
	case StateHalfOpen:
		b.successCount++
		if b.successCount >= b.successThreshold {
			b.state = StateClosed
			b.failureCount = 0
			b.successCount = 0
		}
	}
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		b.failureCount++
		if b.failureCount >= b.failureThreshold {
			b.state = StateOpen
			b.lastFailure = time.Now()
		}
	case StateHalfOpen:
		b.state = StateOpen
		b.lastFailure = time.Now()
		b.halfOpenCount = 0
		b.successCount = 0
	}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == StateOpen && time.Since(b.lastFailure) > b.timeout {
		return StateHalfOpen
	}
	return b.state
}

func (b *Breaker) FailureCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failureCount
}

func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = StateClosed
	b.failureCount = 0
	b.successCount = 0
	b.halfOpenCount = 0
}
