package ratepool

import (
	"sync"
	"time"
)

type Pool struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	interval   time.Duration
	maxTokens  int
	cleanupInt time.Duration
	stopCh     chan struct{}
}

type bucket struct {
	tokens   int
	lastTime time.Time
}

func New(interval time.Duration, maxTokens int) *Pool {
	p := &Pool{
		buckets:    make(map[string]*bucket),
		interval:   interval,
		maxTokens:  maxTokens,
		cleanupInt: interval * 10,
		stopCh:     make(chan struct{}),
	}
	go p.cleanup()
	return p
}

func (p *Pool) Allow(key string) bool {
	return p.AllowN(key, 1)
}

func (p *Pool) AllowN(key string, n int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.buckets[key]
	if !ok {
		b = &bucket{tokens: p.maxTokens, lastTime: time.Now()}
		p.buckets[key] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.lastTime)
	refill := int(elapsed / p.interval)
	if refill > 0 {
		b.tokens += refill
		if b.tokens > p.maxTokens {
			b.tokens = p.maxTokens
		}
		b.lastTime = now
	}
	if b.tokens >= n {
		b.tokens -= n
		return true
	}
	return false
}

func (p *Pool) Tokens(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.buckets[key]
	if !ok {
		return p.maxTokens
	}
	return b.tokens
}

func (p *Pool) Reset(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.buckets, key)
}

func (p *Pool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buckets)
}

func (p *Pool) Stop() {
	close(p.stopCh)
}

func (p *Pool) cleanup() {
	ticker := time.NewTicker(p.cleanupInt)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			now := time.Now()
			for k, b := range p.buckets {
				if now.Sub(b.lastTime) > p.cleanupInt {
					delete(p.buckets, k)
				}
			}
			p.mu.Unlock()
		case <-p.stopCh:
			return
		}
	}
}
