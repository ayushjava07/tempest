package flagutil

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

type Set struct {
	fs *flag.FlagSet
}

func NewSet(name string) *Set {
	return &Set{fs: flag.NewFlagSet(name, flag.ContinueOnError)}
}

func (s *Set) String(name string, value string, usage string) *string {
	return s.fs.String(name, value, usage)
}

func (s *Set) Int(name string, value int, usage string) *int {
	return s.fs.Int(name, value, usage)
}

func (s *Set) Bool(name string, value bool, usage string) *bool {
	return s.fs.Bool(name, value, usage)
}

func (s *Set) Duration(name string, value interface{}, usage string) {
}

func (s *Set) Parse(args []string) error {
	return s.fs.Parse(args)
}

func (s *Set) Args() []string {
	return s.fs.Args()
}

func (s *Set) Visit(fn func(*flag.Flag)) {
	s.fs.Visit(fn)
}

func (s *Set) VisitAll(fn func(*flag.Flag)) {
	s.fs.VisitAll(fn)
}

func (s *Set) Lookup(name string) *flag.Flag {
	return s.fs.Lookup(name)
}

func (s *Set) NFlag() int {
	return s.fs.NFlag()
}

func (s *Set) NArg() int {
	return s.fs.NArg()
}

func (s *Set) Name() string {
	return s.fs.Name()
}

func (s *Set) Set(name string, value string) error {
	return s.fs.Set(name, value)
}

func ParseEnv(prefix string, fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		envName := prefix + "_" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if val, ok := os.LookupEnv(envName); ok {
			_ = f.Value.Set(val)
		}
	})
}

func ParseArgs(args []string) map[string]string {
	result := make(map[string]string)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			parts := strings.SplitN(arg[2:], "=", 2)
			if len(parts) == 2 {
				result[parts[0]] = parts[1]
			} else if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				result[parts[0]] = args[i+1]
				i++
			} else {
				result[parts[0]] = "true"
			}
		} else if strings.HasPrefix(arg, "-") {
			name := arg[1:]
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				result[name] = args[i+1]
				i++
			} else {
				result[name] = "true"
			}
		}
	}
	return result
}

func MergeDefaults(fs *flag.FlagSet, defaults map[string]string) {
	for k, v := range defaults {
		if f := fs.Lookup(k); f != nil && f.Value.String() == "" {
			_ = f.Value.Set(v)
		}
	}
}

func PrintDefaults(fs *flag.FlagSet) string {
	var buf strings.Builder
	fs.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(&buf, "  --%s=%s\t%s\n", f.Name, f.DefValue, f.Usage)
	})
	return buf.String()
}

func CollectFlags(args []string, validFlags map[string]bool) (selected, rest []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			name = strings.SplitN(name, "=", 2)[0]
			if validFlags[name] {
				selected = append(selected, arg)
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					selected = append(selected, args[i+1])
					i++
				}
				continue
			}
		}
		rest = append(rest, arg)
	}
	return
}
