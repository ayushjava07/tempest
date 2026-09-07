package cluster

import (
	"errors"
	"math"
	"sync"
	"time"
)

var (
	ErrNodeNotFound     = errors.New("cluster: node not found")
	ErrNodeAlreadyAlive = errors.New("cluster: node is already registered")
)

type NodeState string

const (
	StateAlive   NodeState = "ALIVE"
	StateSuspect NodeState = "SUSPECT"
	StateDead    NodeState = "DEAD"
)

// Node represents a member instance in the Tempest cluster.
type Node struct {
	ID             string
	Address        string
	Role           string
	State          NodeState
	LastHeartbeat  time.Time
	history        []float64 // Inter-arrival intervals in seconds
	maxHistorySize int
}

func newNode(id, address, role string, maxHistory int) *Node {
	if maxHistory <= 0 {
		maxHistory = 50
	}
	return &Node{
		ID:             id,
		Address:        address,
		Role:           role,
		State:          StateAlive,
		LastHeartbeat:  time.Now().UTC(),
		history:        make([]float64, 0, maxHistory),
		maxHistorySize: maxHistory,
	}
}

func (n *Node) recordHeartbeat(now time.Time) {
	if !n.LastHeartbeat.IsZero() {
		interval := now.Sub(n.LastHeartbeat).Seconds()
		if len(n.history) >= n.maxHistorySize {
			n.history = n.history[1:]
		}
		n.history = append(n.history, interval)
	}
	n.LastHeartbeat = now
	n.State = StateAlive
}

// Phi computes the suspicion score using the Phi Accrual failure detection formula.
// Phi = -log10(P_later(elapsed)) where P_later is estimated using normal CDF.
func (n *Node) Phi(now time.Time) float64 {
	if len(n.history) < 2 {
		return 0.0
	}

	elapsed := now.Sub(n.LastHeartbeat).Seconds()

	// Compute mean and standard deviation of heartbeat intervals
	sum := 0.0
	for _, v := range n.history {
		sum += v
	}
	mean := sum / float64(len(n.history))

	variance := 0.0
	for _, v := range n.history {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(n.history))
	stdDev := math.Sqrt(variance)
	minStdDev := 0.25 * mean
	if minStdDev < 0.1 {
		minStdDev = 0.1
	}
	if stdDev < minStdDev {
		stdDev = minStdDev
	}

	// Normal distribution survival function: 1 - CDF(elapsed)
	// Approximation using error function erfc: 0.5 * erfc(y / sqrt(2)) where y = (elapsed - mean) / stdDev
	y := (elapsed - mean) / stdDev
	if y <= 0 {
		return 0.0
	}

	pLater := 0.5 * math.Erfc(y/math.Sqrt2)
	if pLater <= 1e-15 {
		pLater = 1e-15 // Prevent -log10(0) infinity
	}

	return -math.Log10(pLater)
}

// Config tunes the failure detector thresholds.
type Config struct {
	SuspectThreshold float64 // Typically 8.0
	DeadThreshold    float64 // Typically 12.0
	HeartbeatHistory int
}

func DefaultConfig() Config {
	return Config{
		SuspectThreshold: 8.0,
		DeadThreshold:    12.0,
		HeartbeatHistory: 50,
	}
}

// Coordinator monitors cluster node health.
type Coordinator struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	cfg   Config
}

func NewCoordinator(cfg Config) *Coordinator {
	if cfg.SuspectThreshold <= 0 {
		cfg.SuspectThreshold = 8.0
	}
	if cfg.DeadThreshold <= cfg.SuspectThreshold {
		cfg.DeadThreshold = cfg.SuspectThreshold + 4.0
	}
	return &Coordinator{
		nodes: make(map[string]*Node),
		cfg:   cfg,
	}
}

// Register adds a new node to the cluster.
func (c *Coordinator) Register(id, address, role string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[id]; exists {
		return ErrNodeAlreadyAlive
	}

	c.nodes[id] = newNode(id, address, role, c.cfg.HeartbeatHistory)
	return nil
}

// Deregister removes a node upon graceful shutdown.
func (c *Coordinator) Deregister(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[id]; !exists {
		return ErrNodeNotFound
	}
	delete(c.nodes, id)
	return nil
}

// Heartbeat updates the last observed heartbeat timestamp from node.
func (c *Coordinator) Heartbeat(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	n, ok := c.nodes[id]
	if !ok {
		return ErrNodeNotFound
	}

	n.recordHeartbeat(time.Now().UTC())
	return nil
}

// EvaluateHealth recalculates Phi for all nodes and updates their lifecycle states.
func (c *Coordinator) EvaluateHealth(now time.Time) map[string]NodeState {
	c.mu.Lock()
	defer c.mu.Unlock()

	states := make(map[string]NodeState, len(c.nodes))
	for id, n := range c.nodes {
		phi := n.Phi(now)
		if phi >= c.cfg.DeadThreshold {
			n.State = StateDead
		} else if phi >= c.cfg.SuspectThreshold {
			n.State = StateSuspect
		} else {
			n.State = StateAlive
		}
		states[id] = n.State
	}
	return states
}

// ActiveNodes returns all currently ALIVE nodes.
func (c *Coordinator) ActiveNodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var active []string
	for id, n := range c.nodes {
		if n.State == StateAlive {
			active = append(active, id)
		}
	}
	return active
}
