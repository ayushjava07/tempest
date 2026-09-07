package eventbus

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrBusClosed          = errors.New("eventbus: bus is closed")
	ErrSubscriptionClosed = errors.New("eventbus: subscription closed")
	ErrInvalidPattern     = errors.New("eventbus: invalid topic pattern")
)

// Message encapsulates a dispatched event payload.
type Message struct {
	ID         string            `json:"id"`
	Topic      string            `json:"topic"`
	Payload    []byte            `json:"payload"`
	Headers    map[string]string `json:"headers,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	RetryCount int               `json:"retry_count"`
}

// DeadLetter represents a failed message forwarded to the dead-letter queue.
type DeadLetter struct {
	Msg      Message
	Err      error
	FailedAt time.Time
	GroupID  string
}

// HandlerFunc processes a received message.
type HandlerFunc func(ctx context.Context, msg Message) error

type subscription struct {
	id         string
	group      string // empty if individual broadcast
	pattern    string
	patternSeg []string
	handler    HandlerFunc
	ch         chan Message
}

// Config configures the event bus behavior.
type Config struct {
	QueueCapacity int
	WorkerCount   int
	MaxRetries    int
	RetryBackoff  time.Duration
}

// DefaultConfig provides sane defaults.
func DefaultConfig() Config {
	return Config{
		QueueCapacity: 1024,
		WorkerCount:   4,
		MaxRetries:    2,
		RetryBackoff:  10 * time.Millisecond,
	}
}

// EventBus is an asynchronous pub-sub bus with wildcard matching and consumer groups.
type EventBus struct {
	mu              sync.RWMutex
	cfg             Config
	subs            map[string]*subscription
	groupMembers    map[string][]string // group -> subIDs
	groupRoundRobin map[string]*atomic.Uint64
	dlq             []DeadLetter
	dlqMu           sync.Mutex
	inCh            chan Message
	stopCh          chan struct{}
	wg              sync.WaitGroup
	closed          atomic.Bool
	seq             atomic.Uint64
}

// New creates a new EventBus.
func New(cfg Config) *EventBus {
	eb := &EventBus{
		cfg:             cfg,
		subs:            make(map[string]*subscription),
		groupMembers:    make(map[string][]string),
		groupRoundRobin: make(map[string]*atomic.Uint64),
		dlq:             make([]DeadLetter, 0),
		inCh:            make(chan Message, cfg.QueueCapacity),
		stopCh:          make(chan struct{}),
	}

	for i := 0; i < cfg.WorkerCount; i++ {
		eb.wg.Add(1)
		go eb.workerLoop()
	}

	return eb
}

// MatchTopic evaluates whether `topic` matches `pattern`.
// Supports `*` (single segment) and `#` (zero or more segments).
func MatchTopic(pattern, topic string) bool {
	if pattern == "#" || pattern == topic {
		return true
	}

	pSeg := strings.Split(pattern, ".")
	tSeg := strings.Split(topic, ".")

	return matchSegments(pSeg, tSeg)
}

func matchSegments(pSeg, tSeg []string) bool {
	pIdx := 0
	tIdx := 0

	for pIdx < len(pSeg) && tIdx < len(tSeg) {
		p := pSeg[pIdx]
		if p == "#" {
			// # at end matches everything remaining
			if pIdx == len(pSeg)-1 {
				return true
			}
			// Lookahead for next pattern segment
			for t := tIdx; t <= len(tSeg); t++ {
				if matchSegments(pSeg[pIdx+1:], tSeg[t:]) {
					return true
				}
			}
			return false
		} else if p == "*" || p == tSeg[tIdx] {
			pIdx++
			tIdx++
		} else {
			return false
		}
	}

	for pIdx < len(pSeg) && pSeg[pIdx] == "#" {
		pIdx++
	}

	return pIdx == len(pSeg) && tIdx == len(tSeg)
}

func generateID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Subscribe registers an individual broadcast subscriber.
func (eb *EventBus) Subscribe(pattern string, handler HandlerFunc) (string, error) {
	return eb.subscribeInternal("", pattern, handler)
}

// SubscribeGroup registers a subscriber within a load-balanced consumer group.
func (eb *EventBus) SubscribeGroup(group, pattern string, handler HandlerFunc) (string, error) {
	if group == "" {
		return "", errors.New("eventbus: group name cannot be empty")
	}
	return eb.subscribeInternal(group, pattern, handler)
}

func (eb *EventBus) subscribeInternal(group, pattern string, handler HandlerFunc) (string, error) {
	if pattern == "" {
		return "", ErrInvalidPattern
	}

	subID := generateID()
	sub := &subscription{
		id:         subID,
		group:      group,
		pattern:    pattern,
		patternSeg: strings.Split(pattern, "."),
		handler:    handler,
		ch:         make(chan Message, eb.cfg.QueueCapacity),
	}

	eb.mu.Lock()
	if eb.closed.Load() {
		eb.mu.Unlock()
		return "", ErrBusClosed
	}

	eb.subs[subID] = sub
	if group != "" {
		eb.groupMembers[group] = append(eb.groupMembers[group], subID)
		if eb.groupRoundRobin[group] == nil {
			eb.groupRoundRobin[group] = &atomic.Uint64{}
		}
	}
	eb.mu.Unlock()

	eb.wg.Add(1)
	go eb.subscriptionLoop(sub)

	return subID, nil
}

// Unsubscribe removes a subscription by ID.
func (eb *EventBus) Unsubscribe(subID string) error {
	eb.mu.Lock()
	sub, ok := eb.subs[subID]
	if !ok {
		eb.mu.Unlock()
		return ErrSubscriptionClosed
	}

	delete(eb.subs, subID)
	if sub.group != "" {
		members := eb.groupMembers[sub.group]
		filtered := make([]string, 0, len(members))
		for _, m := range members {
			if m != subID {
				filtered = append(filtered, m)
			}
		}
		eb.groupMembers[sub.group] = filtered
	}
	eb.mu.Unlock()

	close(sub.ch)
	return nil
}

// Publish enqueues a message for distribution.
func (eb *EventBus) Publish(ctx context.Context, topic string, payload []byte, headers map[string]string) error {
	if eb.closed.Load() {
		return ErrBusClosed
	}

	msg := Message{
		ID:        fmt.Sprintf("msg-%d-%s", eb.seq.Add(1), generateID()[:6]),
		Topic:     topic,
		Payload:   append([]byte(nil), payload...),
		Headers:   headers,
		Timestamp: time.Now().UTC(),
	}

	select {
	case eb.inCh <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-eb.stopCh:
		return ErrBusClosed
	}
}

func (eb *EventBus) workerLoop() {
	defer eb.wg.Done()

	for {
		select {
		case <-eb.stopCh:
			return
		case msg, ok := <-eb.inCh:
			if !ok {
				return
			}
			eb.dispatch(msg)
		}
	}
}

func (eb *EventBus) dispatch(msg Message) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// 1. Deliver to individual broadcast subscribers
	for _, sub := range eb.subs {
		if sub.group == "" && MatchTopic(sub.pattern, msg.Topic) {
			select {
			case sub.ch <- msg:
			default:
				// Queue full, drop or handle backpressure
			}
		}
	}

	// 2. Deliver to consumer groups (one member per matching group)
	for group, members := range eb.groupMembers {
		if len(members) == 0 {
			continue
		}

		// Find members matching topic
		var matchingSubs []*subscription
		for _, mID := range members {
			if s, ok := eb.subs[mID]; ok && MatchTopic(s.pattern, msg.Topic) {
				matchingSubs = append(matchingSubs, s)
			}
		}

		if len(matchingSubs) == 0 {
			continue
		}

		// Pick one subscriber round-robin
		rr := eb.groupRoundRobin[group]
		idx := int(rr.Add(1) % uint64(len(matchingSubs)))
		target := matchingSubs[idx]

		select {
		case target.ch <- msg:
		default:
		}
	}
}

func (eb *EventBus) subscriptionLoop(sub *subscription) {
	defer eb.wg.Done()

	for {
		select {
		case <-eb.stopCh:
			return
		case msg, ok := <-sub.ch:
			if !ok {
				return
			}
			eb.executeHandler(sub, msg)
		}
	}
}

func (eb *EventBus) executeHandler(sub *subscription, msg Message) {
	ctx := context.Background()
	var lastErr error

	for attempt := 0; attempt <= eb.cfg.MaxRetries; attempt++ {
		msg.RetryCount = attempt
		err := sub.handler(ctx, msg)
		if err == nil {
			return
		}
		lastErr = err
		if attempt < eb.cfg.MaxRetries && eb.cfg.RetryBackoff > 0 {
			time.Sleep(eb.cfg.RetryBackoff)
		}
	}

	// Forward to Dead Letter Queue
	eb.dlqMu.Lock()
	eb.dlq = append(eb.dlq, DeadLetter{
		Msg:      msg,
		Err:      lastErr,
		FailedAt: time.Now().UTC(),
		GroupID:  sub.group,
	})
	eb.dlqMu.Unlock()
}

// DeadLetters returns a snapshot copy of the Dead Letter Queue.
func (eb *EventBus) DeadLetters() []DeadLetter {
	eb.dlqMu.Lock()
	defer eb.dlqMu.Unlock()

	cp := make([]DeadLetter, len(eb.dlq))
	copy(cp, eb.dlq)
	return cp
}

// Stop drains queues and cleanly terminates all workers.
func (eb *EventBus) Stop() {
	if !eb.closed.CompareAndSwap(false, true) {
		return
	}
	close(eb.stopCh)

	eb.mu.Lock()
	for _, sub := range eb.subs {
		close(sub.ch)
	}
	eb.subs = make(map[string]*subscription)
	eb.groupMembers = make(map[string][]string)
	eb.mu.Unlock()

	eb.wg.Wait()
}
