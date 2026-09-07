package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLICompletion(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectSub   string
		expectError bool
	}{
		{
			name:        "bash completion generation",
			args:        []string{"completion", "bash"},
			expectSub:   "_tempest_completion()",
			expectError: false,
		},
		{
			name:        "zsh completion generation",
			args:        []string{"completion", "zsh"},
			expectSub:   "_tempest()",
			expectError: false,
		},
		{
			name:        "missing shell argument",
			args:        []string{"completion"},
			expectError: true,
		},
		{
			name:        "unsupported shell",
			args:        []string{"completion", "fish"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			err := runWith(tt.args, stdout, stderr)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for args %v, got nil", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for args %v: %v", tt.args, err)
			}
			if !strings.Contains(stdout.String(), tt.expectSub) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.expectSub, stdout.String())
			}
		})
	}
}
