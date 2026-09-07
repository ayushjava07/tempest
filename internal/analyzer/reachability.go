package analyzer

import (
	"fmt"
	"sort"
)

const (
	RuleIDUnreachableStep   = "WF-001"
	RuleIDMissingDependency = "WF-002"
	RuleIDDeadEndStep       = "WF-003"
)

// UnreachableStepRule flags steps that cannot be executed from initial root steps.
type UnreachableStepRule struct{}

func (r UnreachableStepRule) ID() string { return RuleIDUnreachableStep }
func (r UnreachableStepRule) Description() string {
	return "Detects disconnected or unreachable steps in workflow DAG"
}
func (r UnreachableStepRule) Severity() Severity { return SeverityWarning }

func (r UnreachableStepRule) Analyze(ast *WorkflowAST) []Diagnostic {
	if len(ast.Steps) == 0 {
		return nil
	}

	// Adjacency graph: parent -> children
	children := make(map[string][]string)
	inDegree := make(map[string]int)

	for id := range ast.Steps {
		inDegree[id] = 0
	}

	for id, step := range ast.Steps {
		for _, dep := range step.DependsOn {
			if _, exists := ast.Steps[dep]; exists {
				children[dep] = append(children[dep], id)
				inDegree[id]++
			}
		}
	}

	// Roots are nodes with inDegree == 0
	visited := make(map[string]bool)
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
			visited[id] = true
		}
	}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, ch := range children[curr] {
			if !visited[ch] {
				visited[ch] = true
				queue = append(queue, ch)
			}
		}
	}

	var diags []Diagnostic
	for id := range ast.Steps {
		if !visited[id] {
			diags = append(diags, Diagnostic{
				RuleID:        r.ID(),
				Severity:      r.Severity(),
				StepID:        id,
				Message:       fmt.Sprintf("step %q is unreachable from any workflow entrypoint", id),
				FixSuggestion: "Check step dependencies or add connection to root execution path",
			})
		}
	}

	sort.Slice(diags, func(i, j int) bool { return diags[i].StepID < diags[j].StepID })
	return diags
}

// MissingDependencyRule flags references to nonexistent step IDs.
type MissingDependencyRule struct{}

func (r MissingDependencyRule) ID() string { return RuleIDMissingDependency }
func (r MissingDependencyRule) Description() string {
	return "Flags dependencies pointing to non-existent step IDs"
}
func (r MissingDependencyRule) Severity() Severity { return SeverityError }

func (r MissingDependencyRule) Analyze(ast *WorkflowAST) []Diagnostic {
	var diags []Diagnostic

	for id, step := range ast.Steps {
		for _, dep := range step.DependsOn {
			if _, exists := ast.Steps[dep]; !exists {
				diags = append(diags, Diagnostic{
					RuleID:        r.ID(),
					Severity:      r.Severity(),
					StepID:        id,
					Message:       fmt.Sprintf("step %q depends on non-existent parent step %q", id, dep),
					FixSuggestion: fmt.Sprintf("Define step %q or remove dependency reference", dep),
				})
			}
		}
	}

	sort.Slice(diags, func(i, j int) bool {
		if diags[i].StepID != diags[j].StepID {
			return diags[i].StepID < diags[j].StepID
		}
		return diags[i].Message < diags[j].Message
	})
	return diags
}
