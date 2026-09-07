package analyzer

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	RuleIDMissingParameterRef = "WF-006"
	RuleIDResourceQuota       = "WF-007"
)

var (
	inputParamRegex = regexp.MustCompile(`\$\{\s*inputs\.([a-zA-Z0-9_\-\.]+)\s*\}`)
	stepOutputRegex = regexp.MustCompile(`\$\{\s*steps\.([a-zA-Z0-9_\-]+)\.([a-zA-Z0-9_\-\.]+)\s*\}`)
)

// MissingParameterRule scans step expressions for references to undeclared workflow inputs or outputs.
type MissingParameterRule struct{}

func (r MissingParameterRule) ID() string { return RuleIDMissingParameterRef }
func (r MissingParameterRule) Description() string {
	return "Validates that variable references point to defined workflow inputs or upstream step outputs"
}
func (r MissingParameterRule) Severity() Severity { return SeverityError }

func (r MissingParameterRule) Analyze(ast *WorkflowAST) []Diagnostic {
	var diags []Diagnostic

	for id, step := range ast.Steps {
		// Scan step Command
		r.scanString(id, "command", step.Command, ast, &diags)

		// Scan step Parameters
		for pKey, pVal := range step.Parameters {
			r.scanString(id, fmt.Sprintf("param.%s", pKey), pVal, ast, &diags)
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

func (r MissingParameterRule) scanString(stepID, field, val string, ast *WorkflowAST, diags *[]Diagnostic) {
	if val == "" {
		return
	}

	// 1. Check ${inputs.var}
	inputMatches := inputParamRegex.FindAllStringSubmatch(val, -1)
	for _, m := range inputMatches {
		if len(m) > 1 {
			paramName := m[1]
			if _, exists := ast.Parameters[paramName]; !exists {
				*diags = append(*diags, Diagnostic{
					RuleID:        r.ID(),
					Severity:      r.Severity(),
					StepID:        stepID,
					Message:       fmt.Sprintf("step %q %s references undefined workflow input %q", stepID, field, paramName),
					FixSuggestion: fmt.Sprintf("Declare %q in workflow parameters", paramName),
				})
			}
		}
	}

	// 2. Check ${steps.stepName.output}
	stepMatches := stepOutputRegex.FindAllStringSubmatch(val, -1)
	for _, m := range stepMatches {
		if len(m) > 2 {
			targetStep := m[1]
			if _, exists := ast.Steps[targetStep]; !exists {
				*diags = append(*diags, Diagnostic{
					RuleID:        r.ID(),
					Severity:      r.Severity(),
					StepID:        stepID,
					Message:       fmt.Sprintf("step %q %s references output from non-existent step %q", stepID, field, targetStep),
					FixSuggestion: fmt.Sprintf("Ensure step %q is defined before referencing its outputs", targetStep),
				})
			}
		}
	}
}

// ResourceQuotaRule checks timeouts, retry limits, and step count limits.
type ResourceQuotaRule struct {
	MaxStepCount int
	MaxRetries   int
}

func NewDefaultResourceQuotaRule() ResourceQuotaRule {
	return ResourceQuotaRule{
		MaxStepCount: 1000,
		MaxRetries:   20,
	}
}

func (r ResourceQuotaRule) ID() string { return RuleIDResourceQuota }
func (r ResourceQuotaRule) Description() string {
	return "Enforces resource limits, retry boundaries, and timeout validity"
}
func (r ResourceQuotaRule) Severity() Severity { return SeverityWarning }

func (r ResourceQuotaRule) Analyze(ast *WorkflowAST) []Diagnostic {
	var diags []Diagnostic

	maxSteps := r.MaxStepCount
	if maxSteps <= 0 {
		maxSteps = 1000
	}

	if len(ast.Steps) > maxSteps {
		diags = append(diags, Diagnostic{
			RuleID:        r.ID(),
			Severity:      SeverityError,
			Message:       fmt.Sprintf("workflow exceeds maximum step count limit (%d > %d)", len(ast.Steps), maxSteps),
			FixSuggestion: "Partition workflow into smaller sub-workflows",
		})
	}

	maxRetries := r.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 20
	}

	for id, step := range ast.Steps {
		// Validate timeout
		if step.Timeout != "" {
			if _, err := time.ParseDuration(step.Timeout); err != nil {
				// Also try parsing integer seconds
				if sec, numErr := strconv.Atoi(strings.TrimSpace(step.Timeout)); numErr != nil || sec <= 0 {
					diags = append(diags, Diagnostic{
						RuleID:        r.ID(),
						Severity:      SeverityError,
						StepID:        id,
						Message:       fmt.Sprintf("step %q has invalid timeout %q (must be valid Go duration e.g. '30s' or positive seconds)", id, step.Timeout),
						FixSuggestion: "Use valid duration format such as '10s', '5m', '1h'",
					})
				}
			}
		}

		// Validate retries
		if step.Retries < 0 {
			diags = append(diags, Diagnostic{
				RuleID:        r.ID(),
				Severity:      SeverityError,
				StepID:        id,
				Message:       fmt.Sprintf("step %q has negative retry count %d", id, step.Retries),
				FixSuggestion: "Set retries >= 0",
			})
		} else if step.Retries > maxRetries {
			diags = append(diags, Diagnostic{
				RuleID:        r.ID(),
				Severity:      SeverityWarning,
				StepID:        id,
				Message:       fmt.Sprintf("step %q has excessive retry count %d (max recommended: %d)", id, step.Retries, maxRetries),
				FixSuggestion: fmt.Sprintf("Lower retry count below %d to avoid prolonged task blocking", maxRetries),
			})
		}
	}

	sort.Slice(diags, func(i, j int) bool { return diags[i].StepID < diags[j].StepID })
	return diags
}
