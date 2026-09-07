package manifest

import (
	"strings"
	"testing"
)

func TestParseReadsCompleteManifest(t *testing.T) {
	input := []byte(`name = "demo"
ard = ">= 0.40.0"
target = "go"

[go]
build_tags = ["sqlite"]

[dependencies]
ui = { path = "../ui" }
remote = { git = "https://example.com/remote.git", tag = "v1.2.3" }

[build.values]
version = { type = "Str", default = "dev", release = true }
build_number = { type = "Int", default = 0 }
experimental = { type = "Bool", default = false }
`)

	got, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Name != "demo" || got.Ard != ">= 0.40.0" || got.Target != "go" {
		t.Fatalf("manifest identity = %#v", got)
	}
	if len(got.Go.BuildTags) != 1 || got.Go.BuildTags[0] != "sqlite" {
		t.Fatalf("build tags = %#v", got.Go.BuildTags)
	}
	if got.Dependencies["ui"].Path != "../ui" || got.Dependencies["remote"].Tag != "v1.2.3" {
		t.Fatalf("dependencies = %#v", got.Dependencies)
	}
	if value := got.Build.Values["version"]; value.Type != BuildValueStr || value.Default != "dev" || !value.Release {
		t.Fatalf("version = %#v", value)
	}
	if value := got.Build.Values["build_number"]; value.Type != BuildValueInt || value.Default != int64(0) || value.Release {
		t.Fatalf("build_number = %#v", value)
	}
	if value := got.Build.Values["experimental"]; value.Type != BuildValueBool || value.Default != false {
		t.Fatalf("experimental = %#v", value)
	}
}

func TestParseRejectsInvalidBuildValues(t *testing.T) {
	tests := []struct {
		name    string
		decl    string
		wantErr string
	}{
		{name: "missing type", decl: `version = { default = "dev" }`, wantErr: `build value "version" is missing type`},
		{name: "missing default", decl: `version = { type = "Str" }`, wantErr: `build value "version" is missing default`},
		{name: "unsupported type", decl: `version = { type = "Float", default = 1.0 }`, wantErr: `unsupported type "Float"`},
		{name: "wrong string default", decl: `version = { type = "Str", default = 1 }`, wantErr: `default must be Str`},
		{name: "wrong int default", decl: `number = { type = "Int", default = "1" }`, wantErr: `default must be Int`},
		{name: "wrong bool default", decl: `enabled = { type = "Bool", default = "false" }`, wantErr: `default must be Bool`},
		{name: "bad release", decl: `version = { type = "Str", default = "dev", release = "yes" }`, wantErr: `release must be Bool`},
		{name: "unknown field", decl: `version = { type = "Str", default = "dev", secret = true }`, wantErr: `unknown field "secret"`},
		{name: "keyword", decl: `match = { type = "Str", default = "dev" }`, wantErr: `not a valid Ard identifier`},
		{name: "leading digit", decl: `123version = { type = "Str", default = "dev" }`, wantErr: `not a valid Ard identifier`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte("name = \"demo\"\nard = \">= 0.40.0\"\n\n[build.values]\n" + tt.decl + "\n")
			_, err := Parse(input)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Parse error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRejectsMalformedCompleteManifest(t *testing.T) {
	_, err := Parse([]byte("name = \"demo\"\nard = [\n"))
	if err == nil {
		t.Fatal("Parse succeeded for malformed TOML")
	}
}

func TestParseToleratesUnknownTopLevelFields(t *testing.T) {
	if _, err := Parse([]byte("name = \"demo\"\nard = \">= 0.40.0\"\nmystery = true\n")); err != nil {
		t.Fatalf("Parse: %v", err)
	}
}
