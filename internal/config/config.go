package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

const EnvPrefix = "TEMPEST_"

type Config struct {
	ServerAddr          string `json:"server.addr"`
	ServerGRPCAddr      string `json:"server.grpc_addr"`
	ServerInsecure      bool   `json:"server.insecure"`
	ServerNamespace     string `json:"server.namespace"`
	StorageDriver       string `json:"storage.driver"`
	StorageURL          string `json:"storage.url"`
	SchedulerWorkers    int    `json:"scheduler.workers"`
	SchedulerMaxDequeue int    `json:"scheduler.max_dequeue"`
	LogLevel            string `json:"log.level"`
	LogFormat           string `json:"log.format"`
	DashboardEnabled    bool   `json:"dashboard.enabled"`
	DashboardAddr       string `json:"dashboard.addr"`
}

func Default() Config {
	return Config{
		ServerAddr:          ":8080",
		ServerGRPCAddr:      ":9090",
		ServerInsecure:      true,
		ServerNamespace:     "default",
		StorageDriver:       "memory",
		StorageURL:          "",
		SchedulerWorkers:    4,
		SchedulerMaxDequeue: 10,
		LogLevel:            "info",
		LogFormat:           "text",
		DashboardEnabled:    false,
		DashboardAddr:       ":8088",
	}
}

type fieldRef struct {
	idx  int
	kind reflect.Kind
}

var refs map[string]fieldRef

func init() {
	refs = make(map[string]fieldRef)
	t := reflect.TypeOf(Config{})
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		refs[f.Tag.Get("json")] = fieldRef{idx: i, kind: f.Type.Kind()}
	}
}

func EnvName(key string) string {
	return EnvPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

func FlagName(key string) string {
	return strings.ReplaceAll(key, ".", "-")
}

type BuildOptions struct {
	FilePath string
	Env      []string
	Flags    []string
}

func Load(opts BuildOptions) (Config, error) {
	cfg := Default()
	if opts.FilePath != "" {
		if err := applyFile(&cfg, opts.FilePath); err != nil {
			return Config{}, err
		}
	}
	if opts.Env != nil {
		if err := applyEnv(&cfg, opts.Env); err != nil {
			return Config{}, err
		}
	}
	if opts.Flags != nil && len(opts.Flags) > 0 {
		if err := applyFlags(&cfg, opts.Flags); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func applyFile(cfg *Config, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var present map[string]json.RawMessage
	if err := json.Unmarshal(raw, &present); err != nil {
		return fmt.Errorf("parse config file %s: %w", path, err)
	}
	for key := range present {
		if _, ok := refs[key]; !ok {
			return fmt.Errorf("config file %s: unknown key %q", path, key)
		}
	}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	return nil
}

func applyEnv(cfg *Config, environ []string) error {
	known := make(map[string]bool, len(refs))
	for key := range refs {
		known[EnvName(key)] = true
	}
	for _, kv := range environ {
		if !strings.HasPrefix(kv, EnvPrefix) {
			continue
		}
		name, _, _ := strings.Cut(kv, "=")
		if !known[name] {
			return fmt.Errorf("environment: unknown %q", name)
		}
	}
	lookup := environMap(environ)
	for key, f := range refs {
		if v, ok := lookup[EnvName(key)]; ok {
			if err := setField(cfg, f, v); err != nil {
				return fmt.Errorf("environment %s: %w", EnvName(key), err)
			}
		}
	}
	return nil
}

func applyFlags(cfg *Config, args []string) error {
	fs := NewFlagSet()
	if err := fs.Parse(args); err != nil {
		return err
	}
	set := map[string]string{}
	fs.Visit(func(f *flag.Flag) {
		set[f.Name] = f.Value.String()
	})
	for key, f := range refs {
		if v, ok := set[FlagName(key)]; ok {
			if err := setField(cfg, f, v); err != nil {
				return fmt.Errorf("flag --%s: %w", FlagName(key), err)
			}
		}
	}
	return nil
}

func NewFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("tempest", flag.ContinueOnError)
	zeros := Config{}
	for key, f := range refs {
		name := FlagName(key)
		cur := reflect.ValueOf(&zeros).Elem().Field(f.idx)
		switch f.kind {
		case reflect.String:
			fs.StringVar(cur.Addr().Interface().(*string), name, cur.String(), "")
		case reflect.Bool:
			fs.BoolVar(cur.Addr().Interface().(*bool), name, cur.Bool(), "")
		case reflect.Int:
			fs.IntVar(cur.Addr().Interface().(*int), name, int(cur.Int()), "")
		}
	}
	return fs
}

func setField(cfg *Config, f fieldRef, value string) error {
	dst := reflect.ValueOf(cfg).Elem().Field(f.idx)
	switch f.kind {
	case reflect.String:
		dst.SetString(value)
		return nil
	case reflect.Bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean %q", value)
		}
		dst.SetBool(v)
		return nil
	case reflect.Int:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer %q", value)
		}
		dst.SetInt(v)
		return nil
	default:
		return fmt.Errorf("unsupported kind %v", f.kind)
	}
}

func environMap(environ []string) map[string]string {
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}
