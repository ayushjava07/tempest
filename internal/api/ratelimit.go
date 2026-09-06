package api

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*tokenBucket
	rate     int
	burst    int
	stop     chan struct{}
}

type tokenBucket struct {
	tokens   int
	lastTime time.Time
}

func NewRateLimiter(rate, burst int) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*tokenBucket),
		rate:    rate,
		burst:   burst,
		stop:    make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	bucket, exists := rl.clients[clientID]
	if !exists {
		bucket = &tokenBucket{tokens: rl.burst, lastTime: now}
		rl.clients[clientID] = bucket
	}
	elapsed := now.Sub(bucket.lastTime).Seconds()
	refill := int(elapsed * float64(rl.rate))
	if refill > 0 {
		bucket.tokens += refill
		if bucket.tokens > rl.burst {
			bucket.tokens = rl.burst
		}
		bucket.lastTime = now
	}
	if bucket.tokens <= 0 {
		return false
	}
	bucket.tokens--
	return true
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for id, bucket := range rl.clients {
				if now.Sub(bucket.lastTime) > 5*time.Minute {
					delete(rl.clients, id)
				}
			}
			rl.mu.Unlock()
		}
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.RemoteAddr
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			clientID = fwd
		}
		if !rl.Allow(clientID) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
