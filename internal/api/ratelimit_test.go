package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 5)
	defer rl.Stop()
	for i := 0; i < 5; i++ {
		if !rl.Allow("client1") {
			t.Errorf("expected allow on attempt %d", i)
		}
	}
	if rl.Allow("client1") {
		t.Error("expected deny after burst exhausted")
	}
}

func TestRateLimiter_DifferentClients(t *testing.T) {
	rl := NewRateLimiter(10, 2)
	defer rl.Stop()
	rl.Allow("a")
	rl.Allow("a")
	if rl.Allow("a") {
		t.Error("expected client a to be rate limited")
	}
	if !rl.Allow("b") {
		t.Error("expected client b to be allowed")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	now := time.Now()
	rl := NewRateLimiter(10, 2)
	defer rl.Stop()
	rl.Allow("c")
	rl.Allow("c")
	if rl.Allow("c") {
		t.Error("expected deny")
	}
	rl.mu.Lock()
	bucket := rl.clients["c"]
	bucket.lastTime = now.Add(-time.Second)
	bucket.tokens = 0
	rl.mu.Unlock()
	if !rl.Allow("c") {
		t.Error("expected allow after refill")
	}
}

func TestRateLimiter_Middleware(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	defer rl.Stop()
	called := false
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("expected handler to be called")
	}
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "127.0.0.1:1234"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w2.Code)
	}
}
