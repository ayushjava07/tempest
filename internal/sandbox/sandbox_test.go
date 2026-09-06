package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestSandbox_EchoCommand(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	res, err := s.Execute(ctx, ProcessConfig{
		Command: "echo",
		Args:    []string{"hello", "tempest"},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if strings.TrimSpace(string(res.Stdout)) != "hello tempest" {
		t.Errorf("unexpected stdout: %q", string(res.Stdout))
	}
	if res.TimedOut {
		t.Error("should not have timed out")
	}
}

func TestSandbox_TimeoutKilling(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	// Run sleep for 5 seconds with a 100ms timeout
	res, _ := s.Execute(ctx, ProcessConfig{
		Command: "sleep",
		Args:    []string{"5"},
		Timeout: 100 * time.Millisecond,
	})

	if !res.TimedOut {
		t.Error("expected process to time out")
	}
	if res.ExitCode == 0 {
		t.Errorf("expected non-zero exit code on timeout, got %d", res.ExitCode)
	}
}

func TestSandbox_OutputTruncation(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	// Max 10 bytes output
	res, err := s.Execute(ctx, ProcessConfig{
		Command:        "echo",
		Args:           []string{"12345678901234567890"},
		MaxOutputBytes: 10,
		Timeout:        5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(res.Stdout) != 10 {
		t.Errorf("expected exactly 10 bytes of stdout, got %d", len(res.Stdout))
	}
	if !res.OutputTruncated {
		t.Error("expected OutputTruncated to be true")
	}
}

func TestSandbox_EnvironmentSanitization(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	res, err := s.Execute(ctx, ProcessConfig{
		Command: "sh",
		Args:    []string{"-c", "echo key=$SAFE_KEY secret=$TEMPEST_ADMIN_TOKEN"},
		Env: map[string]string{
			"SAFE_KEY":            "visible_val",
			"TEMPEST_ADMIN_TOKEN": "forbidden_token",
		},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := string(res.Stdout)
	if !strings.Contains(output, "key=visible_val") {
		t.Errorf("expected safe key in output: %s", output)
	}
	if strings.Contains(output, "forbidden_token") {
		t.Errorf("sensitive token leaked into sandbox environment: %s", output)
	}
}

func TestSandbox_NonZeroExitCode(t *testing.T) {
	s := NewSupervisor()
	ctx := context.Background()

	res, err := s.Execute(ctx, ProcessConfig{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
		Timeout: 5 * time.Second,
	})

	if res.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", res.ExitCode)
	}
	if err == nil {
		t.Error("expected error for non-zero exit code")
	}
}
