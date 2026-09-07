package versioning

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string
}

func Parse(v string) (Version, error) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, "-")
	verParts := strings.Split(parts[0], ".")
	if len(verParts) != 3 {
		return Version{}, fmt.Errorf("invalid version: %s", v)
	}
	major, _ := strconv.Atoi(verParts[0])
	minor, _ := strconv.Atoi(verParts[1])
	patch, _ := strconv.Atoi(verParts[2])
	pre := ""
	if len(parts) > 1 {
		pre = parts[1]
	}
	return Version{Major: major, Minor: minor, Patch: patch, Pre: pre}, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		if v.Major > other.Major {
			return 1
		}
		return -1
	}
	if v.Minor != other.Minor {
		if v.Minor > other.Minor {
			return 1
		}
		return -1
	}
	if v.Patch != other.Patch {
		if v.Patch > other.Patch {
			return 1
		}
		return -1
	}
	if v.Pre == "" && other.Pre == "" {
		return 0
	}
	if v.Pre == "" {
		return 1
	}
	if other.Pre == "" {
		return -1
	}
	if v.Pre > other.Pre {
		return 1
	}
	if v.Pre < other.Pre {
		return -1
	}
	return 0
}

func (v Version) Equal(other Version) bool {
	return v.Compare(other) == 0
}

func (v Version) LessThan(other Version) bool {
	return v.Compare(other) < 0
}

func (v Version) GreaterThan(other Version) bool {
	return v.Compare(other) > 0
}

func (v Version) Satisfies(constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return true
	}
	if strings.HasPrefix(constraint, ">=") {
		other, _ := Parse(constraint[2:])
		return v.GreaterThan(other) || v.Equal(other)
	}
	if strings.HasPrefix(constraint, "<=") {
		other, _ := Parse(constraint[2:])
		return v.LessThan(other) || v.Equal(other)
	}
	if strings.HasPrefix(constraint, ">") {
		other, _ := Parse(constraint[1:])
		return v.GreaterThan(other)
	}
	if strings.HasPrefix(constraint, "<") {
		other, _ := Parse(constraint[1:])
		return v.LessThan(other)
	}
	if strings.HasPrefix(constraint, "~") {
		other, _ := Parse(constraint[1:])
		return v.Major == other.Major && v.Minor == other.Minor && v.Patch >= other.Patch
	}
	if strings.HasPrefix(constraint, "^") {
		other, _ := Parse(constraint[1:])
		if other.Major == 0 {
			return v.Major == other.Major && v.Minor >= other.Minor
		}
		return v.Major == other.Major && (v.Minor > other.Minor || (v.Minor == other.Minor && v.Patch >= other.Patch))
	}
	other, _ := Parse(constraint)
	return v.Equal(other)
}

func IncrementMajor(v Version) Version {
	return Version{Major: v.Major + 1, Minor: 0, Patch: 0}
}

func IncrementMinor(v Version) Version {
	return Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0}
}

func IncrementPatch(v Version) Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1}
}