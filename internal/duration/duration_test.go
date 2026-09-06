package duration

import (
	"testing"
	"time"
)

func TestFormatShort(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "500us"},
		{1500 * time.Microsecond, "1ms"},
		{5 * time.Second, "5.0s"},
		{90 * time.Second, "1.5m"},
		{3 * time.Hour, "3.0h"},
	}
	for _, tt := range tests {
		got := FormatShort(tt.d)
		if got != tt.want {
			t.Errorf("FormatShort(%v) = %s, want %s", tt.d, got, tt.want)
		}
	}
}

func TestFormatHuman(t *testing.T) {
	if got := FormatHuman(5 * time.Second); got != "5 seconds" {
		t.Errorf("got %s", got)
	}
	if got := FormatHuman(3 * time.Hour); got != "3 hours" {
		t.Errorf("got %s", got)
	}
}

func TestParseHuman(t *testing.T) {
	d, err := ParseHuman("5 seconds")
	if err != nil {
		t.Fatal(err)
	}
	if d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}
	d, err = ParseHuman("2 hours")
	if err != nil {
		t.Fatal(err)
	}
	if d != 2*time.Hour {
		t.Errorf("expected 2h, got %v", d)
	}
}

func TestParseHuman_Invalid(t *testing.T) {
	_, err := ParseHuman("xyz")
	if err == nil {
		t.Error("expected error")
	}
}

func TestParseDuration_Empty(t *testing.T) {
	_, err := ParseDuration("")
	if err == nil {
		t.Error("expected error")
	}
}

func TestClamp(t *testing.T) {
	if got := Clamp(5*time.Second, time.Second, 10*time.Second); got != 5*time.Second {
		t.Errorf("got %v", got)
	}
	if got := Clamp(0, time.Second, 10*time.Second); got != time.Second {
		t.Errorf("got %v", got)
	}
	if got := Clamp(20*time.Second, time.Second, 10*time.Second); got != 10*time.Second {
		t.Errorf("got %v", got)
	}
}

func TestCeil(t *testing.T) {
	if got := Ceil(7*time.Second, 5*time.Second); got != 10*time.Second {
		t.Errorf("got %v", got)
	}
	if got := Ceil(10*time.Second, 5*time.Second); got != 10*time.Second {
		t.Errorf("got %v", got)
	}
}

func TestFloor(t *testing.T) {
	if got := Floor(7*time.Second, 5*time.Second); got != 5*time.Second {
		t.Errorf("got %v", got)
	}
	if got := Floor(3*time.Second, 5*time.Second); got != 0 {
		t.Errorf("got %v", got)
	}
}
