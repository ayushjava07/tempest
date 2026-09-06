package validation

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	maxWorkflowNameLen = 64
	maxStepIDLen       = 64
	maxStepTimeoutMS   = 24 * time.Hour
)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9\-\.]{0,63}$`)

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name must not be empty")
	}
	if len(name) > maxWorkflowNameLen {
		return fmt.Errorf("name %q exceeds %d chars", name, maxWorkflowNameLen)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("name %q must match %s", name, nameRe.String())
	}
	return nil
}

func ValidateNamespace(ns string) error {
	if strings.TrimSpace(ns) == "" {
		return fmt.Errorf("namespace must not be empty")
	}
	return ValidateName(ns)
}

func ValidateVersion(v int) error {
	if v < 0 {
		return fmt.Errorf("version must be non-negative, got %d", v)
	}
	return nil
}

func ValidateStepID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("step id must not be empty")
	}
	if len(id) > maxStepIDLen {
		return fmt.Errorf("step id %q exceeds %d chars", id, maxStepIDLen)
	}
	if !nameRe.MatchString(id) {
		return fmt.Errorf("step id %q must match %s", id, nameRe.String())
	}
	return nil
}

type RetryPolicy struct {
	MaxAttempts     int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxElapsed      time.Duration
}

func ValidateRetryPolicy(r *RetryPolicy) error {
	if r == nil {
		return nil
	}
	if r.MaxAttempts < 0 {
		return fmt.Errorf("max_attempts must be non-negative")
	}
	if r.InitialInterval < 0 {
		return fmt.Errorf("initial_interval must be non-negative")
	}
	if r.MaxInterval < 0 {
		return fmt.Errorf("max_interval must be non-negative")
	}
	if r.Multiplier < 1.0 && r.Multiplier != 0 {
		return fmt.Errorf("multiplier must be >= 1.0 or 0")
	}
	return nil
}

type StepDef struct {
	ID        string
	Handler   string
	DependsOn []string
}

func ValidateDefinitionSteps(steps []StepDef) error {
	if len(steps) == 0 {
		return fmt.Errorf("definition must have at least one step")
	}
	ids := make(map[string]bool, len(steps))
	for _, s := range steps {
		if err := ValidateStepID(s.ID); err != nil {
			return err
		}
		if ids[s.ID] {
			return fmt.Errorf("duplicate step id %q", s.ID)
		}
		ids[s.ID] = true
		if s.Handler == "" {
			return fmt.Errorf("step %q requires a handler", s.ID)
		}
		for _, dep := range s.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("step %q depends on unknown step %q (must be declared first)", s.ID, dep)
			}
		}
	}
	return nil
}
