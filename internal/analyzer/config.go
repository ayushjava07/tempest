package analyzer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// LintConfig configures static analyzer behaviors, severity overrides, and rule suppressions.
type LintConfig struct {
	DisabledRules     []string            `json:"disabled_rules,omitempty"`
	StepSuppressions  map[string][]string `json:"step_suppressions,omitempty"`
	SeverityOverrides map[string]Severity `json:"severity_overrides,omitempty"`
	MaxSteps          int                 `json:"max_steps,omitempty"`
}

// NewDefaultConfig returns a standard lint configuration.
func NewDefaultConfig() *LintConfig {
	return &LintConfig{
		DisabledRules:     make([]string, 0),
		StepSuppressions:  make(map[string][]string),
		SeverityOverrides: make(map[string]Severity),
		MaxSteps:          1000,
	}
}

// ParseConfig decodes a JSON-formatted linting configuration.
func ParseConfig(data []byte) (*LintConfig, error) {
	cfg := NewDefaultConfig()
	if len(data) == 0 {
		return cfg, nil
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse lint config: %w", err)
	}

	return cfg, nil
}

// IsSuppressed returns true if a given rule violation is muted globally or on a specific step.
func (c *LintConfig) IsSuppressed(ruleID, stepID string) bool {
	if c == nil {
		return false
	}

	for _, disabled := range c.DisabledRules {
		if strings.EqualFold(disabled, ruleID) || disabled == "*" {
			return true
		}
	}

	if stepID != "" && c.StepSuppressions != nil {
		if suppressedRules, ok := c.StepSuppressions[stepID]; ok {
			for _, r := range suppressedRules {
				if strings.EqualFold(r, ruleID) || r == "*" {
					return true
				}
			}
		}
	}

	return false
}

// EffectiveSeverity returns the overridden severity if specified in config, or the default.
func (c *LintConfig) EffectiveSeverity(ruleID string, defaultSev Severity) Severity {
	if c == nil || c.SeverityOverrides == nil {
		return defaultSev
	}
	if override, ok := c.SeverityOverrides[ruleID]; ok && override != "" {
		return override
	}
	return defaultSev
}

// FilterDiagnostics applies suppression rules and severity overrides to a slice of diagnostics.
func (c *LintConfig) FilterDiagnostics(diags []Diagnostic) []Diagnostic {
	if c == nil {
		return diags
	}

	filtered := make([]Diagnostic, 0, len(diags))
	for _, d := range diags {
		if c.IsSuppressed(d.RuleID, d.StepID) {
			continue
		}
		d.Severity = c.EffectiveSeverity(d.RuleID, d.Severity)
		filtered = append(filtered, d)
	}
	return filtered
}
