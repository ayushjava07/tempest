package analyzer

import (
	"fmt"
	"sort"
	"sync"
)

// Severity indicates the criticalness of an analyzer diagnostic.
type Severity string

const (
	SeverityInfo    Severity = "INFO"
	SeverityWarning Severity = "WARNING"
	SeverityError   Severity = "ERROR"
)

// Diagnostic details a rule violation or lint warning found during AST inspection.
type Diagnostic struct {
	RuleID        string   `json:"rule_id"`
	Severity      Severity `json:"severity"`
	StepID        string   `json:"step_id,omitempty"`
	Message       string   `json:"message"`
	Line          int      `json:"line,omitempty"`
	FixSuggestion string   `json:"fix_suggestion,omitempty"`
}

func (d Diagnostic) String() string {
	if d.StepID != "" {
		return fmt.Sprintf("[%s] %s (step: %s): %s", d.Severity, d.RuleID, d.StepID, d.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", d.Severity, d.RuleID, d.Message)
}

// StepNode represents an abstract syntax node for an individual workflow step.
type StepNode struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	DependsOn  []string          `json:"depends_on,omitempty"`
	Parameters map[string]string `json:"parameters,omitempty"`
	Command    string            `json:"command,omitempty"`
	Timeout    string            `json:"timeout,omitempty"`
	Retries    int               `json:"retries,omitempty"`
}

// WorkflowAST represents the top-level abstract syntax tree of a workflow definition.
type WorkflowAST struct {
	WorkflowID string              `json:"workflow_id"`
	Version    string              `json:"version"`
	Steps      map[string]StepNode `json:"steps"`
	Parameters map[string]string   `json:"parameters,omitempty"`
}

// Rule defines the interface for an AST inspection validator.
type Rule interface {
	ID() string
	Description() string
	Severity() Severity
	Analyze(ast *WorkflowAST) []Diagnostic
}

// Analyzer orchestrates static inspection rules across a workflow definition.
type Analyzer struct {
	mu    sync.RWMutex
	rules map[string]Rule
}

// New creates an analyzer instance.
func New() *Analyzer {
	return &Analyzer{
		rules: make(map[string]Rule),
	}
}

// Register registers a static inspection rule.
func (a *Analyzer) Register(rule Rule) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rules[rule.ID()] = rule
}

// Rules returns a slice of all registered rules sorted by ID.
func (a *Analyzer) Rules() []Rule {
	a.mu.RLock()
	defer a.mu.RUnlock()

	res := make([]Rule, 0, len(a.rules))
	for _, r := range a.rules {
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID() < res[j].ID() })
	return res
}

// Analyze runs all registered rules against the AST and returns sorted diagnostics.
func (a *Analyzer) Analyze(ast *WorkflowAST) []Diagnostic {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var diagnostics []Diagnostic
	for _, rule := range a.rules {
		diags := rule.Analyze(ast)
		diagnostics = append(diagnostics, diags...)
	}

	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Severity != diagnostics[j].Severity {
			// ERROR > WARNING > INFO
			return severityWeight(diagnostics[i].Severity) > severityWeight(diagnostics[j].Severity)
		}
		if diagnostics[i].StepID != diagnostics[j].StepID {
			return diagnostics[i].StepID < diagnostics[j].StepID
		}
		return diagnostics[i].RuleID < diagnostics[j].RuleID
	})

	return diagnostics
}

// HasErrors returns true if any diagnostic has SeverityError.
func HasErrors(diags []Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

func severityWeight(s Severity) int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}
