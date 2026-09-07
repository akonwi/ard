package gotarget

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
)

func TestRunProgramSupportsGlobalInitializerLocalBinding(t *testing.T) {
	program := lowerSource(t, `
		let answer = {
			let value = 42
			value
		}

		fn main() {
			if answer != 42 { panic("bad") }
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestLowerGlobalInitializerUsesIIFEOnlyForSetupStatements(t *testing.T) {
	program := lowerSource(t, `
		let direct = 1
		let computed = {
			let value = 41
			value + 1
		}
		fn main() { let _ = direct + computed }
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	usesIIFE := func(name string) bool {
		return astFilesContain(files, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || spec.Names[0].Name != name || len(spec.Values) != 1 {
				return false
			}
			call, ok := spec.Values[0].(*ast.CallExpr)
			if !ok {
				return false
			}
			_, ok = call.Fun.(*ast.FuncLit)
			return ok
		})
	}
	if usesIIFE("Direct") {
		t.Fatal("literal global initializer unexpectedly uses an IIFE")
	}
	if !usesIIFE("Computed") {
		t.Fatal("statement-producing global initializer does not use an IIFE")
	}
}

func TestRunProgramSupportsImportedGlobalInitializerLocals(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.ard"), []byte(`
let saved = {
  let value = 42
  value
}

fn read() Int { saved }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  if feature::saved != 42 { panic("bad direct global") }
  if feature::read() != 42 { panic("bad function global") }
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
		t.Fatalf("lower AIR: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestGoTargetParityGlobalInitializerLocalContexts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "contextual Result ok block",
			source: `
				let saved: Int!Str = { Result::ok(42) }
				fn main() Int { saved.expect("bad") }
			`,
			want: "42",
		},
		{
			name: "contextual Result err block",
			source: `
				let saved: Int!Str = { Result::err("bad") }
				fn main() Str {
					match saved {
						ok(_) => "wrong",
						err(message) => message,
					}
				}
			`,
			want: `"bad"`,
		},
		{
			name: "contextual Maybe block",
			source: `
				let saved: Int? = { Maybe::new(42) }
				fn main() Int { saved.expect("bad") }
			`,
			want: "42",
		},
		{
			name: "multiple initializer local namespaces and function zero",
			source: `
				fn identity(value: Int) Int { value }
				let first = {
					let value = identity(20)
					value
				}
				let second = {
					let value = identity(22)
					value
				}
				fn main() Int { first + second }
			`,
			want: "42",
		},
		{
			name: "nested shadowing",
			source: `
				let answer = {
					let value = 20
					let nested = {
						let value = 22
						value
					}
					value + nested
				}
				fn main() Int { answer }
			`,
			want: "42",
		},
		{
			name: "mutable local assignment",
			source: `
				let answer = {
					mut value = 40
					value = value + 2
					value
				}
				fn main() Int { answer }
			`,
			want: "42",
		},
		{
			name: "escaping closure captures mutable slot",
			source: `
				let counter: fn() Int = {
					mut value = 40
					fn increment() Int {
						value = value + 1
						value
					}
					increment
				}
				fn main() Int {
					counter()
					counter()
				}
			`,
			want: "42",
		},
		{
			name: "recursive local function escapes initializer",
			source: `
				let add: fn(Int) Int = {
					fn add_inner(value: Int) Int {
						match value == 0 {
							true => 40,
							false => 1 + add_inner(value - 1),
						}
					}
					add_inner
				}
				fn main() Int { add(2) }
			`,
			want: "42",
		},
		{
			name: "match arm binding",
			source: `
				fn outcome() Int!Str { Result::ok(42) }
				let answer = match outcome() {
					ok(value) => value,
					err(_) => 0,
				}
				fn main() Int { answer }
			`,
			want: "42",
		},
		{
			name: "loop binding",
			source: `
				let answer = {
					mut total = 0
					for value in [20, 22] {
						total = total + value
					}
					total
				}
				fn main() Int { answer }
			`,
			want: "42",
		},
		{
			name: "escaping reference local",
			source: `
				struct Box { value: Int }
				let shared: mut Box = {
					let box = mut Box{value: 1}
					box
				}
				fn main() Int {
					shared.value = 42
					shared.value
				}
			`,
			want: "42",
		},
		{
			name: "later global dependency through function",
			source: `
				let observed = {
					let value = read_later()
					value
				}
				let later = {
					let value = 42
					value
				}
				fn read_later() Int { later }
				fn main() Int { observed }
			`,
			want: "42",
		},
		{
			name: "Go import alias collides with initializer local",
			source: `
				use go:fmt
				let answer = {
					let fmt = 42
					fmt::Sprint(fmt)
				}
				fn main() Str { answer }
			`,
			want: `"42"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerParitySource(t, test.source)
			if got := runGoTargetParityJSON(t, program); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}
