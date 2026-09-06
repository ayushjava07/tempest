package errors

import (
	"fmt"
	"testing"
)

func FuzzClassify(f *testing.F) {
	f.Add("not found")
	f.Add("already exists")
	f.Add("conflict")
	f.Add("unauthenticated")
	f.Add("permission denied")
	f.Add("internal error")
	f.Add("unavailable")
	f.Add("deadline exceeded")
	f.Add("canceled")
	f.Add("unknown error")
	f.Add("")
	f.Fuzz(func(t *testing.T, msg string) {
		err := Of(ClassInternal, fmt.Errorf("%s", msg))
		class := Classify(err)
		if class == "" {
			t.Error("empty class")
		}
	})
}

func FuzzErrorf(f *testing.F) {
	f.Add("not_found", "resource %s missing")
	f.Add("internal", "something broke: %d")
	f.Fuzz(func(t *testing.T, class, format string) {
		err := Errorf(Class(class), format, "test")
		if err == nil {
			t.Fatal("nil error")
		}
		if err.Error() == "" {
			t.Error("empty error string")
		}
		_ = err.Unwrap()
	})
}
