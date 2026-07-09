package checker_test

import (
	"strings"
	"testing"

	checker "github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestUnknownParameterTypeMethodLookupReportsDiagnostics(t *testing.T) {
	result := parse.Parse([]byte(strings.Join([]string{
		`fn stringify(x: Missing) Str {`,
		`  x.to_str()`,
		`}`,
	}, "\n")), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}

	c := checker.New("test.ard", result.Program, nil)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("checker panicked for method lookup on unresolved type: %v", r)
		}
	}()
	c.Check()

	diagnostics := diagnosticsString(c.Diagnostics())
	if !strings.Contains(diagnostics, "Unrecognized type: Missing") {
		t.Fatalf("diagnostics = %v, want unrecognized type diagnostic", c.Diagnostics())
	}
	if !strings.Contains(diagnostics, "Undefined: x.to_str") {
		t.Fatalf("diagnostics = %v, want undefined method diagnostic", c.Diagnostics())
	}
}
func TestFunctions(t *testing.T) {
	run(t, []test{
		{
			name: "Calling empty function",
			input: strings.Join(
				[]string{
					`fn noop() {}`,
					`noop()`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name:       "noop",
							Parameters: []checker.Parameter{},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "noop",
							Args: []checker.Expression{},
						},
					},
				},
			},
		},
		{
			name: "Calling function with parameters",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) {}`,
					`add(1, 2)`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "add",
							Parameters: []checker.Parameter{
								{Name: "a", Type: checker.Int, Mutable: false},
								{Name: "b", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "add",
							Args: []checker.Expression{
								&checker.IntLiteral{Value: 1},
								&checker.IntLiteral{Value: 2},
							},
						},
					},
				},
			},
		},
		{
			name: "Mutable parameters",
			input: strings.Join(
				[]string{
					`fn update(value: mut Int) {}`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "update",
							Parameters: []checker.Parameter{
								{Name: "value", Type: checker.Int, Mutable: true},
							},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
				},
			},
		},
		{
			name: "Functions should return the declared return type",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) Int { false }`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Bool"},
			},
		},
		{
			name: "A functiton with a declared return type should have a final expression that satisfies it",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) Void!Bool {
						let foo = false
					}`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Void!Bool, got Void"},
			},
		},
		{
			name: "Functions can declare Void in return type",
			input: strings.Join(
				[]string{
					`fn no_return() Void { true }`,
					`fn void_ok() Void!Bool { Result::err(true) }`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Type mismatch in function arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str) {}`,
					`greet(42)`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "Incorrect number of arguments",
			input: strings.Join(
				[]string{
					`fn add(a: Int, b: Int) {}`,
					`add(1)`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "missing argument for parameter: b"},
			},
		},
		{
			name: "Reports all callsites with too many arguments",
			input: strings.Join(
				[]string{
					`fn read_move() Int { 1 }`,
					`fn main() {`,
					`  let player = "X"`,
					`  mut move = read_move(player)`,
					`  while move < 0 {`,
					`    move = read_move(player)`,
					`  }`,
					`  while move < 9 {`,
					`    move = read_move(player)`,
					`  }`,
					`}`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 0, got 1"},
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 0, got 1"},
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 0, got 1"},
			},
		},
	})
}
func TestTestFunctions(t *testing.T) {
	t.Run("marks valid test functions", func(t *testing.T) {
		result := parse.Parse([]byte(`test fn works() Void!Str { Result::ok(()) }`), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("Parse errors: %v", result.Errors[0].Message)
		}

		c := checker.New("test.ard", result.Program, nil)
		c.Check()
		if c.HasErrors() {
			t.Fatalf("Diagnostics found: %v", c.Diagnostics())
		}

		fn, ok := c.Module().Program().Statements[0].Expr.(*checker.FunctionDef)
		if !ok {
			t.Fatalf("Expected first statement to be a function definition, got %T", c.Module().Program().Statements[0].Expr)
		}
		if !fn.IsTest {
			t.Fatalf("Expected function to be marked as test")
		}
	})

	run(t, []test{
		{
			name:  "test functions must not take parameters",
			input: `test fn invalid(name: Str) Void!Str { Result::ok(()) }`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "test functions must not take parameters"},
			},
		},
		{
			name:  "test functions must return Void!Str",
			input: `test fn invalid() {}`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "test functions must return Void!Str"},
			},
		},
		{
			name: "co-located test can call private functions",
			input: strings.Join([]string{
				`private fn secret() Int { 42 }`,
				`test fn test_secret() Void!Str {`,
				`  secret()`,
				`  Result::ok(())`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "co-located test can call public functions",
			input: strings.Join([]string{
				`fn public_fn() Int { 7 }`,
				`test fn test_public() Void!Str {`,
				`  public_fn()`,
				`  Result::ok(())`,
				`}`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{},
		},

		{
			name: "explicit type arguments on non-generic function are rejected",
			input: strings.Join([]string{
				`fn foo() Int { 0 }`,
				`foo<Int>()`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "function foo does not take type arguments"},
			},
		},
		{
			name:  "explicit type arguments on panic are rejected",
			input: `panic<Int>("boom")`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "function panic does not take type arguments"},
			},
		},
		{
			name:  "test functions returning wrong error type",
			input: `test fn wrong_err() Void!Int { Result::ok(()) }`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "test functions must return Void!Str"},
			},
		},
		{
			name:  "test functions returning plain value",
			input: `test fn returns_int() Int { 42 }`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "test functions must return Void!Str"},
			},
		},
	})
}
func TestCallingInstanceMethods(t *testing.T) {
	run(t, []test{
		{
			name:  "Int.to_str()",
			input: `200.to_str()`,
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.IntMethod{
							Subject: &checker.IntLiteral{200},
							Kind:    checker.IntToStr,
						},
					},
				},
			},
		},
	})
}
func TestNullableParameterCallSugar(t *testing.T) {
	run(t, []test{
		{
			name: "omits trailing nullable positional arguments",
			input: strings.Join([]string{
				`fn configure(name: Str, retries: Int?, verbose: Bool?) {}`,
				`configure("worker")`,
			}, "\n"),
		},
		{
			name: "wraps provided nullable parameter values",
			input: strings.Join([]string{
				`fn configure(name: Str, retries: Int?, verbose: Bool?) {}`,
				`configure("worker", 3, true)`,
			}, "\n"),
		},
		{
			name: "passes explicit maybe arguments through",
			input: strings.Join([]string{
				`fn configure(name: Str, retries: Int?) {}`,
				`configure("worker", Maybe::new(3))`,
				`configure("worker", Maybe::new())`,
			}, "\n"),
		},
		{
			name: "named arguments can skip nullable parameters",
			input: strings.Join([]string{
				`fn configure(retries: Int?, name: Str) {}`,
				`configure(name: "worker")`,
			}, "\n"),
		},
		{
			name: "static functions use nullable argument sugar",
			input: strings.Join([]string{
				`struct Config { name: Str, retries: Int? }`,
				`fn Config::new(name: Str, retries: Int?) Config { Config{name: name, retries: retries} }`,
				`Config::new("worker")`,
				`Config::new(name: "worker")`,
				`Config::new("worker", 3)`,
			}, "\n"),
		},
		{
			name: "generic nullable parameter can be omitted",
			input: strings.Join([]string{
				`fn optional(value: $T?) $T? { value }`,
				`let missing: Int? = optional<Int>()`,
				`let present: Int? = optional<Int>(1)`,
				`let explicit: Int? = optional<Int>(Maybe::new(2))`,
			}, "\n"),
		},
		{
			name: "positional arguments cannot skip non-trailing nullable parameters",
			input: strings.Join([]string{
				`fn configure(retries: Int?, name: Str) {}`,
				`configure("worker")`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{{Kind: checker.Error, Message: "missing argument for parameter: name"}},
		},
	})
}

func TestNamedArguments(t *testing.T) {
	run(t, []test{
		{
			name: "Named arguments in correct order",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) Str { "hello" }`,
					`greet(name: "Alice", age: 25)`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "greet",
							Parameters: []checker.Parameter{
								{Name: "name", Type: checker.Str, Mutable: false},
								{Name: "age", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Str,
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "hello"}},
								},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "greet",
							Args: []checker.Expression{
								&checker.StrLiteral{Value: "Alice"},
								&checker.IntLiteral{Value: 25},
							},
						},
					},
				},
			},
		},
		{
			name: "Named arguments in reverse order",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) Str { "hello" }`,
					`greet(age: 42, name: "Bob")`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "greet",
							Parameters: []checker.Parameter{
								{Name: "name", Type: checker.Str, Mutable: false},
								{Name: "age", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Str,
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.StrLiteral{Value: "hello"}},
								},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "greet",
							Args: []checker.Expression{
								&checker.StrLiteral{Value: "Bob"},
								&checker.IntLiteral{Value: 42},
							},
						},
					},
				},
			},
		},
		{
			name: "Named arguments with three parameters",
			input: strings.Join(
				[]string{
					`fn calculate(x: Int, y: Int, z: Int) Int { x }`,
					`calculate(z: 3, x: 1, y: 2)`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "calculate",
							Parameters: []checker.Parameter{
								{Name: "x", Type: checker.Int, Mutable: false},
								{Name: "y", Type: checker.Int, Mutable: false},
								{Name: "z", Type: checker.Int, Mutable: false},
							},
							ReturnType: checker.Int,
							Body: &checker.Block{
								Stmts: []checker.Statement{
									{Expr: &checker.Variable{}},
								},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "calculate",
							Args: []checker.Expression{
								&checker.IntLiteral{Value: 1},
								&checker.IntLiteral{Value: 2},
								&checker.IntLiteral{Value: 3},
							},
						},
					},
				},
			},
		},
		{
			name: "Unknown parameter name",
			input: strings.Join(
				[]string{
					`fn greet(name: Str) {}`,
					`greet(unknown: "value")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "unknown parameter name: unknown"},
			},
		},
		{
			name: "Parameter specified multiple times with positional and named",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet("Alice", name: "Bob")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "parameter name specified multiple times"},
			},
		},
		{
			name: "Parameter specified multiple times with named arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet(name: "Alice", name: "Bob")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "parameter name specified multiple times"},
			},
		},
		{
			name: "Missing parameter with named arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet(name: "Alice")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "missing argument for parameter: age"},
			},
		},
		{
			name: "Type mismatch with named arguments",
			input: strings.Join(
				[]string{
					`fn greet(name: Str, age: Int) {}`,
					`greet(name: 42, age: "hello")`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "Named arguments with single parameter",
			input: strings.Join(
				[]string{
					`fn hello(name: Str) {}`,
					`hello(name: "World")`,
				},
				"\n",
			),
			output: &checker.Program{
				Statements: []checker.Statement{
					{
						Expr: &checker.FunctionDef{
							Name: "hello",
							Parameters: []checker.Parameter{
								{Name: "name", Type: checker.Str, Mutable: false},
							},
							ReturnType: checker.Void,
							Body: &checker.Block{
								Stmts: []checker.Statement{},
							},
						},
					},
					{
						Expr: &checker.FunctionCall{
							Name: "hello",
							Args: []checker.Expression{
								&checker.StrLiteral{Value: "World"},
							},
						},
					},
				},
			},
		},
	})
}
func TestInferringEmptyCollectionArguments(t *testing.T) {
	run(t, []test{
		{
			name: "Empty array argument has no error",
			input: `
				fn greet(names: [Str]) {}
				greet(names: [])
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "Empty map argument has no error",
			input: `
				fn foo(map: [Str:Int]) {}
				foo(map: [:])
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
func TestGroupedNullableFunctionTypes(t *testing.T) {
	run(t, []test{
		{
			name: "Grouped nullable function type supports Maybe APIs",
			input: `

				let f: (fn(Int) Void)? = Maybe::new()
				f.is_none()
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
func TestTypeDoubleColonFunctionDefinition(t *testing.T) {
	run(t, []test{
		{
			name: "Defining and calling Type::function should recognize the function",
			input: `
				struct Fixture {
					id: Int,
					name: Str,
				}

				fn Fixture::from_entry(data: Str) Fixture {
					Fixture{id: 1, name: data}
				}

				let f = Fixture::from_entry("Test")
				f.name
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
func TestCallingFunctionValuedStructFields(t *testing.T) {
	run(t, []test{
		{
			name: "direct struct function field call",
			input: `
				struct EventContext {}
				struct Option {}

				struct Props {
					on_confirm: fn(EventContext, Option),
				}

				fn confirm(ctx: EventContext, opt: Option) {}

				fn main() {
					let p = Props{on_confirm: confirm}
					p.on_confirm(EventContext{}, Option{})
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "parenthesized struct function field call",
			input: `
				struct EventContext {}
				struct Option {}

				struct Props {
					on_confirm: fn(EventContext, Option),
				}

				fn confirm(ctx: EventContext, opt: Option) {}

				fn main() {
					let p = Props{on_confirm: confirm}
					(p.on_confirm)(EventContext{}, Option{})
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "try direct result-returning function field call",
			input: `
				struct Props {
					cb: fn() Int!Str,
				}

				fn ok() Int!Str {
					Result::ok(1)
				}

				fn main() Int!Str {
					let p = Props{cb: ok}
					let x = try p.cb()
					Result::ok(x)
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "generic function field call",
			input: `
				struct Props {
					cb: $F,
				}

				fn ok() Int!Str {
					Result::ok(1)
				}

				fn main() Int!Str {
					let p: Props<fn() Int!Str> = Props{cb: ok}
					let x = try p.cb()
					Result::ok(x)
				}
			`,
			diagnostics: []checker.Diagnostic{},
		},
	})
}
func TestInferringAnonymousFunctionTypes(t *testing.T) {
	run(t, []test{
		{
			name: "Anonymous function argument count mismatch",
			input: strings.Join(
				[]string{
					`fn process(callback: fn(Int, Str) Bool) {}`,
					`process(fn(x) { true })`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Incorrect number of arguments: Expected 2, got 1"},
			},
		},
		{
			name: "Anonymous function return type mismatch",
			input: strings.Join(
				[]string{
					`fn process(callback: fn(Int) Str) {}`,
					`process(fn(x) { 42 })`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
			},
		},
		{
			name: "Anonymous function inferred parameter types work correctly",
			input: strings.Join(
				[]string{
					`fn process(callback: fn(Str) Bool) {}`,
					`process(fn(x) { x.size() > 0 })`,
				},
				"\n",
			),
			diagnostics: []checker.Diagnostic{},
		},
	})
}

func TestCallingPackageFunctions(t *testing.T) {
	run(t, []test{
		{
			name: "Non-final side-effect match is not checked against function return type",
			input: `
			fn log(message: Str) { () }

			fn value(flag: Bool) Int {
			  match flag {
			    true => log("yes"),
			    false => log("no"),
			  }
			  1
			}
			`,
		},
		{
			name: "If branches ending in list set are Void-compatible",
			input: `
			struct Tab { id: Str }

			fn update(tabs: mut [Tab], idx: Int, id: Str) {
			  if idx == 0 {
			    tabs.set(0, Tab{id: id})
			  } else {
			    tabs.set(1, Tab{id: id})
			  }
			}
			`,
		},
	})
}

// Top-level function signatures are hoisted, so functions can be referenced
// before their declaration within a module.
func TestForwardFunctionReferences(t *testing.T) {
	run(t, []test{
		{
			name: "call a function declared later in the module",
			input: strings.Join([]string{
				`fn caller() Str {`,
				`  helper("x")`,
				`}`,
				`fn helper(v: Str) Str { v }`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "forward reference inside a closure",
			input: strings.Join([]string{
				`fn make() fn() Str {`,
				`  fn() Str { helper("x") }`,
				`}`,
				`fn helper(v: Str) Str { v }`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "forward reference with a bad argument still reports a mismatch",
			input: strings.Join([]string{
				`fn caller() Str {`,
				`  helper(1)`,
				`}`,
				`fn helper(v: Str) Str { v }`,
			}, "\n"),
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Int"},
				{Kind: checker.Error, Message: "Type mismatch: Expected Str, got Void"},
			},
		},
	})
}

func TestClosureReturnTypeInference(t *testing.T) {
	run(t, []test{
		{
			name: "closure without return annotation infers its return from the body",
			input: `
				let f = fn(x: Int) Int { x + 1 }
				let inferred = fn(x: Int) { x + 1 }
				let y: Int = inferred(1)
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "inferred closure return participates in expressions",
			input: `
				let double = fn(x: Int) { x * 2 }
				let total = double(2) + double(3)
				let text = fn(name: Str) { "hi {name}" }
				let greeting: Str = text("Ada")
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "closure with statement body stays void",
			input: `
				let noop = fn(x: Int) {
					let y = x
				}
				let bad: Int = noop(1)
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Void"},
			},
		},
		{
			name: "declared void return is not overridden by a value body",
			input: `
				fn use_void(cb: fn(Int)) {}
				use_void(fn(x: Int) { x + 1 })
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "explicit Void annotation opts out of inference and discards the value",
			input: `
				let g = fn(x: Int) Void { x + 1 }
				let bad: Int = g(1)
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected Int, got Void"},
			},
		},
		{
			name: "inferred closure value flows into fn-typed params and struct fields",
			input: `
				struct Holder {
					transform: fn(Int) Int,
				}

				fn apply(f: fn(Int) Int, x: Int) Int {
					f(x)
				}

				let inc = fn(x: Int) { x + 1 }
				let holder = Holder{transform: inc}
				let a: Int = apply(inc, 4)
				let b: Int = holder.transform(4)
			`,
			diagnostics: []checker.Diagnostic{},
		},
		{
			name: "inferred non-void closure is rejected where a void callback is expected",
			input: `
				fn run(cb: fn(Int)) {}
				let produce = fn(x: Int) { x + 1 }
				run(produce)
			`,
			diagnostics: []checker.Diagnostic{
				{Kind: checker.Error, Message: "Type mismatch: Expected fn(Int), got fn(Int) Int"},
			},
		},
	})
}
