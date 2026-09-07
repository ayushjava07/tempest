package throttler

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrRateLimitExceeded  = errors.New("throttler: rate limit quota exceeded")
	ErrInvalidQuotaConfig = errors.New("throttler: invalid rate limit quota configuration")
	ErrQuotaNotFound      = errors.New("throttler: quota key not found")
)

// ThrottleConfig defines rate limiting parameters for a quota key.
type ThrottleConfig struct {
	QuotaKey   string        `json:"quota_key"`
	RateLimit  int           `json:"rate_limit"`  // Max operations per window
	Window     time.Duration `json:"window"`      // Rolling window duration
	BurstLimit int           `json:"burst_limit"` // Max allowed burst
}

// Validate ensures throttle configuration attributes are legal.
func (c ThrottleConfig) Validate() error {
	if c.QuotaKey == "" {
		return fmt.Errorf("%w: quota key must not be empty", ErrInvalidQuotaConfig)
	}
	if c.RateLimit <= 0 {
		return fmt.Errorf("%w: rate limit must be positive", ErrInvalidQuotaConfig)
	}
	if c.Window <= 0 {
		return fmt.Errorf("%w: window duration must be positive", ErrInvalidQuotaConfig)
	}
	if c.BurstLimit < c.RateLimit {
		c.BurstLimit = c.RateLimit
	}
	return nil
}

// Decision represents the result of a rate limit admission evaluation.
type Decision struct {
	Allowed      bool          `json:"allowed"`
	Remaining    int           `json:"remaining"`
	ResetAfter   time.Duration `json:"reset_after"`
	WaitDuration time.Duration `json:"wait_duration"`
}

// Throttler defines the rate limiter contract.
type Throttler interface {
	Allow(key string) (Decision, error)
	AllowN(key string, n int) (Decision, error)
	SetQuota(cfg ThrottleConfig) error
	GetQuota(key string) (ThrottleConfig, bool)
}
