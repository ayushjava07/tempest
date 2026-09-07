package lane

import (
	"sync"
	"time"
)

// LaneLatencyMetrics tracks moving average wait times and current dynamic quantum.
type LaneLatencyMetrics struct {
	LaneID         string        `json:"lane_id"`
	CurrentQuantum int           `json:"current_quantum"`
	BaseQuantum    int           `json:"base_quantum"`
	EMALatency     time.Duration `json:"ema_latency"`
	SampleCount    uint64        `json:"sample_count"`
}

// LatencyTuner adjusts lane scheduling quantums based on measured queue wait telemetry.
type LatencyTuner struct {
	mu           sync.Mutex
	lanes        map[string]*Lane
	baseQuantums map[string]int
	emaLatency   map[string]time.Duration
	sampleCount  map[string]uint64
	alpha        float64 // EMA smoothing factor (e.g. 0.2)
	maxBoost     int     // maximum multiplier over base quantum (e.g. 5x)
}

// NewLatencyTuner initializes a dynamic weight tuner.
func NewLatencyTuner(lanes []*Lane, alpha float64, maxBoost int) *LatencyTuner {
	if alpha <= 0 || alpha > 1.0 {
		alpha = 0.2
	}
	if maxBoost < 1 {
		maxBoost = 4
	}

	laneMap := make(map[string]*Lane, len(lanes))
	baseQ := make(map[string]int, len(lanes))
	ema := make(map[string]time.Duration, len(lanes))
	samples := make(map[string]uint64, len(lanes))

	for _, l := range lanes {
		laneMap[l.ID()] = l
		baseQ[l.ID()] = l.cfg.Quantum
		ema[l.ID()] = 0
		samples[l.ID()] = 0
	}

	return &LatencyTuner{
		lanes:        laneMap,
		baseQuantums: baseQ,
		emaLatency:   ema,
		sampleCount:  samples,
		alpha:        alpha,
		maxBoost:     maxBoost,
	}
}

// RecordLatency updates the exponential moving average wait time for a lane.
func (t *LatencyTuner) RecordLatency(laneID string, latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	prev, exists := t.emaLatency[laneID]
	if !exists {
		return
	}

	t.sampleCount[laneID]++
	if t.sampleCount[laneID] == 1 {
		t.emaLatency[laneID] = latency
		return
	}

	// EMA formula: EMA = alpha * latency + (1 - alpha) * prev
	newEMA := time.Duration(t.alpha*float64(latency) + (1.0-t.alpha)*float64(prev))
	t.emaLatency[laneID] = newEMA
}

// TuneQuantums evaluates measured latency against SLA target and dynamically updates lane quantums.
func (t *LatencyTuner) TuneQuantums(targetLatency time.Duration) map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()

	adjusted := make(map[string]int, len(t.lanes))

	for id, lane := range t.lanes {
		base := t.baseQuantums[id]
		measured := t.emaLatency[id]

		if targetLatency <= 0 || measured <= targetLatency {
			// Within SLA: keep base quantum
			lane.mu.Lock()
			lane.cfg.Quantum = base
			lane.mu.Unlock()
			adjusted[id] = base
			continue
		}

		// Above SLA: boost quantum proportionally
		ratio := float64(measured) / float64(targetLatency)
		boost := int(ratio)
		if boost > t.maxBoost {
			boost = t.maxBoost
		}
		if boost < 1 {
			boost = 1
		}

		newQ := base * boost
		lane.mu.Lock()
		lane.cfg.Quantum = newQ
		lane.mu.Unlock()
		adjusted[id] = newQ
	}

	return adjusted
}

// Metrics returns current telemetry metrics across all lanes.
func (t *LatencyTuner) Metrics() []LaneLatencyMetrics {
	t.mu.Lock()
	defer t.mu.Unlock()

	res := make([]LaneLatencyMetrics, 0, len(t.lanes))
	for id, lane := range t.lanes {
		lane.mu.RLock()
		currQ := lane.cfg.Quantum
		lane.mu.RUnlock()

		res = append(res, LaneLatencyMetrics{
			LaneID:         id,
			CurrentQuantum: currQ,
			BaseQuantum:    t.baseQuantums[id],
			EMALatency:     t.emaLatency[id],
			SampleCount:    t.sampleCount[id],
		})
	}
	return res
}
