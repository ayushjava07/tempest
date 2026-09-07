package deadlock

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrTaskNotFound = errors.New("deadlock: task not found")
	ErrNoVictim     = errors.New("deadlock: no victim could be selected")
)

// VictimPolicy defines how to select which task to terminate to break a deadlock.
type VictimPolicy int

const (
	VictimLowestPriority VictimPolicy = iota
	VictimYoungest
	VictimFewestLocks
)

// TaskInfo represents a task in the dependency graph.
type TaskInfo struct {
	ID          string
	WorkflowID  string
	Priority    int
	StartedAt   time.Time
	HeldLocks   map[string]bool
	WaitingLock string
}

// Detector tracks lock acquisitions and analyzes wait-for graphs for cycles.
type Detector struct {
	mu          sync.RWMutex
	tasks       map[string]*TaskInfo
	lockHolders map[string]string   // lockID -> taskID holding it
	lockWaiters map[string][]string // lockID -> list of taskIDs waiting for it
	stopCh      chan struct{}
	wg          sync.WaitGroup
	running     bool
}

// New creates a new deadlock Detector.
func New() *Detector {
	return &Detector{
		tasks:       make(map[string]*TaskInfo),
		lockHolders: make(map[string]string),
		lockWaiters: make(map[string][]string),
		stopCh:      make(chan struct{}),
	}
}

// RegisterTask registers a task with priority and start time.
func (d *Detector) RegisterTask(id, workflowID string, priority int, startedAt time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.tasks[id] = &TaskInfo{
		ID:         id,
		WorkflowID: workflowID,
		Priority:   priority,
		StartedAt:  startedAt,
		HeldLocks:  make(map[string]bool),
	}
}

// UnregisterTask removes a task and frees all its associated locks.
func (d *Detector) UnregisterTask(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	t, ok := d.tasks[id]
	if !ok {
		return
	}

	// Release any held locks
	for lockID := range t.HeldLocks {
		if d.lockHolders[lockID] == id {
			delete(d.lockHolders, lockID)
		}
	}

	// Clear waiting lock
	if t.WaitingLock != "" {
		waiters := d.lockWaiters[t.WaitingLock]
		filtered := make([]string, 0, len(waiters))
		for _, w := range waiters {
			if w != id {
				filtered = append(filtered, w)
			}
		}
		d.lockWaiters[t.WaitingLock] = filtered
	}

	delete(d.tasks, id)
}

// AcquireLock grants lockID to taskID if available.
func (d *Detector) AcquireLock(taskID, lockID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	t, ok := d.tasks[taskID]
	if !ok {
		return false, ErrTaskNotFound
	}

	holder, held := d.lockHolders[lockID]
	if held && holder != taskID {
		// Cannot acquire immediately, record waiting state
		t.WaitingLock = lockID
		d.lockWaiters[lockID] = append(d.lockWaiters[lockID], taskID)
		return false, nil
	}

	// Granted
	d.lockHolders[lockID] = taskID
	t.HeldLocks[lockID] = true
	t.WaitingLock = ""
	return true, nil
}

// ReleaseLock relinquishes ownership of lockID.
func (d *Detector) ReleaseLock(taskID, lockID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	t, ok := d.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}

	delete(t.HeldLocks, lockID)
	if d.lockHolders[lockID] == taskID {
		delete(d.lockHolders, lockID)
	}

	return nil
}

// BuildWaitForGraph constructs the directed wait-for graph:
// If Task A is waiting for lock L, and Task B holds lock L, an edge A -> B is added.
func (d *Detector) BuildWaitForGraph() map[string][]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	wfg := make(map[string][]string)

	for taskID, task := range d.tasks {
		if task.WaitingLock != "" {
			if holder, ok := d.lockHolders[task.WaitingLock]; ok && holder != taskID {
				wfg[taskID] = append(wfg[taskID], holder)
			}
		}
	}

	return wfg
}

// DetectDeadlocks searches the wait-for graph for cyclic wait dependencies.
func (d *Detector) DetectDeadlocks() [][]string {
	wfg := d.BuildWaitForGraph()

	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var path []string
	var cycles [][]string

	var dfs func(u string)
	dfs = func(u string) {
		visited[u] = true
		recStack[u] = true
		path = append(path, u)

		for _, v := range wfg[u] {
			if !visited[v] {
				dfs(v)
			} else if recStack[v] {
				// Found cycle! Extract path from v to end
				cycleStart := -1
				for i, node := range path {
					if node == v {
						cycleStart = i
						break
					}
				}
				if cycleStart != -1 {
					cycle := make([]string, len(path)-cycleStart)
					copy(cycle, path[cycleStart:])
					cycles = append(cycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		recStack[u] = false
	}

	for u := range wfg {
		if !visited[u] {
			dfs(u)
		}
	}

	return cycles
}

// SelectVictim chooses the best candidate to abort according to policy.
func (d *Detector) SelectVictim(cycle []string, policy VictimPolicy) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(cycle) == 0 {
		return "", ErrNoVictim
	}

	candidates := make([]*TaskInfo, 0, len(cycle))
	for _, id := range cycle {
		if t, ok := d.tasks[id]; ok {
			candidates = append(candidates, t)
		}
	}

	if len(candidates) == 0 {
		return "", ErrNoVictim
	}

	switch policy {
	case VictimLowestPriority:
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Priority != candidates[j].Priority {
				return candidates[i].Priority < candidates[j].Priority // lowest first
			}
			return candidates[i].StartedAt.After(candidates[j].StartedAt) // younger tie-break
		})
	case VictimYoungest:
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].StartedAt.After(candidates[j].StartedAt)
		})
	case VictimFewestLocks:
		sort.Slice(candidates, func(i, j int) bool {
			return len(candidates[i].HeldLocks) < len(candidates[j].HeldLocks)
		})
	}

	return candidates[0].ID, nil
}

// ResolveDeadlocks runs deadlock detection and invokes abortCallback for each chosen victim.
func (d *Detector) ResolveDeadlocks(policy VictimPolicy, abortCallback func(taskID string)) int {
	cycles := d.DetectDeadlocks()
	resolvedCount := 0
	aborted := make(map[string]bool)

	for _, cycle := range cycles {
		// Check if cycle was already broken
		alreadyBroken := false
		for _, node := range cycle {
			if aborted[node] {
				alreadyBroken = true
				break
			}
		}
		if alreadyBroken {
			continue
		}

		victim, err := d.SelectVictim(cycle, policy)
		if err == nil && victim != "" {
			aborted[victim] = true
			d.UnregisterTask(victim)
			if abortCallback != nil {
				abortCallback(victim)
			}
			resolvedCount++
		}
	}

	return resolvedCount
}

// StartSweeper launches background periodic deadlock inspection.
func (d *Detector) StartSweeper(ctx context.Context, interval time.Duration, policy VictimPolicy, abortCallback func(taskID string)) {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-d.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.ResolveDeadlocks(policy, abortCallback)
			}
		}
	}()
}

// Stop cleanly stops the background sweeper.
func (d *Detector) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	close(d.stopCh)
	d.mu.Unlock()

	d.wg.Wait()
}
