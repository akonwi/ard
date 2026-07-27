package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func TestAnyConversionRecordsSourceOwnership(t *testing.T) {
	result := parse.Parse([]byte(`
fn consume(value: Any) {}

fn pass_reference(value: mut $T) {
  consume(value)
}

fn pass_value(value: $T) {
  consume(value)
}

fn consume_maybe(value: Any?) {}

fn pass_maybe_reference(value: mut $T) {
  consume_maybe(value)
}

struct User {}

fn infer_maybe_reference(value: $T?) {}

fn main() {
  mut user = User{}
  infer_maybe_reference(mut user)
}
`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	conversionFor := func(name string) *InterfaceConversion {
		t.Helper()
		for _, statement := range checker.program.Statements {
			fn, ok := statement.Expr.(*FunctionDef)
			if !ok || fn.Name != name || fn.Body == nil || len(fn.Body.Stmts) == 0 {
				continue
			}
			call, ok := fn.Body.Stmts[0].Expr.(*FunctionCall)
			if !ok || len(call.Args) != 1 {
				t.Fatalf("%s body = %#v, want one-argument function call", name, fn.Body.Stmts[0].Expr)
			}
			conversion, ok := call.Args[0].(*InterfaceConversion)
			if !ok {
				t.Fatalf("%s argument = %T, want *InterfaceConversion", name, call.Args[0])
			}
			return conversion
		}
		t.Fatalf("function %s not found", name)
		return nil
	}

	reference := conversionFor("pass_reference")
	if reference.Destination != Any || reference.Mode != InterfaceReference {
		t.Fatalf("reference conversion = %#v, want Any reference mode", reference)
	}
	value := conversionFor("pass_value")
	if value.Destination != Any || value.Mode != InterfaceValue {
		t.Fatalf("value conversion = %#v, want Any value mode", value)
	}

	foundMaybeConversion := false
	for _, statement := range checker.program.Statements {
		fn, ok := statement.Expr.(*FunctionDef)
		if !ok {
			continue
		}
		if fn.Name == "pass_maybe_reference" {
			call, ok := fn.Body.Stmts[0].Expr.(*FunctionCall)
			if !ok || len(call.Args) != 1 {
				t.Fatalf("pass_maybe_reference body = %#v", fn.Body.Stmts[0].Expr)
			}
			maybeCall, ok := call.Args[0].(*ModuleFunctionCall)
			if !ok || len(maybeCall.Call.Args) != 1 {
				t.Fatalf("Maybe argument = %T, want synthesized Maybe call", call.Args[0])
			}
			conversion, ok := maybeCall.Call.Args[0].(*InterfaceConversion)
			if !ok || conversion.Mode != InterfaceReference || conversion.Destination != Any {
				t.Fatalf("Maybe payload conversion = %#v, want Any reference mode", maybeCall.Call.Args[0])
			}
			foundMaybeConversion = true
		}
		if fn.Name == "main" {
			call, ok := fn.Body.Stmts[1].Expr.(*FunctionCall)
			if !ok {
				t.Fatalf("main call = %T, want FunctionCall", fn.Body.Stmts[1].Expr)
			}
			binding := call.Definition().GenericBindings["T"]
			if _, ok := binding.(*MutableRef); !ok {
				t.Fatalf("inferred Maybe generic = %T %v, want mutable reference", binding, binding)
			}
		}
	}
	if !foundMaybeConversion {
		t.Fatal("pass_maybe_reference not found")
	}
}
