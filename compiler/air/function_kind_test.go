package air

import (
	"strings"
	"testing"
)

// Function.Kind tells a declared function apart from the helpers lowering
// lifts out of other bodies, which share Program.Functions and
// Module.Functions with it.
func TestLowerClassifiesFunctionKinds(t *testing.T) {
	program := lowerSourceWithTests(t, `
		let doubler: fn(Int) Int = fn(x: Int) Int { x * 2 }

		struct Counter { n: Int }

		fn Counter::new() Counter { Counter{n: 0} }

		trait Speaks {
			fn speak() Str
		}

		impl Speaks for Counter {
			fn speak() Str { "count" }
		}

		fn describe(speaker: Speaks) Str { speaker.speak() }

		impl Counter {
			fn add(k: Int) [Int] {
				List::map([1, 2], fn(v: Int) Int { v + k })
			}
		}

		fn anon_func_1_1(x: Int) Int { x + 1 }

		fn ident(x: $T) $T { x }

		fn outer(value: Int) Int {
			fn inner() Int { value }
			fn plain(x: Int) Int { x }
			inner() + plain(1)
		}

		fn make_adder(k: Int) fn(Int) Int {
			fn(x: Int) Int { x + k }
		}

		fn main() {
			let c = Counter::new()
			let _ = describe(c)
			let _ = c.add(1)
			let _ = anon_func_1_1(1)
			let _ = ident(4)
			let _ = outer(1)
			let _ = make_adder(1)
			let _ = doubler(1)
		}

		test fn checks() Void!Str {
			let h = fn(x: Int) Int { x }
			if h(1) == 1 { Result::ok(()) } else { Result::err("bad") }
		}
	`)

	want := map[string]FunctionKind{
		"main":                 FunctionDeclared,
		"anon_func_1_1":        FunctionDeclared,
		"ident":                FunctionDeclared,
		"outer":                FunctionDeclared,
		"make_adder":           FunctionDeclared,
		"checks":               FunctionDeclared,
		"Counter::new":         FunctionDeclared,
		"describe":             FunctionDeclared,
		"Counter.add":          FunctionMethod,
		"Counter.Speaks.speak": FunctionMethod,
		"inner":                FunctionNested,
		"plain":                FunctionNested,
	}
	for name, kind := range want {
		if got := findFunction(t, program, name).Kind; got != kind {
			t.Errorf("%s kind = %v, want %v", name, got, kind)
		}
	}

	closures := 0
	for _, fn := range program.Functions {
		if fn.Name == "anon_func_1_1" || !strings.HasPrefix(fn.Name, "anon_func_") {
			continue
		}
		closures++
		if fn.Kind != FunctionClosure {
			t.Errorf("%s kind = %v, want %v", fn.Name, fn.Kind, FunctionClosure)
		}
	}
	// The global initializer, the method body, make_adder, and the test each
	// contain one anonymous function.
	if closures != 4 {
		t.Fatalf("found %d anonymous functions, want 4", closures)
	}

	for _, fn := range program.Functions {
		if fn.Receiver != NoType && fn.Kind != FunctionMethod {
			t.Errorf("%s has receiver but kind %v, want %v", fn.Name, fn.Kind, FunctionMethod)
		}
	}
}

func TestValidateRejectsInconsistentFunctionKind(t *testing.T) {
	base := func(fn Function) *Program {
		return &Program{
			Modules:   []Module{{ID: 0, Path: "test", Functions: []FunctionID{0}}},
			Types:     []TypeInfo{{ID: 1, Kind: TypeInt, Name: "Int"}, {ID: 2, Kind: TypeVoid, Name: "Void"}},
			Functions: []Function{fn},
			Entry:     NoFunction,
		}
	}
	cases := []struct {
		name string
		fn   Function
		want string
	}{
		{
			name: "method without receiver",
			fn:   Function{ID: 0, Name: "m", Kind: FunctionMethod, Signature: Signature{Return: 2}},
			want: "method",
		},
		{
			name: "receiver on a non-method",
			fn:   Function{ID: 0, Name: "f", Kind: FunctionDeclared, Receiver: 1, Signature: Signature{Return: 2}},
			want: "receiver",
		},
		{
			name: "captures on a declared function",
			fn: Function{
				ID:        0,
				Name:      "f",
				Kind:      FunctionDeclared,
				Signature: Signature{Return: 2},
				Locals:    []Local{{ID: 0, Name: "x", Type: 1}},
				Captures:  []Capture{{Name: "x", Type: 1, Local: 0}},
			},
			want: "captures",
		},
		{
			name: "unknown kind",
			fn:   Function{ID: 0, Name: "f", Kind: FunctionKind(99), Signature: Signature{Return: 2}},
			want: "kind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(base(tc.fn))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate error = %v, want error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsClosureValueFromDeclaredFunction(t *testing.T) {
	program := &Program{
		Modules: []Module{{ID: 0, Path: "test", Functions: []FunctionID{0, 1}}},
		Types: []TypeInfo{
			{ID: 1, Kind: TypeInt, Name: "Int"},
			{ID: 2, Kind: TypeFunction, Name: "fn() Int", Return: 1},
		},
		Functions: []Function{
			{
				ID:        0,
				Name:      "make",
				Signature: Signature{Return: 2},
				Body: Block{Result: &Expr{
					Kind:    ExprMakeClosure,
					Type:    2,
					Payload: &CallExprPayload{Function: 1},
				}},
			},
			{
				ID:        1,
				Name:      "helper",
				Kind:      FunctionDeclared,
				Signature: Signature{Return: 1},
				Body:      Block{Result: &Expr{Kind: ExprConstInt, Type: 1, Payload: &TextExprPayload{Value: "1"}}},
			},
		},
		Entry: NoFunction,
	}
	if err := Validate(program); err == nil || !strings.Contains(err.Error(), "closure value made from declared function helper") {
		t.Fatalf("Validate error = %v, want closure-from-declared-function error", err)
	}
	program.Functions[1].Kind = FunctionClosure
	if err := Validate(program); err != nil {
		t.Fatalf("Validate with closure kind: %v", err)
	}
}
