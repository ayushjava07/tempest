package backoff

import (
	"testing"
	"time"
)

func TestExponential(t *testing.T) {
	initial := 100 * time.Millisecond
	max := 1 * time.Second
	multiplier := 2.0

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, initial},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, max},
		{6, max},
	}
	for _, tc := range cases {
		got := Exponential(tc.attempt, initial, max, multiplier)
		if got != tc.want {
			t.Errorf("Exponential(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestExponential_ZeroMultiplier(t *testing.T) {
	got := Exponential(2, 100*time.Millisecond, 0, 0)
	if got != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", got)
	}
}
