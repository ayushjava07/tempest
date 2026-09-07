package lane

import (
	"context"
	"fmt"
	"sync"
)

// DRRScheduler coordinates deficit round-robin scheduling across multiple priority lanes.
type DRRScheduler struct {
	mu           sync.Mutex
	lanes        []*Lane
	laneMap      map[string]*Lane
	roundIndex   int
	quantumAdded bool
	totalTasks   int
	cond         *sync.Cond
}

// NewDRRScheduler creates a DRR scheduler managing the provided lanes.
func NewDRRScheduler(lanes []*Lane) (*DRRScheduler, error) {
	if len(lanes) == 0 {
		return nil, fmt.Errorf("at least one lane required")
	}

	laneMap := make(map[string]*Lane, len(lanes))
	for _, l := range lanes {
		if _, exists := laneMap[l.ID()]; exists {
			return nil, fmt.Errorf("duplicate lane ID: %s", l.ID())
		}
		laneMap[l.ID()] = l
	}

	s := &DRRScheduler{
		lanes:        lanes,
		laneMap:      laneMap,
		roundIndex:   0,
		quantumAdded: false,
	}
	s.cond = sync.NewCond(&s.mu)
	return s, nil
}

// Enqueue routes a task to its designated lane and wakes any waiting schedulers.
func (s *DRRScheduler) Enqueue(task *Task) error {
	s.mu.Lock()
	l, exists := s.laneMap[task.LaneID]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("%w: %s", ErrLaneNotFound, task.LaneID)
	}

	if err := l.Enqueue(task); err != nil {
		return err
	}

	s.mu.Lock()
	s.totalTasks++
	s.cond.Signal()
	s.mu.Unlock()
	return nil
}

// ScheduleNext blocks until a task is eligible for dispatch under DRR or context cancellation.
func (s *DRRScheduler) ScheduleNext(ctx context.Context) (*Task, error) {
	doneChan := ctx.Done()

	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		select {
		case <-doneChan:
			return nil, ctx.Err()
		default:
		}

		if s.totalTasks == 0 {
			waitCh := make(chan struct{})
			go func() {
				select {
				case <-doneChan:
					s.mu.Lock()
					s.cond.Broadcast()
					s.mu.Unlock()
				case <-waitCh:
				}
			}()

			s.cond.Wait()
			close(waitCh)

			select {
			case <-doneChan:
				return nil, ctx.Err()
			default:
			}
		}

		if s.totalTasks == 0 {
			continue
		}

		// 1. Anti-starvation defense
		for _, l := range s.lanes {
			if l.Len() > 0 && l.IsStarving() {
				if t, ok := l.Dequeue(); ok {
					t.AgeBoosted = true
					s.totalTasks--
					return t, nil
				}
			}
		}

		// 2. Deficit Round-Robin progression
		numLanes := len(s.lanes)
		for attempts := 0; attempts < numLanes*2; attempts++ {
			lane := s.lanes[s.roundIndex]

			lane.mu.Lock()
			if len(lane.tasks) == 0 {
				lane.deficit = 0
				lane.mu.Unlock()
				s.roundIndex = (s.roundIndex + 1) % numLanes
				s.quantumAdded = false
				continue
			}

			if !s.quantumAdded {
				lane.deficit += lane.cfg.Quantum
				s.quantumAdded = true
			}

			head := lane.tasks[0]
			if lane.deficit >= head.Cost {
				// Dequeue task
				task := lane.tasks[0]
				lane.tasks = lane.tasks[1:]
				lane.deficit -= task.Cost
				lane.dequeuedCount++

				if len(lane.tasks) == 0 {
					lane.deficit = 0
					s.roundIndex = (s.roundIndex + 1) % numLanes
					s.quantumAdded = false
				} else if lane.deficit < lane.tasks[0].Cost {
					// Next task cannot be served in this turn, advance lane
					s.roundIndex = (s.roundIndex + 1) % numLanes
					s.quantumAdded = false
				}
				lane.mu.Unlock()

				s.totalTasks--
				return task, nil
			}

			// Cannot serve head task, advance to next lane
			lane.mu.Unlock()
			s.roundIndex = (s.roundIndex + 1) % numLanes
			s.quantumAdded = false
		}
	}
}

// TotalTasks returns the count of queued tasks across all lanes.
func (s *DRRScheduler) TotalTasks() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalTasks
}

// LaneIDs returns all managed lane identifiers.
func (s *DRRScheduler) LaneIDs() []string {
	ids := make([]string, len(s.lanes))
	for i, l := range s.lanes {
		ids[i] = l.ID()
	}
	return ids
}
