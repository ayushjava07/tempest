package versioning

import (
	"testing"
)

func TestVersion_Parse(t *testing.T) {
	v, err := Parse("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if v.Major != 1 || v.Minor != 2 || v.Patch != 3 {
		t.Errorf("expected 1.2.3, got %s", v.String())
	}
}

func TestVersion_ParseWithPre(t *testing.T) {
	v, err := Parse("1.2.3-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if v.Pre != "alpha" {
		t.Errorf("expected alpha, got %s", v.Pre)
	}
}

func TestVersion_ParseWithV(t *testing.T) {
	v, err := Parse("v2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if v.Major != 2 {
		t.Errorf("expected 2, got %d", v.Major)
	}
}

func TestVersion_Compare(t *testing.T) {
	v1, _ := Parse("1.2.3")
	v2, _ := Parse("1.2.4")
	if v1.Compare(v2) != -1 {
		t.Error("expected v1 < v2")
	}
	if v2.Compare(v1) != 1 {
		t.Error("expected v2 > v1")
	}
	v3, _ := Parse("1.2.3")
	if v1.Compare(v3) != 0 {
		t.Error("expected equal")
	}
}

func TestVersion_Satisfies(t *testing.T) {
	v, _ := Parse("1.2.3")
	if !v.Satisfies("1.2.3") {
		t.Error("expected exact match")
	}
	if !v.Satisfies(">=1.0.0") {
		t.Error("expected >= match")
	}
	if !v.Satisfies("<=2.0.0") {
		t.Error("expected <= match")
	}
	if !v.Satisfies(">1.0.0") {
		t.Error("expected > match")
	}
	if !v.Satisfies("<2.0.0") {
		t.Error("expected < match")
	}
	if !v.Satisfies("~1.2.0") {
		t.Error("expected ~ match")
	}
	if !v.Satisfies("^1.0.0") {
		t.Error("expected ^ match")
	}
	if v.Satisfies("2.0.0") {
		t.Error("expected no match")
	}
}

func TestVersion_Increment(t *testing.T) {
	v, _ := Parse("1.2.3")
	if IncrementMajor(v).String() != "2.0.0" {
		t.Error("expected 2.0.0")
	}
	if IncrementMinor(v).String() != "1.3.0" {
		t.Error("expected 1.3.0")
	}
	if IncrementPatch(v).String() != "1.2.4" {
		t.Error("expected 1.2.4")
	}
}

func TestVersion_String(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3, Pre: "beta"}
	if v.String() != "1.2.3-beta" {
		t.Errorf("expected 1.2.3-beta, got %s", v.String())
	}
	v2 := Version{Major: 2, Minor: 0, Patch: 0}
	if v2.String() != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", v2.String())
	}
}