package cron

import (
	"testing"
	"time"
)

func TestCron_EveryFiveMinutes(t *testing.T) {
	s, err := Parse("*/5 * * * *")
	if err != nil {
		t.Fatalf("failed to parse cron: %v", err)
	}

	base := time.Date(2026, 1, 1, 10, 2, 0, 0, time.UTC)
	next := s.Next(base)

	expected := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestCron_DailyAtMidnight(t *testing.T) {
	s, err := Parse("0 0 * * *")
	if err != nil {
		t.Fatalf("failed to parse cron: %v", err)
	}

	base := time.Date(2026, 5, 10, 23, 30, 0, 0, time.UTC)
	next := s.Next(base)

	expected := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestCron_InvalidFieldCounts(t *testing.T) {
	_, err := Parse("* * *")
	if err == nil {
		t.Errorf("expected error on 3-field cron, got nil")
	}

	_, err = Parse("99 * * * *")
	if err == nil {
		t.Errorf("expected error on out-of-bounds minute, got nil")
	}

	_, err = Parse("0 25 * * *")
	if err == nil {
		t.Errorf("expected error on out-of-bounds hour, got nil")
	}
}
