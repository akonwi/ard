package version

import (
	"testing"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input   string
		want    Semver
		wantErr bool
	}{
		{"0.13.0", Semver{0, 13, 0}, false},
		{"v0.13.0", Semver{0, 13, 0}, false},
		{"1.2.3", Semver{1, 2, 3}, false},
		{"0.1", Semver{0, 1, 0}, false},
		{"v1.0", Semver{1, 0, 0}, false},
		{"", Semver{}, true},
		{"abc", Semver{}, true},
		{"1.2.3.4", Semver{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSemver(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseSemver(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSemverCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.13.0", "0.13.0", 0},
		{"0.13.1", "0.13.0", 1},
		{"0.13.0", "0.13.1", -1},
		{"0.14.0", "0.13.0", 1},
		{"1.0.0", "0.99.99", 1},
		{"0.1.0", "0.2.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a, _ := ParseSemver(tt.a)
			b, _ := ParseSemver(tt.b)
			got := a.Compare(b)
			if got != tt.want {
				t.Fatalf("%s.Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		input  string
		wantOp string
		wantV  Semver
	}{
		{">= 0.13.0", ">=", Semver{0, 13, 0}},
		{">=0.13.0", ">=", Semver{0, 13, 0}},
		{"> 1.0.0", ">", Semver{1, 0, 0}},
		{"= 0.13.0", "=", Semver{0, 13, 0}},
		{"< 1.0.0", "<", Semver{1, 0, 0}},
		{"<= 2.0.0", "<=", Semver{2, 0, 0}},
		// bare version treated as >=
		{"0.13.0", ">=", Semver{0, 13, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c, err := ParseConstraint(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Op != tt.wantOp {
				t.Fatalf("op = %q, want %q", c.Op, tt.wantOp)
			}
			if c.Version != tt.wantV {
				t.Fatalf("version = %v, want %v", c.Version, tt.wantV)
			}
		})
	}
}

func TestConstraintCheck(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		{">= 0.13.0", "0.13.0", true},
		{">= 0.13.0", "0.14.0", true},
		{">= 0.13.0", "1.0.0", true},
		{">= 0.13.0", "0.12.0", false},
		{">= 0.13.0", "0.12.99", false},
		{"> 0.13.0", "0.13.0", false},
		{"> 0.13.0", "0.13.1", true},
		{"= 0.13.0", "0.13.0", true},
		{"= 0.13.0", "0.13.1", false},
		{"< 1.0.0", "0.99.0", true},
		{"< 1.0.0", "1.0.0", false},
		{"<= 1.0.0", "1.0.0", true},
		{"<= 1.0.0", "1.0.1", false},
		// bare version = >=
		{"0.13.0", "0.13.0", true},
		{"0.13.0", "0.12.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			v, err := ParseSemver(tt.version)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got := c.Check(v); got != tt.want {
				t.Fatalf("Constraint(%s).Check(%s) = %v, want %v", tt.constraint, tt.version, got, tt.want)
			}
		})
	}
}

func TestCheckVersionDevSkipsCheck(t *testing.T) {
	// The default version is "dev", so CheckVersion should always pass
	if err := CheckVersion(">= 99.0.0"); err != nil {
		t.Fatalf("expected dev version to skip check, got: %v", err)
	}
}

func TestCheckVersionWithRealVersion(t *testing.T) {
	// Temporarily override the version
	original := Version
	defer func() { Version = original }()

	Version = "0.13.0"

	t.Run("satisfied constraint", func(t *testing.T) {
		if err := CheckVersion(">= 0.13.0"); err != nil {
			t.Fatalf("expected check to pass: %v", err)
		}
	})

	t.Run("unsatisfied constraint", func(t *testing.T) {
		err := CheckVersion(">= 0.14.0")
		if err == nil {
			t.Fatal("expected error for unsatisfied constraint")
		}
		expected := "this project requires Ard >= 0.14.0, but you are running 0.13.0"
		if err.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("bare version acts as >=", func(t *testing.T) {
		if err := CheckVersion("0.12.0"); err != nil {
			t.Fatalf("expected bare version check to pass: %v", err)
		}
		if err := CheckVersion("0.14.0"); err == nil {
			t.Fatal("expected bare version check to fail")
		}
	})
}
