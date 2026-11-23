package gotarget

import "testing"

// Issue #618: `self` in a `fn mut` method on a generic struct is a
// `&mut Box<$T>`, so it can be handed to a function taking that pointer and
// the function's writes reach the caller's value.
func TestRunProgramPassesGenericMutableReceiverAsPointer(t *testing.T) {
	program := lowerSource(t, `struct Box<$T> {
  items: [$T],
}

impl Box {
  fn mut forward(item: $T) {
    touch(self, item)
  }

  fn mut forward_bound(item: $T) {
    let ns: &mut Box<$T> = self
    touch(ns, item)
  }

  fn count() Int {
    size(self)
  }

  fn same() Box<$T> {
    self
  }
}

fn touch(b: &mut Box<$T>, item: $T) {
  b.items.push(item)
}

fn size(b: Box<$T>) Int {
  b.items.size()
}

fn main() {
  let b = &mut Box<Str>{items: []}
  b.forward("a")
  b.forward_bound("b")
  if not b.count() == 2 { panic("receiver writes were lost") }
  if not b.same().items.at(1).or("") == "b" { panic("same() lost the receiver value") }
}`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

// Issue #619: a determined generic call result flows into an `Any`
// destination without first binding it to a local.
func TestRunProgramPassesGenericCallResultsAsAny(t *testing.T) {
	program := lowerSource(t, `use go:fmt

struct R {
  n: Int,
}

struct Box<$T> {
  items: [$T],
}

impl Box {
  fn reports() [R] {
    [R{n: 1}, R{n: 2}]
  }
}

fn reports_for(seed: $T) [R] {
  [R{n: 3}]
}

fn render(v: Any) Str {
  fmt::Sprint(v)
}

fn main() {
  let b = Box<Int>{items: []}
  let direct = render(b.reports())
  if not direct == "[\{1} \{2}]" { panic("method result = {direct}") }
  let annotated: Any = reports_for(true)
  let rendered = render(annotated)
  if not rendered == "[\{3}]" { panic("function result = {rendered}") }
}`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
