package flagutil

import (
	"flag"
	"os"
	"testing"
)

func TestSet_Parse(t *testing.T) {
	s := NewSet("test")
	name := s.String("name", "default", "a name")
	if err := s.Parse([]string{"--name", "alice"}); err != nil {
		t.Fatal(err)
	}
	if *name != "alice" {
		t.Errorf("expected alice, got %s", *name)
	}
}

func TestSet_Int(t *testing.T) {
	s := NewSet("test")
	port := s.Int("port", 8080, "port number")
	if err := s.Parse([]string{"--port", "9090"}); err != nil {
		t.Fatal(err)
	}
	if *port != 9090 {
		t.Errorf("expected 9090, got %d", *port)
	}
}

func TestSet_Bool(t *testing.T) {
	s := NewSet("test")
	verbose := s.Bool("verbose", false, "verbose")
	if err := s.Parse([]string{"--verbose"}); err != nil {
		t.Fatal(err)
	}
	if !*verbose {
		t.Error("expected true")
	}
}

func TestParseArgs(t *testing.T) {
	args := []string{"--name", "alice", "-p", "80", "--debug"}
	result := ParseArgs(args)
	if result["name"] != "alice" {
		t.Error("expected name=alice")
	}
	if result["p"] != "80" {
		t.Error("expected p=80")
	}
	if result["debug"] != "true" {
		t.Error("expected debug=true")
	}
}

func TestParseArgs_Equals(t *testing.T) {
	args := []string{"--name=bob"}
	result := ParseArgs(args)
	if result["name"] != "bob" {
		t.Error("expected name=bob")
	}
}

func TestParseEnv(t *testing.T) {
	os.Setenv("TEST_NAME", "envval")
	defer os.Unsetenv("TEST_NAME")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("name", "", "name")
	ParseEnv("TEST", fs)
	if fs.Lookup("name").Value.String() != "envval" {
		t.Error("expected envval")
	}
}

func TestMergeDefaults(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("name", "", "name")
	MergeDefaults(fs, map[string]string{"name": "fallback"})
	if fs.Lookup("name").Value.String() != "fallback" {
		t.Error("expected fallback")
	}
}

func TestCollectFlags(t *testing.T) {
	valid := map[string]bool{"verbose": true, "port": true}
	selected, rest := CollectFlags([]string{"--verbose", "--port", "80", "file.txt", "other.txt"}, valid)
	if len(selected) != 3 {
		t.Errorf("expected 3 selected, got %d: %v", len(selected), selected)
	}
	if len(rest) != 2 {
		t.Errorf("expected 2 rest, got %d: %v", len(rest), rest)
	}
}

func TestPrintDefaults(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("name", "default", "a name")
	output := PrintDefaults(fs)
	if output == "" {
		t.Error("expected non-empty")
	}
}

func TestSet_NFlag_NArg(t *testing.T) {
	s := NewSet("test")
	s.Int("x", 0, "")
	s.Parse([]string{"--x", "1", "extra"})
	if s.NFlag() != 1 {
		t.Errorf("expected 1 flag, got %d", s.NFlag())
	}
	if s.NArg() != 1 {
		t.Errorf("expected 1 arg, got %d", s.NArg())
	}
}
