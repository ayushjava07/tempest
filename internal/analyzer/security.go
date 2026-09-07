package analyzer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	RuleIDShellInjection = "SEC-001"
)

var (
	// Matches direct pipe into shell interpreters
	pipeToShellRegex = regexp.MustCompile(`\|\s*(sh|bash|zsh|dash|ksh|python|perl|ruby)\b`)

	// Matches backtick or $() command substitution with dynamic variables
	subshellInterpolationRegex = regexp.MustCompile(`(\$\([^)]*\$\{inputs\.[^)]*\)|` + "`" + `[^` + "`" + `]*\$\{inputs\.[^` + "`" + `]*` + "`" + `)`)

	// Matches command concatenation with inputs e.g. ; ${inputs...} or && ${inputs...}
	concatInputRegex = regexp.MustCompile(`[;&|]{1,2}\s*\$\{inputs\.[a-zA-Z0-9_\-\.]+\}`)
)

// ShellInjectionRule scans task commands and arguments for unsafe shell interpolation or piping.
type ShellInjectionRule struct{}

func (r ShellInjectionRule) ID() string { return RuleIDShellInjection }
func (r ShellInjectionRule) Description() string {
	return "Detects high-risk shell injection patterns and unescaped variable interpolation in commands"
}
func (r ShellInjectionRule) Severity() Severity { return SeverityError }

func (r ShellInjectionRule) Analyze(ast *WorkflowAST) []Diagnostic {
	var diags []Diagnostic

	for id, step := range ast.Steps {
		r.scanText(id, "command", step.Command, &diags)

		for k, v := range step.Parameters {
			if strings.Contains(strings.ToLower(k), "cmd") ||
				strings.Contains(strings.ToLower(k), "command") ||
				strings.Contains(strings.ToLower(k), "script") ||
				strings.Contains(strings.ToLower(k), "exec") {
				r.scanText(id, fmt.Sprintf("parameter %q", k), v, &diags)
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

func (r ShellInjectionRule) scanText(stepID, field, text string, diags *[]Diagnostic) {
	if text == "" {
		return
	}

	if pipeToShellRegex.MatchString(text) {
		*diags = append(*diags, Diagnostic{
			RuleID:        r.ID(),
			Severity:      SeverityError,
			StepID:        stepID,
			Message:       fmt.Sprintf("step %q %s pipes directly into a dynamic shell interpreter", stepID, field),
			FixSuggestion: "Avoid piping unvalidated data into shell binaries; invoke structured task executors directly",
		})
	}

	if subshellInterpolationRegex.MatchString(text) {
		*diags = append(*diags, Diagnostic{
			RuleID:        r.ID(),
			Severity:      SeverityError,
			StepID:        stepID,
			Message:       fmt.Sprintf("step %q %s contains dynamic variable interpolation inside command substitution subshell", stepID, field),
			FixSuggestion: "Sanitize arguments or pass variables via explicit environment bindings rather than inline interpolation",
		})
	}

	if concatInputRegex.MatchString(text) {
		*diags = append(*diags, Diagnostic{
			RuleID:        r.ID(),
			Severity:      SeverityError,
			StepID:        stepID,
			Message:       fmt.Sprintf("step %q %s contains unescaped command chaining concatenated with input parameters", stepID, field),
			FixSuggestion: "Use separated argv array parameters instead of raw shell concatenation",
		})
	}
}
