package backoff

import (
	"math"
	"time"
)

func Exponential(attempt int, initial, max time.Duration, multiplier float64) time.Duration {
	if attempt <= 0 {
		return initial
	}
	if multiplier <= 0 {
		multiplier = 2.0
	}
	d := float64(initial) * math.Pow(multiplier, float64(attempt-1))
	if max > 0 && time.Duration(d) > max {
		return max
	}
	return time.Duration(d)
}
