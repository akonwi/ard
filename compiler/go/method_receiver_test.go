package gotarget

import (
	"strings"
	"testing"
)

// A struct with a `fn mut` method gives every method a pointer receiver
// (issue #622). These programs pin the Ard semantics that value receivers used
// to provide for non-mut methods: `self` used as a value is a copy, receivers
// that Go cannot address still work, trait objects built from values own a
// copy, and methods implementing imported Go interfaces keep value receivers
// so Go still finds them on a value.
const pointerReceiverProgram = `
use go:fmt

trait Describe {
  fn describe() Str
}

trait Named {
  fn name() Str
}

trait Titled {
  fn name() Str
}

struct Counter {
  name: Str,
  count: Int,
}

impl Counter {
  fn label() Str {
    "{self.name}={self.count}"
  }

  fn copy() Counter {
    self
  }

  fn snapshot() fn() Int {
    fn() { self.count }
  }

  fn plus(n: Int) Int {
    self.count + n
  }

  fn twice() Str {
    self.label() + self.label()
  }

  fn via_function() Str {
    describe_counter(self)
  }

  fn mut bump() {
    self.count = self.count + 1
  }

  fn mut next() Int {
    self.count = self.count + 1
    self.count
  }
}

impl Describe for Counter {
  fn describe() Str {
    "describe {self.name}={self.count}"
  }
}

impl Named for Counter {
  fn name() Str {
    "named {self.name}"
  }
}

impl Titled for Counter {
  fn name() Str {
    "titled {self.name}"
  }
}

impl fmt::Stringer for Counter {
  fn string() Str {
    "C({self.name})"
  }
}

fn describe_counter(c: Counter) Str {
  c.label()
}

fn make(name: Str) Counter {
  Counter{name: name, count: 0}
}

struct Stack {
  items: [$T],
}

impl Stack {
  fn size() Int {
    self.items.size()
  }

  fn mut push(item: $T) {
    self.items.push(item)
  }
}

fn main() {
  let c = &mut Counter{name: "a", count: 1}
  let before = c.copy()
  let later = c.snapshot()
  c.bump()
  fmt::Println(c.label(), before.label(), later())

  // The receiver is read when the method runs, after its arguments, as for
  // a fn mut method (ADR 0078): 3 + 3.
  fmt::Println(c.plus(c.next()))
  c.count = 2

  fmt::Println(make("m").label())
  let items = [Counter{name: "x", count: 4}]
  for item in items {
    fmt::Println(item.twice())
  }

  mut value = Counter{name: "v", count: 5}
  fmt::Println(value.via_function())
  let owned: Describe = value
  value.count = 6
  fmt::Println(owned.describe(), value.label())

  let shared: mut Describe = c
  c.bump()
  fmt::Println(shared.describe())

  let named: Named = value
  let titled: Titled = c
  fmt::Println(named.name(), titled.name())

  fmt::Println(value, c)

  let stack = &mut Stack<Int>{items: []}
  stack.push(1)
  stack.push(2)
  let plain = Stack<Str>{items: ["s"]}
  fmt::Println(stack.size(), plain.size())
}
`

func TestPointerReceiverMethodsKeepValueSemantics(t *testing.T) {
	got := runGoTargetSourceStdout(t, pointerReceiverProgram)
	want := strings.Join([]string{
		"a=2 a=1 1",
		"6",
		"m=0",
		"x=4x=4",
		"v=5",
		"describe v=5 v=6",
		"describe a=3",
		"named v titled a",
		"C(v) C(a)",
		"2 1",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

func TestPointerReceiverMethodShapes(t *testing.T) {
	source := generatedReadBeforeCallSource(t, pointerReceiverProgram)
	for _, want := range []string{
		"func (self *Counter) Label() string",
		"func (self *Counter) Bump()",
		// `self` as a value dereferences the receiver.
		"return *self",
		"DescribeCounter(*self)",
		// Field reads go through the pointer without a copy.
		"self.Name",
		// An imported Go interface method stays on the value.
		"func (self Counter) String() string",
		"func (self *Stack[T]) Size() int",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("generated Go missing %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "(*self).") {
		t.Fatalf("generated Go reads through (*self):\n%s", source)
	}
}
