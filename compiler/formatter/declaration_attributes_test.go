package formatter

import "testing"

func TestFormatDeclarationAttributes(t *testing.T) {
	input := `use go:os
// signals
#platform( unix )
use go:syscall
fn watched() [os::Signal] { [os::Interrupt] }
#platform(unix,  darwin_arm64)
fn watched() [os::Signal] { [os::Interrupt, syscall::SIGWINCH] }

// Fallback size.
let size: Int = 1
#platform(windows)

let size: Int = 2

#platform(unix)
impl Foo {
  fn go() {}
}
`
	want := `use go:os
// signals
#platform(unix)
use go:syscall

fn watched() [os::Signal] {
  [os::Interrupt]
}
#platform(unix, darwin_arm64)
fn watched() [os::Signal] {
  [os::Interrupt, syscall::SIGWINCH]
}

// Fallback size.
let size: Int = 1
#platform(windows)
let size: Int = 2

#platform(unix)
impl Foo {
  fn go() {}
}
`
	got, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	second, err := Format(got, "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != want {
		t.Fatalf("attribute formatting is not idempotent:\n%s", second)
	}
}

func TestFormatSortsAttributedImportsWithTheirAttributes(t *testing.T) {
	input := `#platform(unix)
use go:syscall
use go:os

fn main() {
  os::Exit(syscall::SIGWINCH)
}
`
	want := `use go:os
#platform(unix)
use go:syscall

fn main() {
  os::Exit(syscall::SIGWINCH)
}
`
	got, err := Format([]byte(input), "test.ard")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("format mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
