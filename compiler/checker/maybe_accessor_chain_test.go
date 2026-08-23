package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestMaybeFieldChainsAreSupportedInTryAndMatch(t *testing.T) {
	source := `
struct Profile {
  nickname: Str?,
  age: Int,
}

struct User {
  profile: Profile?,
  primary: Profile,
}

struct Box<$T> {
  value: $T,
}

fn nickname_from_try(user: User?) Str? {
  let nickname = try user.profile.nickname
  Maybe::new(nickname)
}

fn age_from_try(user: User?) Int? {
  let age = try user.primary.age
  Maybe::new(age)
}

fn age_from_match(user: User?) Int {
  match user.profile.age {
    age => age,
    _ => -1,
  }
}

fn reference_match(user: mut Maybe<User>) Int {
  match user.primary.age {
    age => age,
    _ => -1,
  }
}

fn reference_try(user: mut Maybe<User>) Int {
  let age = try user.primary.age -> _ { -1 }
  age
}

fn generic_required(box: Box<Str>?) Str {
  match box.value {
    value => value,
    _ => "missing",
  }
}

fn generic_optional(box: Box<Str?>?) Str {
  match box.value {
    value => value,
    _ => "missing",
  }
}

fn native_maybe_member(user: User?) Bool {
  match user.is_some() {
    true => true,
    false => false,
  }
}
`
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestMaybeFieldChainsSupportImportedArdStructs(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "types.ard"), []byte(`struct Profile {
  nickname: Str?,
}

struct User {
  profile: Profile?,
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	source := `use demo/types

fn from_match(user: types::User?) Str {
  match user.profile.nickname {
    nickname => nickname,
    _ => "missing",
  }
}

fn from_try(user: types::User?) Str {
  let nickname = try user.profile.nickname -> _ { "missing" }
  nickname
}
`
	result := parse.Parse([]byte(source), mainPath)
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	resolver, err := checker.NewModuleResolver(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	c := checker.New(mainPath, result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", c.Diagnostics())
	}
}

func TestMaybeFieldChainRejectsUnknownGenericOptionality(t *testing.T) {
	source := `
struct Box<$T> { value: $T }

fn invalid(box: Box<$T>?) Bool {
  match box.value {
    value => true,
    _ => false,
  }
}
`
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if len(c.Diagnostics()) != 1 {
		t.Fatalf("diagnostics = %v, want exactly one", c.Diagnostics())
	}
	for _, diagnostic := range c.Diagnostics() {
		if diagnostic.Code != checker.DiagnosticCodeUnresolvedGeneric {
			continue
		}
		if diagnostic.Title != "Unresolved nullable field access" {
			t.Fatalf("diagnostic title = %q", diagnostic.Title)
		}
		if diagnostic.Message != "Cannot access field `value` through Maybe while its type is unresolved: $T" {
			t.Fatalf("diagnostic message = %q", diagnostic.Message)
		}
		if diagnostic.Primary.Message != "field type must be concrete here" {
			t.Fatalf("primary label = %q", diagnostic.Primary.Message)
		}
		return
	}
	t.Fatalf("diagnostics = %v, want %s", c.Diagnostics(), checker.DiagnosticCodeUnresolvedGeneric)
}

func TestMaybeFieldChainRemainsInvalidOutsideTryOrMatch(t *testing.T) {
	source := `
struct User { name: Str? }

fn invalid(user: User?) Str? {
  user.name
}
`
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if !c.HasErrors() {
		t.Fatal("expected direct Maybe field access to remain invalid")
	}
	for _, diagnostic := range c.Diagnostics() {
		if diagnostic.Code == checker.DiagnosticCodeUndefinedMember {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want %s", c.Diagnostics(), checker.DiagnosticCodeUndefinedMember)
}
