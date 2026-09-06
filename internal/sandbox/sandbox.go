package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrExecutionTimeout = errors.New("sandbox: execution timeout exceeded")
	ErrOutputExceeded   = errors.New("sandbox: output buffer exceeded limit")
	ErrEmptyCommand     = errors.New("sandbox: command cannot be empty")
)

// ProcessConfig specifies execution parameters and resource constraints.
type ProcessConfig struct {
	Command        string
	Args           []string
	Dir            string
	Env            map[string]string
	InheritHostEnv bool
	Timeout        time.Duration
	MaxOutputBytes int64
	Stdin          io.Reader
}

// ProcessResult captures the output, metrics, and exit status of a sandboxed execution.
type ProcessResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	Duration        time.Duration
	TimedOut        bool
	OutputTruncated bool
	Error           error
}

// Supervisor executes processes within bounded resource envelopes.
type Supervisor struct {
	defaultTimeout   time.Duration
	defaultMaxOutput int64
	blockedEnvKeys   map[string]bool
}

func NewSupervisor() *Supervisor {
	blocked := map[string]bool{
		"DATABASE_URL":           true,
		"AWS_SECRET_ACCESS_KEY":  true,
		"AWS_SESSION_TOKEN":      true,
		"GITHUB_TOKEN":           true,
		"TEMPEST_ADMIN_TOKEN":    true,
		"TEMPEST_ENCRYPTION_KEY": true,
	}
	return &Supervisor{
		defaultTimeout:   30 * time.Second,
		defaultMaxOutput: 10 * 1024 * 1024, // 10MB default
		blockedEnvKeys:   blocked,
	}
}

// Execute spawns and monitors the sandboxed process until completion or timeout.
func (s *Supervisor) Execute(ctx context.Context, cfg ProcessConfig) (ProcessResult, error) {
	if cfg.Command == "" {
		return ProcessResult{}, ErrEmptyCommand
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = s.defaultTimeout
	}

	maxOutput := cfg.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = s.defaultMaxOutput
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	// Prepare sanitized environment
	cmd.Env = s.sanitizeEnv(cfg.Env, cfg.InheritHostEnv)

	// Bounded output buffers
	stdoutLimiter := newBoundedBuffer(maxOutput)
	stderrLimiter := newBoundedBuffer(maxOutput)
	cmd.Stdout = stdoutLimiter
	cmd.Stderr = stderrLimiter
	if cfg.Stdin != nil {
		cmd.Stdin = cfg.Stdin
	}

	// Ensure process group isolation on Unix systems
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	start := time.Now()
	err := cmd.Start()
	if err != nil {
		return ProcessResult{
			ExitCode: -1,
			Duration: time.Since(start),
			Error:    err,
		}, err
	}

	// Wait for process with process group termination on timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var waitErr error
	var timedOut bool

	select {
	case <-ctx.Done():
		timedOut = true
		waitErr = ErrExecutionTimeout
		// Kill entire process group
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		<-done // Wait for goroutine to finish
	case waitErr = <-done:
	}

	duration := time.Since(start)
	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	res := ProcessResult{
		ExitCode:        exitCode,
		Stdout:          stdoutLimiter.Bytes(),
		Stderr:          stderrLimiter.Bytes(),
		Duration:        duration,
		TimedOut:        timedOut,
		OutputTruncated: stdoutLimiter.Truncated() || stderrLimiter.Truncated(),
		Error:           waitErr,
	}

	return res, waitErr
}

func (s *Supervisor) sanitizeEnv(customEnv map[string]string, inheritHost bool) []string {
	var env []string
	if inheritHost {
		// Only inherit non-sensitive host variables
		for _, e := range exec.Command("").Environ() {
			pair := strings.SplitN(e, "=", 2)
			if len(pair) == 2 && !s.blockedEnvKeys[strings.ToUpper(pair[0])] {
				env = append(env, e)
			}
		}
	}

	for k, v := range customEnv {
		if !s.blockedEnvKeys[strings.ToUpper(k)] {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return env
}

// boundedBuffer captures output up to maxBytes, discarding the rest safely.
type boundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	maxBytes  int64
	truncated bool
}

func newBoundedBuffer(maxBytes int64) *boundedBuffer {
	return &boundedBuffer{
		maxBytes: maxBytes,
	}
}

func (b *boundedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	currentLen := int64(b.buf.Len())
	if currentLen >= b.maxBytes {
		b.truncated = true
		return len(p), nil
	}

	remaining := b.maxBytes - currentLen
	if int64(len(p)) > remaining {
		b.truncated = true
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}

	return b.buf.Write(p)
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, b.buf.Len())
	copy(out, b.buf.Bytes())
	return out
}

func (b *boundedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
