package air

import (
	"strings"
	"testing"
)

func TestLowerPreservesAnyConversionOwnership(t *testing.T) {
	program := lowerSource(t, `
struct User {}

fn consume(value: Any) {}

fn pass_reference(value: mut $T) {
  consume(value)
}

fn pass_value(value: $T) {
  consume(value)
}

fn main() {
  mut user = User{}
  pass_reference(user)
  pass_value(user)
}
`)

	var findConversion func(*Expr) (*Expr, bool)
	findConversion = func(expr *Expr) (*Expr, bool) {
		if expr == nil {
			return nil, false
		}
		if expr.Kind == ExprInterfaceConversion {
			return expr, true
		}
		if conversion, ok := findConversion(expr.Target); ok {
			return conversion, true
		}
		for i := range expr.Args {
			if conversion, ok := findConversion(&expr.Args[i]); ok {
				return conversion, true
			}
		}
		return nil, false
	}

	modeFor := func(name string) InterfaceConversionMode {
		t.Helper()
		for _, fn := range program.Functions {
			if !strings.HasPrefix(fn.Name, name) {
				continue
			}
			if conversion, ok := findConversion(fn.Body.Result); ok {
				if program.Types[conversion.Type-1].Kind != TypeAny {
					t.Fatalf("%s conversion type = %v, want Any", fn.Name, program.Types[conversion.Type-1].Kind)
				}
				return conversion.InterfaceMode
			}
			for i := range fn.Body.Stmts {
				conversion, ok := findConversion(fn.Body.Stmts[i].Expr)
				if !ok {
					conversion, ok = findConversion(fn.Body.Stmts[i].Value)
				}
				if !ok {
					continue
				}
				if program.Types[conversion.Type-1].Kind != TypeAny {
					t.Fatalf("%s conversion type = %v, want Any", fn.Name, program.Types[conversion.Type-1].Kind)
				}
				return conversion.InterfaceMode
			}
		}
		t.Fatalf("interface conversion in function %s not found", name)
		return InterfaceValue
	}

	if got := modeFor("pass_reference"); got != InterfaceReference {
		t.Fatalf("reference mode = %v, want %v", got, InterfaceReference)
	}
	if got := modeFor("pass_value"); got != InterfaceValue {
		t.Fatalf("value mode = %v, want %v", got, InterfaceValue)
	}
}
