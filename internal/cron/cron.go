package cron

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Job struct {
	Name     string
	Schedule Schedule
	Func     func(ctx context.Context) error
}

type Schedule interface {
	Next(time.Time) time.Time
}

type EverySchedule struct {
	Interval time.Duration
}

func (s EverySchedule) Next(after time.Time) time.Time {
	return after.Add(s.Interval)
}

type AtSchedule struct {
	Times []string
}

func (s AtSchedule) Next(after time.Time) time.Time {
	for _, t := range s.Times {
		h, m := parseTime(t)
		next := time.Date(after.Year(), after.Month(), after.Day(), h, m, 0, 0, after.Location())
		if next.After(after) {
			return next
		}
	}
	h, m := parseTime(s.Times[0])
	return time.Date(after.Year(), after.Month(), after.Day()+1, h, m, 0, 0, after.Location())
}

func parseTime(s string) (int, int) {
	var h, m int
	_, _ = fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m
}

type jobState struct {
	job     *Job
	nextRun time.Time
}

type Scheduler struct {
	mu     sync.Mutex
	jobs   []*jobState
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		stopCh: make(chan struct{}),
	}
}

func (s *Scheduler) AddJob(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, &jobState{job: job, nextRun: time.Now()})
}

func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.run(ctx)
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for _, js := range s.jobs {
				if now.After(js.nextRun) || now.Equal(js.nextRun) {
					js.nextRun = js.job.Schedule.Next(now)
					fn := js.job.Func
					s.wg.Add(1)
					go func() {
						defer s.wg.Done()
						_ = fn(ctx)
					}()
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) JobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

func Every(interval time.Duration) Schedule {
	return EverySchedule{Interval: interval}
}

func At(times ...string) Schedule {
	return AtSchedule{Times: times}
}
