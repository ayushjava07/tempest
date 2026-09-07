package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEverySchedule_Next(t *testing.T) {
	s := Every(5 * time.Second)
	now := time.Now()
	next := s.Next(now)
	if next.Sub(now) != 5*time.Second {
		t.Errorf("expected 5s, got %v", next.Sub(now))
	}
}

func TestAtSchedule_Next(t *testing.T) {
	s := At("12:00", "18:00")
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	next := s.Next(now)
	if next.Hour() != 12 {
		t.Errorf("expected 12:00, got %v", next)
	}
	now = time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)
	next = s.Next(now)
	if next.Hour() != 18 {
		t.Errorf("expected 18:00, got %v", next)
	}
	now = time.Date(2024, 1, 1, 19, 0, 0, 0, time.UTC)
	next = s.Next(now)
	if next.Hour() != 12 || next.Day() != 2 {
		t.Errorf("expected next day 12:00, got %v", next)
	}
}

func TestScheduler_AddJob(t *testing.T) {
	s := NewScheduler()
	s.AddJob(&Job{Name: "test", Schedule: Every(time.Second), Func: func(ctx context.Context) error { return nil }})
	if s.JobCount() != 1 {
		t.Errorf("expected 1, got %d", s.JobCount())
	}
}

func TestScheduler_StartStop(t *testing.T) {
	s := NewScheduler()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	s.Start(ctx)
	cancel()
	time.Sleep(20 * time.Millisecond)
	s.Stop()
}

func TestScheduler_RunsJob(t *testing.T) {
	var called atomic.Bool
	s := NewScheduler()
	s.AddJob(&Job{
		Name:     "test",
		Schedule: Every(10 * time.Millisecond),
		Func: func(ctx context.Context) error {
			called.Store(true)
			return nil
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	s.Start(ctx)
	defer cancel()
	time.Sleep(100 * time.Millisecond)
	s.Stop()
	if !called.Load() {
		t.Error("expected job to be called")
	}
}

func TestScheduler_MultipleJobs(t *testing.T) {
	var count atomic.Int32
	s := NewScheduler()
	for i := 0; i < 3; i++ {
		s.AddJob(&Job{
			Name:     "job",
			Schedule: Every(10 * time.Millisecond),
			Func: func(ctx context.Context) error {
				count.Add(1)
				return nil
			},
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	s.Start(ctx)
	defer cancel()
	time.Sleep(100 * time.Millisecond)
	s.Stop()
	if count.Load() < 3 {
		t.Errorf("expected at least 3 calls, got %d", count.Load())
	}
}