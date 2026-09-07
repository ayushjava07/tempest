package analyzer

import (
	"fmt"
	"sort"
	"strings"
)

const (
	RuleIDCyclicDependency = "WF-004"
	RuleIDUnboundedLoop    = "WF-005"
)

type color int

const (
	white color = iota // Unvisited
	gray               // Currently in active recursion stack
	black              // Fully explored
)

// CyclicDependencyRule detects cycles in workflow step dependencies.
type CyclicDependencyRule struct{}

func (r CyclicDependencyRule) ID() string { return RuleIDCyclicDependency }
func (r CyclicDependencyRule) Description() string {
	return "Detects circular dependencies and closed loops in workflow DAG"
}
func (r CyclicDependencyRule) Severity() Severity { return SeverityError }

func (r CyclicDependencyRule) Analyze(ast *WorkflowAST) []Diagnostic {
	var diags []Diagnostic
	colors := make(map[string]color)
	for id := range ast.Steps {
		colors[id] = white
	}

	// Adjacency: parent (depended upon) -> child (depends on parent)
	// Or directed dependency edge: step -> depends_on (prerequisite)
	// In DAG, step cannot execute until dep completes. Cycle means step -> dep1 -> dep2 -> step.
	var path []string
	detectedCycles := make(map[string]bool)

	var dfs func(u string)
	dfs = func(u string) {
		colors[u] = gray
		path = append(path, u)

		step, exists := ast.Steps[u]
		if exists {
			for _, v := range step.DependsOn {
				// Only traverse to existing steps
				if _, ok := ast.Steps[v]; !ok {
					continue
				}

				if v == u {
					// Self-loop
					cycleKey := fmt.Sprintf("%s->%s", u, u)
					if !detectedCycles[cycleKey] {
						detectedCycles[cycleKey] = true
						diags = append(diags, Diagnostic{
							RuleID:        r.ID(),
							Severity:      r.Severity(),
							StepID:        u,
							Message:       fmt.Sprintf("step %q has self-referential dependency on itself", u),
							FixSuggestion: "Remove self-dependency",
						})
					}
					continue
				}

				if colors[v] == gray {
					// Found back-edge! Reconstruct cycle path
					cycleStartIdx := -1
					for i, node := range path {
						if node == v {
							cycleStartIdx = i
							break
						}
					}
					if cycleStartIdx >= 0 {
						cycleSlice := append(path[cycleStartIdx:], v)
						cycleStr := strings.Join(cycleSlice, " -> ")
						if !detectedCycles[cycleStr] {
							detectedCycles[cycleStr] = true
							diags = append(diags, Diagnostic{
								RuleID:        r.ID(),
								Severity:      r.Severity(),
								StepID:        u,
								Message:       fmt.Sprintf("circular dependency detected: %s", cycleStr),
								FixSuggestion: "Break the cycle by removing or reversing one of the dependency edges",
							})
						}
					}
				} else if colors[v] == white {
					dfs(v)
				}
			}
		}

		path = path[:len(path)-1]
		colors[u] = black
	}

	// Iterate in deterministic order
	stepIDs := make([]string, 0, len(ast.Steps))
	for id := range ast.Steps {
		stepIDs = append(stepIDs, id)
	}
	sort.Strings(stepIDs)

	for _, id := range stepIDs {
		if colors[id] == white {
			dfs(id)
		}
	}

	return diags
}

// UnboundedLoopRule validates loop steps have finite upper bounds or termination guards.
type UnboundedLoopRule struct{}

func (r UnboundedLoopRule) ID() string { return RuleIDUnboundedLoop }
func (r UnboundedLoopRule) Description() string {
	return "Detects loop constructs without max iterations or termination predicates"
}
func (r UnboundedLoopRule) Severity() Severity { return SeverityWarning }

func (r UnboundedLoopRule) Analyze(ast *WorkflowAST) []Diagnostic {
	var diags []Diagnostic

	for id, step := range ast.Steps {
		if step.Type == "loop" || step.Type == "iterator" {
			maxIter, hasMax := step.Parameters["max_iterations"]
			_, hasUntil := step.Parameters["until"]

			if !hasMax && !hasUntil {
				diags = append(diags, Diagnostic{
					RuleID:        r.ID(),
					Severity:      r.Severity(),
					StepID:        id,
					Message:       fmt.Sprintf("loop step %q lacks both 'max_iterations' and 'until' termination criteria", id),
					FixSuggestion: "Define 'max_iterations' or 'until' condition to avoid infinite execution",
				})
			} else if hasMax && (maxIter == "0" || maxIter == "-1") {
				diags = append(diags, Diagnostic{
					RuleID:        r.ID(),
					Severity:      r.Severity(),
					StepID:        id,
					Message:       fmt.Sprintf("loop step %q has non-positive max_iterations=%s", id, maxIter),
					FixSuggestion: "Set max_iterations to a positive integer",
				})
			}
		}
	}

	sort.Slice(diags, func(i, j int) bool { return diags[i].StepID < diags[j].StepID })
	return diags
}
