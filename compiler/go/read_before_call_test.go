package gotarget

import (
	"strings"
	"testing"
)

// The tests in this file cover reads that sit to the left of a call in one Go
// expression (#575). Go orders only the calls, so without help a read can see
// a value the call to its right wrote. The backend wraps such a read in
// ard.Read only when the call could change what it reads.

const readBeforeCallHelpers = `
struct Box {
  value: Int,
}

struct Holder {
  box: &mut Box,
  n: Int,
}

fn bump(box: &mut Box) Int {
  box.value = 5
  0
}

fn set_int(n: &mut Int) Int {
  n.* = 5
  0
}

fn noop() Int {
  0
}
`

func TestGoTargetReadsBeforeCallsSeeTheirValueFirst(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "a field reached through a pointer, in each composite shape",
			body: `
  let box = &mut Box{value: 1}
  let list = [box.value, bump(box)]
  box.value = 1
  let sum = box.value + bump(box)
  box.value = 1
  let args = take(box.value, bump(box))
  box.value = 1
  let holder = Holder{box: &mut Box{value: 0}, n: box.value + 0 * bump(box)}
  box.value = 1
  let pair = Pair{a: box.value, b: bump(box)}
  fmt::Println(list, sum, args, holder.n, pair.a)`,
			want: "[1 0] 1 10 1 1\n",
		},
		{
			name: "a field through a pointer held in a let struct value",
			body: `
  let holder = Holder{box: &mut Box{value: 1}, n: 1}
  fmt::Println([holder.box.value, bump(holder.box)])`,
			want: "[1 0]\n",
		},
		{
			name: "a dereferenced pointer",
			body: `
  mut n = 1
  let p = &mut n
  fmt::Println([p.*, set_int(p)])`,
			want: "[1 0]\n",
		},
		{
			name: "a mut local borrowed with &mut",
			body: `
  mut n = 1
  fmt::Println([n, set_int(&mut n)])`,
			want: "[1 0]\n",
		},
		{
			name: "a mut local a closure assigns",
			body: `
  mut n = 1
  let change = fn() Int {
    n = 5
    0
  }
  fmt::Println([n, change()])`,
			want: "[1 0]\n",
		},
		{
			name: "a let local whose address Go code writes through",
			body: `
  let n = 1
  fmt::Println([n, overwrite(&n)])`,
			want: "[1 0]\n",
		},
		{
			name: "a mut module global",
			body: `
  counter = 1
  let list = [counter, bump_counter()]
  counter = 1
  fmt::Println(list, counter + bump_counter())`,
			want: "[1 0] 1\n",
		},
		{
			name: "a Go package variable",
			body: `
  os::Args = ["original"]
  fmt::Println(os::Args, reset_args())`,
			want: "[original] 0\n",
		},
		{
			name: "a callee read through a pointer before its argument's call",
			body: `
  let holder = &mut Callbacks{callback: fn(x: Int) Int { x }}
  fmt::Println(holder.callback(swap(holder)))`,
			want: "1\n",
		},
		{
			name: "an assignment target whose pointer a call rebinds",
			body: `
  mut box = &mut Box{value: 1}
  let first = box
  let rebox = fn() Int {
    box = &mut Box{value: 7}
    9
  }
  box.value = rebox()
  fmt::Println(first.value, box.value)`,
			want: "9 7\n",
		},
		{
			name: "a while condition",
			body: `
  let box = &mut Box{value: 0}
  mut n = 0
  while box.value < step(box) {
    n = n + 1
  }
  fmt::Println(n, box.value)`,
			want: "3 4\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runGoTargetSourceStdout(t, readBeforeCallProgram(tt.body))
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Reads that no call can change stay inline: the generated Go is what it was
// before reads were ordered.
func TestGoTargetReadsNoCallCanChangeStayInline(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		inline string
	}{
		{
			name: "a let local",
			body: `
  let a = 1
  fmt::Println([a, noop()])`,
			inline: "[]int{a, Noop()}",
		},
		{
			name: "a mut local nothing borrows or assigns from a closure",
			body: `
  mut m = 1
  m = 2
  let read = fn() Int {
    m
  }
  fmt::Println([m, read()])`,
			inline: "[]int{m, read()}",
		},
		{
			name: "a field of a let struct value",
			body: `
  let holder = Holder{box: &mut Box{value: 1}, n: 1}
  fmt::Println([holder.n, bump(holder.box)])`,
			inline: "[]int{holder.N, Bump(holder.Box)}",
		},
		{
			name: "a pointer held in a let binding",
			body: `
  let box = &mut Box{value: 1}
  fmt::Println([bump(box), bump(box)], box.value)
  box.value = bump(box)`,
			inline: "box.Value = Bump(box)",
		},
		{
			name: "the size of a let string",
			body: `
  let text = "abc"
  fmt::Println([text.size(), noop()])`,
			inline: "[]int{len(text), Noop()}",
		},
		{
			name: "a let module global and a Go constant",
			body: `
  fmt::Println([limit, strconv::IntSize, noop()])`,
			inline: "[]int{Limit, strconv.IntSize, Noop()}",
		},
		{
			name: "an operand that is itself a call",
			body: `
  let box = &mut Box{value: 1}
  fmt::Println([noop(), bump(box)], [Maybe::new(box.value), Maybe::new(bump(box))])`,
			inline: "[]int{Noop(), Bump(box)}",
		},
		{
			name: "a read before calls to the runtime only",
			body: `
  let box = &mut Box{value: 1}
  let opt = Opt{a: box.value, b: Maybe::new(2)}
  fmt::Println(opt.a)`,
			inline: "Opt{A: box.Value, B: ard.Some[int](2)}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := generatedReadBeforeCallSource(t, readBeforeCallProgram(tt.body))
			if !strings.Contains(source, tt.inline) {
				t.Fatalf("generated Go does not contain %q:\n%s", tt.inline, source)
			}
			if strings.Contains(source, "ard.Read(") {
				t.Fatalf("generated Go reads an operand in order that no call can change:\n%s", source)
			}
		})
	}
}

// Maybe::new and the string methods lower to a Go function call, so the reads
// in their arguments happen before a call to their right without a wrapper.
// valueMayChange relies on this; the output changes if their lowering stops
// being a call.
func TestReadOrderCallKindsLowerToCalls(t *testing.T) {
	got := runGoTargetSourceStdout(t, readBeforeCallProgram(`
  let box = &mut Box{value: 1}
  let label = &mut Label{text: "ab"}
  let some = [Maybe::new(box.value), Maybe::new(bump(box))]
  let has = [label.text.contains("b"), relabel(label).contains("b")]
  label.text = "ab"
  let starts = [label.text.starts_with("a"), relabel(label).starts_with("a")]
  label.text = "ab"
  let ends = [label.text.ends_with("b"), relabel(label).ends_with("b")]
  label.text = "ab"
  let replaced = [label.text.replace("a", "x"), relabel(label).replace("a", "x")]
  label.text = "ab"
  let all = [label.text.replace_all("a", "x"), relabel(label).replace_all("a", "x")]
  label.text = " ab "
  let trimmed = [label.text.trim(), relabel(label).trim()]
  fmt::Println(some.at(0).or(Maybe::new(0)).or(0), has, starts, ends, replaced, all, trimmed)`))
	want := "1 [true false] [true false] [true false] [xb zz] [xb zz] [ab zz]\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func readBeforeCallProgram(body string) string {
	return `use go:fmt
use go:os
use go:reflect
use go:strconv
` + readBeforeCallHelpers + `
struct Pair {
  a: Int,
  b: Int,
}

struct Opt {
  a: Int,
  b: Int?,
}

struct Label {
  text: Str,
}

struct Callbacks {
  callback: fn(Int) Int,
}

mut counter = 1

let limit = 3

fn take(a: Int, b: Int) Int {
  a * 10 + b
}

fn bump_counter() Int {
  counter = 5
  0
}

fn overwrite(p: &Int) Int {
  reflect::ValueOf(p).Elem().SetInt(Int64::from(5))
  0
}

fn reset_args() Int {
  os::Args = ["changed"]
  0
}

fn swap(holder: &mut Callbacks) Int {
  holder.callback = fn(x: Int) Int { x + 100 }
  1
}

fn step(box: &mut Box) Int {
  box.value = box.value + 1
  3
}

fn relabel(label: &mut Label) Str {
  label.text = "zz"
  "zz"
}

fn main() {` + body + `
}
`
}

func generatedReadBeforeCallSource(t *testing.T, input string) string {
	t.Helper()
	program := lowerParitySource(t, input)
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	return joinGeneratedSources(sources)
}
