package promexporter

import (
	"sync"
)

// WorkflowMetrics manages standardized Prometheus instruments for the Tempest workflow runtime.
type WorkflowMetrics struct {
	mu           sync.RWMutex
	registry     *CollectorRegistry
	activeRuns   map[string]*Gauge
	execTotals   map[string]*Counter
	stepTotals   map[string]*Counter
	stepDuration map[string]*Histogram
	queueGauges  map[string]*Gauge
}

// NewWorkflowMetrics initializes and registers workflow runtime metric collectors.
func NewWorkflowMetrics(registry *CollectorRegistry) *WorkflowMetrics {
	if registry == nil {
		registry = NewRegistry()
	}
	return &WorkflowMetrics{
		registry:     registry,
		activeRuns:   make(map[string]*Gauge),
		execTotals:   make(map[string]*Counter),
		stepTotals:   make(map[string]*Counter),
		stepDuration: make(map[string]*Histogram),
		queueGauges:  make(map[string]*Gauge),
	}
}

// Registry returns the underlying collector registry.
func (w *WorkflowMetrics) Registry() *CollectorRegistry {
	return w.registry
}

// RecordWorkflowStart increments active workflow run gauge and execution total.
func (w *WorkflowMetrics) RecordWorkflowStart(workflowID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	g, exists := w.activeRuns[workflowID]
	if !exists {
		g = NewGauge("tempest_workflow_active_runs", "Number of currently executing workflow runs", map[string]string{"workflow_id": workflowID})
		w.registry.RegisterGauge(g)
		w.activeRuns[workflowID] = g
	}
	g.Inc()
}

// RecordWorkflowComplete decrements active runs and records execution outcome counter.
func (w *WorkflowMetrics) RecordWorkflowComplete(workflowID string, success bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if g, exists := w.activeRuns[workflowID]; exists {
		g.Dec()
	}

	status := "success"
	if !success {
		status = "failure"
	}

	key := workflowID + ":" + status
	c, exists := w.execTotals[key]
	if !exists {
		c = NewCounter("tempest_workflow_executions_total", "Total completed workflow runs by status", map[string]string{
			"workflow_id": workflowID,
			"status":      status,
		})
		w.registry.RegisterCounter(c)
		w.execTotals[key] = c
	}
	c.Inc()
}

// RecordStepExecution updates step execution counts and duration distribution.
func (w *WorkflowMetrics) RecordStepExecution(stepType string, durationSec float64, success bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	status := "success"
	if !success {
		status = "failure"
	}

	key := stepType + ":" + status
	c, exists := w.stepTotals[key]
	if !exists {
		c = NewCounter("tempest_step_executions_total", "Total executed steps by type and status", map[string]string{
			"step_type": stepType,
			"status":    status,
		})
		w.registry.RegisterCounter(c)
		w.stepTotals[key] = c
	}
	c.Inc()

	h, exists := w.stepDuration[stepType]
	if !exists {
		h = NewHistogram("tempest_step_duration_seconds", "Step execution duration in seconds", map[string]string{"step_type": stepType}, nil)
		w.registry.RegisterHistogram(h)
		w.stepDuration[stepType] = h
	}
	h.Observe(durationSec)
}

// UpdateQueueDepth sets the current task depth for a scheduler lane.
func (w *WorkflowMetrics) UpdateQueueDepth(laneID string, depth float64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	g, exists := w.queueGauges[laneID]
	if !exists {
		g = NewGauge("tempest_queue_depth", "Current task depth in priority scheduling lane", map[string]string{"lane_id": laneID})
		w.registry.RegisterGauge(g)
		w.queueGauges[laneID] = g
	}
	g.Set(depth)
}
