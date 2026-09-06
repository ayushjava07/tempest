package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.ServerAddr != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.ServerAddr)
	}
	if cfg.StorageDriver != "memory" {
		t.Errorf("expected memory, got %s", cfg.StorageDriver)
	}
	if cfg.SchedulerWorkers != 4 {
		t.Errorf("expected 4, got %d", cfg.SchedulerWorkers)
	}
}

func TestEnvName(t *testing.T) {
	got := EnvName("server.addr")
	want := "TEMPEST_SERVER_ADDR"
	if got != want {
		t.Errorf("EnvName(server.addr) = %q, want %q", got, want)
	}
}

func TestFlagName(t *testing.T) {
	got := FlagName("server.addr")
	want := "server-addr"
	if got != want {
		t.Errorf("FlagName(server.addr) = %q, want %q", got, want)
	}
}

func TestLoad_FileOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"server.addr":":9999","storage.driver":"pgx"}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(BuildOptions{FilePath: cfgPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerAddr != ":9999" {
		t.Errorf("expected :9999, got %s", cfg.ServerAddr)
	}
	if cfg.StorageDriver != "pgx" {
		t.Errorf("expected pgx, got %s", cfg.StorageDriver)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	env := []string{"TEMPEST_SERVER_ADDR=:7777"}
	cfg, err := Load(BuildOptions{Env: env})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerAddr != ":7777" {
		t.Errorf("expected :7777, got %s", cfg.ServerAddr)
	}
}

func TestLoad_UnknownFileKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(cfgPath, []byte(`{"unknown.key":"val"}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(BuildOptions{FilePath: cfgPath})
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestLoad_UnknownEnvKey(t *testing.T) {
	env := []string{"TEMPEST_UNKNOWN_KEY=val"}
	_, err := Load(BuildOptions{Env: env})
	if err == nil {
		t.Error("expected error for unknown env key")
	}
}

func TestLoad_FlagOverride(t *testing.T) {
	cfg, err := Load(BuildOptions{Flags: []string{"--server-addr", ":5555"}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerAddr != ":5555" {
		t.Errorf("expected :5555, got %s", cfg.ServerAddr)
	}
}

func TestLoad_PrecedenceFlagOverEnv(t *testing.T) {
	env := []string{"TEMPEST_SERVER_ADDR=:3333"}
	cfg, err := Load(BuildOptions{Env: env, Flags: []string{"--server-addr", ":4444"}})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ServerAddr != ":4444" {
		t.Errorf("expected :4444 (flag wins), got %s", cfg.ServerAddr)
	}
}
