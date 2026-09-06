package lock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrLockAcquisitionTimeout = errors.New("lock: acquisition timeout exceeded")
	ErrLockHeld               = errors.New("lock: already held by another owner")
	ErrLockNotHeld            = errors.New("lock: not held by specified owner")
	ErrDeadlockDetected       = errors.New("lock: deadlock detected in wait-for graph")
	ErrInvalidLease           = errors.New("lock: lease duration must be positive")
)

// LockMode designates shared read vs exclusive write lock semantics.
type LockMode string

const (
	ModeShared    LockMode = "SHARED"
	ModeExclusive LockMode = "EXCLUSIVE"
)

// Lease encapsulates an acquired lock with a monotonic fencing token.
type Lease struct {
	Resource     string
	Owner        string
	Mode         LockMode
	FencingToken uint64
	ExpiresAt    time.Time
	RenewedCount int
}

// Coordinator manages distributed multi-resource locking and deadlock detection.
type Coordinator struct {
	mu           sync.Mutex
	fencingSeq   atomic.Uint64
	locks        map[string]*resourceLock
	waitForGraph map[string]map[string]bool // owner -> map of owners it is waiting on
}

type resourceLock struct {
	resource       string
	exclusive      bool
	exclusiveOwner string
	sharedOwners   map[string]int
	fencingToken   uint64
	expiresAt      time.Time
}

func NewCoordinator() *Coordinator {
	return &Coordinator{
		locks:        make(map[string]*resourceLock),
		waitForGraph: make(map[string]map[string]bool),
	}
}

// Acquire attempts to acquire the named lock, performing deadlock detection if waiting.
func (c *Coordinator) Acquire(ctx context.Context, resource, owner string, mode LockMode, ttl time.Duration) (*Lease, error) {
	if ttl <= 0 {
		return nil, ErrInvalidLease
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		c.mu.Lock()
		resLock, exists := c.locks[resource]

		// Check if expired
		if exists && time.Now().After(resLock.expiresAt) {
			delete(c.locks, resource)
			exists = false
		}

		canAcquire := false
		if !exists {
			canAcquire = true
		} else if mode == ModeShared && !resLock.exclusive {
			canAcquire = true
		} else if mode == ModeExclusive && resLock.exclusive && resLock.exclusiveOwner == owner {
			// Re-entrant exclusive lock
			canAcquire = true
		} else if mode == ModeShared && !resLock.exclusive && resLock.sharedOwners[owner] > 0 {
			// Re-entrant shared lock
			canAcquire = true
		}

		if canAcquire {
			c.removeWaiter(owner)
			token := c.fencingSeq.Add(1)
			expiresAt := time.Now().Add(ttl)

			if !exists {
				resLock = &resourceLock{
					resource:     resource,
					sharedOwners: make(map[string]int),
				}
				c.locks[resource] = resLock
			}

			resLock.fencingToken = token
			resLock.expiresAt = expiresAt

			if mode == ModeExclusive {
				resLock.exclusive = true
				resLock.exclusiveOwner = owner
			} else {
				resLock.sharedOwners[owner]++
			}

			lease := &Lease{
				Resource:     resource,
				Owner:        owner,
				Mode:         mode,
				FencingToken: token,
				ExpiresAt:    expiresAt,
			}
			c.mu.Unlock()
			return lease, nil
		}

		// Cannot acquire immediately. Update Wait-For Graph (WFG)
		var currentHolders []string
		if resLock.exclusive {
			currentHolders = append(currentHolders, resLock.exclusiveOwner)
		} else {
			for holder := range resLock.sharedOwners {
				currentHolders = append(currentHolders, holder)
			}
		}

		for _, holder := range currentHolders {
			if holder != owner {
				c.addWaiter(owner, holder)
			}
		}

		// Check for cycle in Wait-For-Graph
		if c.hasCycle(owner) {
			c.removeWaiter(owner)
			c.mu.Unlock()
			return nil, fmt.Errorf("%w: owner %s caused cycle", ErrDeadlockDetected, owner)
		}

		c.mu.Unlock()

		select {
		case <-ctx.Done():
			c.mu.Lock()
			c.removeWaiter(owner)
			c.mu.Unlock()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Release relinquishes the lock.
func (c *Coordinator) Release(resource, owner string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	resLock, ok := c.locks[resource]
	if !ok {
		return ErrLockNotHeld
	}

	if resLock.exclusive {
		if resLock.exclusiveOwner != owner {
			return ErrLockNotHeld
		}
		delete(c.locks, resource)
	} else {
		count := resLock.sharedOwners[owner]
		if count <= 0 {
			return ErrLockNotHeld
		}
		if count == 1 {
			delete(resLock.sharedOwners, owner)
		} else {
			resLock.sharedOwners[owner]--
		}
		if len(resLock.sharedOwners) == 0 {
			delete(c.locks, resource)
		}
	}

	c.removeWaiter(owner)
	return nil
}

// Renew extends an active lock's expiration.
func (c *Coordinator) Renew(resource, owner string, ttl time.Duration) (*Lease, error) {
	if ttl <= 0 {
		return nil, ErrInvalidLease
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	resLock, ok := c.locks[resource]
	if !ok || time.Now().After(resLock.expiresAt) {
		return nil, ErrLockNotHeld
	}

	if resLock.exclusive && resLock.exclusiveOwner != owner {
		return nil, ErrLockNotHeld
	}
	if !resLock.exclusive && resLock.sharedOwners[owner] <= 0 {
		return nil, ErrLockNotHeld
	}

	resLock.expiresAt = time.Now().Add(ttl)
	return &Lease{
		Resource:     resource,
		Owner:        owner,
		FencingToken: resLock.fencingToken,
		ExpiresAt:    resLock.expiresAt,
	}, nil
}

func (c *Coordinator) addWaiter(waiter, holder string) {
	if c.waitForGraph[waiter] == nil {
		c.waitForGraph[waiter] = make(map[string]bool)
	}
	c.waitForGraph[waiter][holder] = true
}

func (c *Coordinator) removeWaiter(waiter string) {
	delete(c.waitForGraph, waiter)
	for _, holders := range c.waitForGraph {
		delete(holders, waiter)
	}
}

// hasCycle detects directed cycles in the Wait-For-Graph starting from startNode.
func (c *Coordinator) hasCycle(startNode string) bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true

		for neighbor := range c.waitForGraph[node] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if recStack[neighbor] {
				return true
			}
		}

		recStack[node] = false
		return false
	}

	return dfs(startNode)
}
