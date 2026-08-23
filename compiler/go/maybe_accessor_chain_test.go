package gotarget

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramProjectsMaybeFieldsInTryAndMatch(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"maybechain\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`struct Profile {
  nickname: Str?,
  age: Int,
}

struct User {
  profile: Profile?,
  primary: Profile,
}

struct Counter {
  calls: Int,
}

fn nickname_or(user: User?, fallback: Str) Str {
  let nickname = try user.profile.nickname -> _ { fallback }
  nickname
}

fn age_or(user: User?, fallback: Int) Int {
  let age = try user.primary.age -> _ { fallback }
  age
}

fn nickname_from_match(user: User?, fallback: Str) Str {
  match user.profile.nickname {
    nickname => nickname,
    _ => fallback,
  }
}

fn age_from_match(user: User?, fallback: Int) Int {
  match user.primary.age {
    age => age,
    _ => fallback,
  }
}

fn age_from_reference_match(user: mut Maybe<User>, fallback: Int) Int {
  match user.primary.age {
    age => age,
    _ => fallback,
  }
}

fn age_from_reference_try(user: mut Maybe<User>, fallback: Int) Int {
  let age = try user.primary.age -> _ { fallback }
  age
}

fn tracked(counter: mut Counter, user: User?) User? {
  counter.calls = counter.calls + 1
  user
}

fn main() {
  let primary = Profile{nickname: Maybe::new("primary"), age: 42}
  let present: User? = Maybe::new(User{
    profile: Maybe::new(Profile{nickname: Maybe::new("Ada"), age: 7}),
    primary: primary,
  })
  let missing_name: User? = Maybe::new(User{
    profile: Maybe::new(Profile{nickname: Maybe::new(), age: 8}),
    primary: primary,
  })
  let missing_profile: User? = Maybe::new(User{
    profile: Maybe::new(),
    primary: primary,
  })
  let absent: User? = Maybe::new()

  if nickname_or(present, "missing") != "Ada" {
    panic("present optional field did not project")
  }
  if nickname_or(missing_name, "missing") != "missing" {
    panic("absent optional field did not flatten")
  }
  if nickname_or(missing_profile, "missing") != "missing" {
    panic("absent intermediate field did not propagate")
  }
  if nickname_or(absent, "missing") != "missing" {
    panic("absent outer value did not propagate")
  }
  if age_or(present, -1) != 42 {
    panic("required field did not wrap")
  }
  if age_or(absent, -1) != -1 {
    panic("required field did not propagate absence")
  }

  if nickname_from_match(present, "missing") != "Ada" {
    panic("match did not project present optional field")
  }
  if nickname_from_match(missing_name, "missing") != "missing" {
    panic("match did not flatten absent optional field")
  }
  if nickname_from_match(missing_profile, "missing") != "missing" {
    panic("match did not propagate absent intermediate field")
  }
  if nickname_from_match(absent, "missing") != "missing" {
    panic("match did not propagate absent outer value")
  }
  if age_from_match(present, -1) != 42 or age_from_match(absent, -1) != -1 {
    panic("match did not project required field")
  }

  let present_reference = mut present
  let absent_reference = mut absent
  if age_from_reference_match(present_reference, -1) != 42 or age_from_reference_match(absent_reference, -1) != -1 {
    panic("match did not project through a Maybe reference")
  }
  if age_from_reference_try(present_reference, -1) != 42 or age_from_reference_try(absent_reference, -1) != -1 {
    panic("try did not project through a Maybe reference")
  }

  let counter = mut Counter{calls: 0}
  let tracked_name = match tracked(counter, present).profile.nickname {
    nickname => nickname,
    _ => "missing",
  }
  if tracked_name != "Ada" or counter.calls != 1 {
    panic("projection evaluated its receiver more than once")
  }

  if not present.is_some() or absent.is_some() {
    panic("native Maybe methods changed meaning")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
