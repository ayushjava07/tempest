package cronengine

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrScheduleNotFound  = errors.New("cronengine: schedule not found")
	ErrScheduleExists    = errors.New("cronengine: schedule already exists")
	ErrInvalidExpression = errors.New("cronengine: invalid cron expression")
	ErrSchedulerStopped  = errors.New("cronengine: scheduler is stopped")
)

// MissedPolicy determines behavior when a schedule's fire window passed during downtime.
type MissedPolicy int

const (
	MissedFireSkip MissedPolicy = iota
	MissedFireRunOnce
	MissedFireRunAll
)

// OverlapPolicy determines behavior when the previous execution is still running.
type OverlapPolicy int

const (
	OverlapAllow OverlapPolicy = iota
	OverlapForbid
	OverlapReplace
)

// ExecutionRecord tracks a single scheduled run.
type ExecutionRecord struct {
	ExecutionID string
	ScheduleID  string
	ScheduledAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	Duration    time.Duration
	Err         error
	Success     bool
}

// CronField bitset for 0-63 range.
type CronField uint64

func (cf CronField) Matches(val int) bool {
	if val < 0 || val > 63 {
		return false
	}
	return (cf & (1 << uint(val))) != 0
}

// Expression represents a parsed 5-field cron spec.
type Expression struct {
	Minutes CronField
	Hours   CronField
	DOM     CronField
	Months  CronField
	DOW     CronField
}

// ParseExpression parses standard 5-field cron syntax: min hour dom month dow
func ParseExpression(spec string) (*Expression, error) {
	parts := strings.Fields(spec)
	if len(parts) != 5 {
		return nil, fmt.Errorf("%w: expected 5 fields, got %d", ErrInvalidExpression, len(parts))
	}

	min, err := parseField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("%w (minute): %v", ErrInvalidExpression, err)
	}

	hr, err := parseField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("%w (hour): %v", ErrInvalidExpression, err)
	}

	dom, err := parseField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("%w (day of month): %v", ErrInvalidExpression, err)
	}

	mon, err := parseField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("%w (month): %v", ErrInvalidExpression, err)
	}

	dow, err := parseField(parts[4], 0, 6)
	if err != nil {
		return nil, fmt.Errorf("%w (day of week): %v", ErrInvalidExpression, err)
	}

	return &Expression{
		Minutes: min,
		Hours:   hr,
		DOM:     dom,
		Months:  mon,
		DOW:     dow,
	}, nil
}

func parseField(f string, min, max int) (CronField, error) {
	var field CronField
	for _, part := range strings.Split(f, ",") {
		part = strings.TrimSpace(part)
		step := 1
		rangeStr := part

		if strings.Contains(part, "/") {
			sub := strings.Split(part, "/")
			if len(sub) != 2 {
				return 0, fmt.Errorf("invalid step format: %s", part)
			}
			var err error
			step, err = strconv.Atoi(sub[1])
			if err != nil || step <= 0 {
				return 0, fmt.Errorf("invalid step value: %s", sub[1])
			}
			rangeStr = sub[0]
		}

		var start, end int
		if rangeStr == "*" {
			start = min
			end = max
		} else if strings.Contains(rangeStr, "-") {
			r := strings.Split(rangeStr, "-")
			if len(r) != 2 {
				return 0, fmt.Errorf("invalid range format: %s", rangeStr)
			}
			var err error
			start, err = strconv.Atoi(r[0])
			if err != nil || start < min || start > max {
				return 0, fmt.Errorf("range start out of bounds: %s", r[0])
			}
			end, err = strconv.Atoi(r[1])
			if err != nil || end < min || end > max || end < start {
				return 0, fmt.Errorf("range end out of bounds: %s", r[1])
			}
		} else {
			val, err := strconv.Atoi(rangeStr)
			if err != nil || val < min || val > max {
				return 0, fmt.Errorf("value out of bounds [%d-%d]: %s", min, max, rangeStr)
			}
			start = val
			end = val
		}

		for i := start; i <= end; i += step {
			field |= (1 << uint(i))
		}
	}
	return field, nil
}

// Next finds the earliest time after `after` that matches the expression in `loc`.
func (e *Expression) Next(after time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)

	// Search up to 5 years into the future
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if !e.Months.Matches(int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}
		if !e.DOM.Matches(t.Day()) || !e.DOW.Matches(int(t.Weekday())) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if !e.Hours.Matches(t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			continue
		}
		if !e.Minutes.Matches(t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

// HandlerFunc is executed when a schedule triggers.
type HandlerFunc func(ctx context.Context, scheduledAt time.Time) error

// Schedule represents a registered cron schedule.
type Schedule struct {
	ID            string
	WorkflowID    string
	ExpressionStr string
	expr          *Expression
	Location      *time.Location
	NextRun       time.Time
	LastRun       time.Time
	IsPaused      bool
	MissedPolicy  MissedPolicy
	OverlapPolicy OverlapPolicy
	Handler       HandlerFunc

	// internal runtime
	heapIndex  int
	runningCtx context.Context
	cancelFunc context.CancelFunc
	running    bool
	mu         sync.Mutex
}

// priority queue implementation for heap.Interface
type scheduleQueue []*Schedule

func (pq scheduleQueue) Len() int           { return len(pq) }
func (pq scheduleQueue) Less(i, j int) bool { return pq[i].NextRun.Before(pq[j].NextRun) }
func (pq scheduleQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].heapIndex = i
	pq[j].heapIndex = j
}
func (pq *scheduleQueue) Push(x any) {
	item := x.(*Schedule)
	item.heapIndex = len(*pq)
	*pq = append(*pq, item)
}
func (pq *scheduleQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.heapIndex = -1
	*pq = old[0 : n-1]
	return item
}

// EngineConfig configuration options for the cron engine.
type EngineConfig struct {
	ClockSkewAllowance time.Duration
	MaxHistoryPerJob   int
}

// DefaultEngineConfig returns sane defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		ClockSkewAllowance: 50 * time.Millisecond,
		MaxHistoryPerJob:   100,
	}
}

// Engine is the central recurring schedule orchestrator.
type Engine struct {
	mu        sync.Mutex
	cfg       EngineConfig
	schedules map[string]*Schedule
	queue     scheduleQueue
	history   map[string][]ExecutionRecord
	wakeCh    chan struct{}
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   atomic.Bool
	execSeq   atomic.Uint64
}

// New creates a new cron scheduler Engine.
func New(cfg EngineConfig) *Engine {
	e := &Engine{
		cfg:       cfg,
		schedules: make(map[string]*Schedule),
		history:   make(map[string][]ExecutionRecord),
		wakeCh:    make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}
	heap.Init(&e.queue)
	return e
}

// AddSchedule registers a new schedule in the engine.
func (e *Engine) AddSchedule(
	id, workflowID, cronExpr string,
	loc *time.Location,
	missed MissedPolicy,
	overlap OverlapPolicy,
	handler HandlerFunc,
) error {
	if loc == nil {
		loc = time.UTC
	}
	expr, err := ParseExpression(cronExpr)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.schedules[id]; exists {
		return ErrScheduleExists
	}

	now := time.Now().In(loc)
	next := expr.Next(now, loc)

	s := &Schedule{
		ID:            id,
		WorkflowID:    workflowID,
		ExpressionStr: cronExpr,
		expr:          expr,
		Location:      loc,
		NextRun:       next,
		MissedPolicy:  missed,
		OverlapPolicy: overlap,
		Handler:       handler,
	}

	e.schedules[id] = s
	heap.Push(&e.queue, s)

	e.signalWake()
	return nil
}

// PauseSchedule halts upcoming runs of a schedule.
func (e *Engine) PauseSchedule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.schedules[id]
	if !ok {
		return ErrScheduleNotFound
	}

	s.IsPaused = true
	return nil
}

// ResumeSchedule resumes a paused schedule and computes the next trigger time.
func (e *Engine) ResumeSchedule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.schedules[id]
	if !ok {
		return ErrScheduleNotFound
	}

	if !s.IsPaused {
		return nil
	}
	s.IsPaused = false
	now := time.Now().In(s.Location)
	s.NextRun = s.expr.Next(now, s.Location)
	if s.heapIndex >= 0 {
		heap.Fix(&e.queue, s.heapIndex)
	} else {
		heap.Push(&e.queue, s)
	}

	e.signalWake()
	return nil
}

// RemoveSchedule cancels and removes a schedule completely.
func (e *Engine) RemoveSchedule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.schedules[id]
	if !ok {
		return ErrScheduleNotFound
	}

	if s.heapIndex >= 0 {
		heap.Remove(&e.queue, s.heapIndex)
	}
	delete(e.schedules, id)

	s.mu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
	s.mu.Unlock()

	return nil
}

// Start boots the scheduler loop in the background.
func (e *Engine) Start(ctx context.Context) error {
	if !e.running.CompareAndSwap(false, true) {
		return errors.New("cronengine: already started")
	}

	e.wg.Add(1)
	go e.loop(ctx)
	return nil
}

// Stop gracefully terminates the scheduler engine and waits for running jobs.
func (e *Engine) Stop() {
	if !e.running.CompareAndSwap(true, false) {
		return
	}
	close(e.stopCh)
	e.wg.Wait()
}

func (e *Engine) signalWake() {
	select {
	case e.wakeCh <- struct{}{}:
	default:
	}
}

func (e *Engine) loop(ctx context.Context) {
	defer e.wg.Done()

	for {
		e.mu.Lock()
		now := time.Now()
		var sleepDuration time.Duration

		if len(e.queue) == 0 {
			sleepDuration = 24 * time.Hour
		} else {
			earliest := e.queue[0]
			if !earliest.NextRun.After(now.Add(e.cfg.ClockSkewAllowance)) {
				// Due now! Pop and execute
				s := heap.Pop(&e.queue).(*Schedule)
				triggerTime := s.NextRun

				// Advance next run
				s.LastRun = triggerTime
				s.NextRun = s.expr.Next(triggerTime, s.Location)
				if !s.IsPaused {
					heap.Push(&e.queue, s)
				}
				e.mu.Unlock()

				e.dispatch(s, triggerTime)
				continue
			} else {
				sleepDuration = earliest.NextRun.Sub(now)
			}
		}
		e.mu.Unlock()

		timer := time.NewTimer(sleepDuration)
		select {
		case <-e.stopCh:
			timer.Stop()
			return
		case <-ctx.Done():
			timer.Stop()
			return
		case <-e.wakeCh:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (e *Engine) dispatch(s *Schedule, scheduledAt time.Time) {
	s.mu.Lock()
	if s.IsPaused {
		s.mu.Unlock()
		return
	}

	if s.running {
		switch s.OverlapPolicy {
		case OverlapForbid:
			s.mu.Unlock()
			e.recordExecution(s.ID, scheduledAt, time.Now(), time.Now(), errors.New("cronengine: run skipped due to OverlapForbid"))
			return
		case OverlapReplace:
			if s.cancelFunc != nil {
				s.cancelFunc()
			}
		case OverlapAllow:
			// Allow concurrent execution
		}
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.running = true
	s.runningCtx = runCtx
	s.cancelFunc = cancel
	s.mu.Unlock()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		started := time.Now()
		var err error

		if s.Handler != nil {
			err = s.Handler(runCtx, scheduledAt)
		}

		finished := time.Now()

		s.mu.Lock()
		s.running = false
		s.cancelFunc = nil
		s.runningCtx = nil
		s.mu.Unlock()

		e.recordExecution(s.ID, scheduledAt, started, finished, err)
	}()
}

func (e *Engine) recordExecution(scheduleID string, scheduled, started, finished time.Time, err error) {
	rec := ExecutionRecord{
		ExecutionID: fmt.Sprintf("exec-%d", e.execSeq.Add(1)),
		ScheduleID:  scheduleID,
		ScheduledAt: scheduled,
		StartedAt:   started,
		FinishedAt:  finished,
		Duration:    finished.Sub(started),
		Err:         err,
		Success:     err == nil,
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	list := e.history[scheduleID]
	list = append(list, rec)
	if e.cfg.MaxHistoryPerJob > 0 && len(list) > e.cfg.MaxHistoryPerJob {
		list = list[len(list)-e.cfg.MaxHistoryPerJob:]
	}
	e.history[scheduleID] = list
}

// GetHistory returns past execution records for a schedule.
func (e *Engine) GetHistory(scheduleID string) []ExecutionRecord {
	e.mu.Lock()
	defer e.mu.Unlock()

	h := e.history[scheduleID]
	res := make([]ExecutionRecord, len(h))
	copy(res, h)
	return res
}

// TriggerNow forces an immediate execution outside of normal cron cadence.
func (e *Engine) TriggerNow(id string) error {
	e.mu.Lock()
	s, ok := e.schedules[id]
	if !ok {
		e.mu.Unlock()
		return ErrScheduleNotFound
	}
	e.mu.Unlock()

	e.dispatch(s, time.Now())
	return nil
}

// GetSchedule returns a snapshot of a schedule.
func (e *Engine) GetSchedule(id string) (*Schedule, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	s, ok := e.schedules[id]
	if !ok {
		return nil, ErrScheduleNotFound
	}

	return &Schedule{
		ID:            s.ID,
		WorkflowID:    s.WorkflowID,
		ExpressionStr: s.ExpressionStr,
		expr:          s.expr,
		Location:      s.Location,
		NextRun:       s.NextRun,
		LastRun:       s.LastRun,
		IsPaused:      s.IsPaused,
		MissedPolicy:  s.MissedPolicy,
		OverlapPolicy: s.OverlapPolicy,
		Handler:       s.Handler,
	}, nil
}
