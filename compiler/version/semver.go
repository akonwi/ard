package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver represents a parsed semantic version (major.minor.patch).
type Semver struct {
	Major int
	Minor int
	Patch int
}

func (v Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Comparison results for Semver.Compare.
const (
	LessThan    = -1
	Equal       = 0
	GreaterThan = 1
)

// Compare returns LessThan if v < other, Equal if v == other, GreaterThan if v > other.
func (v Semver) Compare(other Semver) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return LessThan
		}
		return GreaterThan
	}
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return LessThan
		}
		return GreaterThan
	}
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return LessThan
		}
		return GreaterThan
	}
	return Equal
}

// ParseSemver parses a version string like "0.13.0" or "v0.13.0".
func ParseSemver(s string) (Semver, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")

	if len(parts) < 2 || len(parts) > 3 {
		return Semver{}, fmt.Errorf("invalid version format: %q (expected MAJOR.MINOR or MAJOR.MINOR.PATCH)", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid major version in %q: %w", s, err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid minor version in %q: %w", s, err)
	}

	patch := 0
	if len(parts) == 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Semver{}, fmt.Errorf("invalid patch version in %q: %w", s, err)
		}
	}

	return Semver{Major: major, Minor: minor, Patch: patch}, nil
}

// Constraint represents a version constraint like ">= 0.13.0".
type Constraint struct {
	Op      string // ">=", ">", "=", "<=", "<"
	Version Semver
}

// Check returns true if the given version satisfies this constraint.
func (c Constraint) Check(v Semver) bool {
	cmp := v.Compare(c.Version)
	switch c.Op {
	case ">=":
		return cmp == GreaterThan || cmp == Equal
	case ">":
		return cmp == GreaterThan
	case "=", "==":
		return cmp == Equal
	case "<=":
		return cmp == LessThan || cmp == Equal
	case "<":
		return cmp == LessThan
	default:
		return false
	}
}

func (c Constraint) String() string {
	return fmt.Sprintf("%s %s", c.Op, c.Version)
}

// ParseConstraint parses a constraint string like ">= 0.13.0" or "0.13.0" (exact match).
func ParseConstraint(s string) (Constraint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Constraint{}, fmt.Errorf("empty version constraint")
	}

	// Try to extract operator
	for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
		if strings.HasPrefix(s, op) {
			ver, err := ParseSemver(strings.TrimSpace(s[len(op):]))
			if err != nil {
				return Constraint{}, err
			}
			return Constraint{Op: op, Version: ver}, nil
		}
	}

	// No operator — treat as exact match
	ver, err := ParseSemver(s)
	if err != nil {
		return Constraint{}, err
	}
	return Constraint{Op: ">=", Version: ver}, nil
}

// CheckVersion checks whether the current compiler version satisfies the given
// constraint string. Returns nil if satisfied, or an error describing the mismatch.
// If the compiler version is "dev", the check is skipped.
func CheckVersion(constraint string) error {
	current := Get()
	if current == "dev" {
		return nil
	}

	currentVer, err := ParseSemver(current)
	if err != nil {
		return fmt.Errorf("failed to parse compiler version %q: %w", current, err)
	}

	c, err := ParseConstraint(constraint)
	if err != nil {
		return fmt.Errorf("invalid ard_version constraint %q: %w", constraint, err)
	}

	if !c.Check(currentVer) {
		return fmt.Errorf("this project requires Ard %s, but you are running %s", c, currentVer)
	}

	return nil
}
