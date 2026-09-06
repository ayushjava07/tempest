package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/tempest-io/tempest/internal/plugin"
	"github.com/tempest-io/tempest/pkg/errors"
)

const (
	PassHandlerName = "pass"
	EchoHandlerName = "echo"
	ShellHandlerName = "shell"
	HTTPHandlerName = "http"
	FailHandlerName = "fail"
)

type PassHandler struct{}

func (h *PassHandler) Name() string { return PassHandlerName }
func (h *PassHandler) Description() string {
	return "complete immediately with an empty result; useful for gateways"
}
func (h *PassHandler) Execute(ctx context.Context, in plugin.Handle) (plugin.Result, error) {
	if delayMs, ok := in.RunInput["delay_ms"].(float64); ok {
		delay := time.Duration(min64(delayMs, 30_000)) * time.Millisecond
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return plugin.Result{}, ctx.Err()
		}
	}
	if in.RunInput["fail"] == true {
		return plugin.Errorf("pass handler directed to fail"), nil
	}
	return plugin.Result{Output: map[string]any{"ok": true}}, nil
}

type EchoHandler struct{}

func (h *EchoHandler) Name() string { return EchoHandlerName }
func (h *EchoHandler) Description() string {
	return "echo the input payload as output"
}
func (h *EchoHandler) Execute(ctx context.Context, in plugin.Handle) (plugin.Result, error) {
	out := make(map[string]any, len(in.RunInput)+2)
	for k, v := range in.RunInput {
		out[k] = v
	}
	out["run_id"] = in.RunID
	out["step_id"] = in.StepID
	return plugin.Result{Output: out}, nil
}

type ShellHandler struct {
	WorkDir string
	Env     []string
}

func (h *ShellHandler) Name() string { return ShellHandlerName }
func (h *ShellHandler) Description() string {
	return "run a shell command; the 'command' input key is required"
}
func (h *ShellHandler) Execute(ctx context.Context, in plugin.Handle) (plugin.Result, error) {
	cmd, ok := in.RunInput["command"].(string)
	if !ok || strings.TrimSpace(cmd) == "" {
		return plugin.Errorf("shell handler requires a non-empty 'command' input string"), nil
	}
	if err := ctx.Err(); err != nil {
		return plugin.Result{}, err
	}
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = h.WorkDir
	c.Env = append(c.Env, h.Env...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		msg := fmt.Sprintf("command failed: %v", err)
		if stderr.Len() > 0 {
			msg += ": " + strings.TrimSpace(stderr.String())
		}
		return plugin.Result{Error: errors.Of(errors.ClassInternal, fmt.Errorf("%s", msg))}, nil
	}
	return plugin.Result{Output: map[string]any{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
		"exit":   "0",
	}}, nil
}

type HTTPHandler struct {
	Client *http.Client
}

func (h *HTTPHandler) Name() string { return HTTPHandlerName }
func (h *HTTPHandler) Description() string {
	return "POST the step input payload to a URL from the 'url' input key"
}
func (h *HTTPHandler) Execute(ctx context.Context, in plugin.Handle) (plugin.Result, error) {
	url, ok := in.RunInput["url"].(string)
	if !ok || (!strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://")) {
		return plugin.Errorf("http handler requires a valid 'url' input"), nil
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	payload, _ := json.Marshal(in.RunInput)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return plugin.Result{Error: err}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tempest/step")
	resp, err := client.Do(req)
	if err != nil {
		return plugin.Result{Error: errors.Of(errors.ClassUnavailable, err)}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return plugin.Result{Error: errors.Of(errors.ClassInvalidArgument,
			fmt.Errorf("http handler got status %d", resp.StatusCode))}, nil
	}
	return plugin.Result{Output: map[string]any{"status": resp.StatusCode}}, nil
}

type FailHandler struct{}

func (h *FailHandler) Name() string { return FailHandlerName }
func (h *FailHandler) Description() string {
	return "always fail with a deterministic error message"
}
func (h *FailHandler) Execute(ctx context.Context, in plugin.Handle) (plugin.Result, error) {
	msg := "fail handler triggered"
	if m, ok := in.RunInput["message"].(string); ok && m != "" {
		msg = m
	}
	return plugin.Result{Error: errors.Of(errors.ClassInternal, fmt.Errorf("%s", msg))}, nil
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
