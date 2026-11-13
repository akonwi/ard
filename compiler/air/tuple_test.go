package air

import (
	"strings"
	"testing"
)

func TestLowerTupleDestructuringBindsElementsFromOneEvaluation(t *testing.T) {
	program := lowerSource(t, `
		fn pair() (Int, Str) {
			(1, "a")
		}

		fn main() {
			let (number, _) = pair()
			let copy = number
		}
	`)

	main := findFunction(t, program, "main")
	if len(main.Body.Stmts) != 3 {
		t.Fatalf("main has %d statements, want the tuple, one bound element, and copy", len(main.Body.Stmts))
	}
	source := main.Body.Stmts[0]
	if source.Kind != StmtLet || source.Value == nil || source.Value.Kind != ExprCall {
		t.Fatalf("first statement = %+v, want a let of the pair() call", source)
	}
	tupleType := program.Types[source.Type-1]
	if tupleType.Kind != TypeTuple || len(tupleType.Elems) != 2 {
		t.Fatalf("destructured value type = %+v, want a two-element tuple", tupleType)
	}
	element := main.Body.Stmts[1]
	if element.Kind != StmtLet || element.Name != "number" || element.Value == nil || element.Value.Kind != ExprTupleElem {
		t.Fatalf("second statement = %+v, want a let of tuple element 0", element)
	}
	if payload := exprPayloadAs[*FieldExprPayload](element.Value); payload == nil || payload.Field != 0 {
		t.Fatalf("tuple element payload = %+v, want index 0", payload)
	}
	if target := element.Value.Target; target == nil || target.Kind != ExprLoadLocal || exprPayloadAs[*LocalExprPayload](target).Local != source.Local {
		t.Fatalf("tuple element target = %+v, want a load of the hidden tuple local", target)
	}
	if element.Type != tupleType.Elems[0] {
		t.Fatalf("bound element type = %d, want tuple element type %d", element.Type, tupleType.Elems[0])
	}
}

func TestLowerTupleLiteralUsesCanonicalTupleType(t *testing.T) {
	program := lowerSource(t, `
		fn pair() (Int, Str) {
			(1, "a")
		}

		fn main() {
			let other: (Int, Str) = (2, "b")
		}
	`)

	pair := findFunction(t, program, "pair")
	result := pair.Body.Result
	if result == nil || result.Kind != ExprMakeTuple || len(result.Args) != 2 {
		t.Fatalf("pair result = %+v, want a two-element tuple constructor", result)
	}
	if result.Type != pair.Signature.Return {
		t.Fatalf("tuple literal type = %d, want the declared return type %d", result.Type, pair.Signature.Return)
	}
	main := findFunction(t, program, "main")
	if got := main.Body.Stmts[0].Type; got != pair.Signature.Return {
		t.Fatalf("annotated tuple type = %d, want the same canonical type %d", got, pair.Signature.Return)
	}
	if name := program.Types[result.Type-1].Name; name != "(Int,Str)" {
		t.Fatalf("tuple type name = %q, want (Int,Str)", name)
	}
}

func TestValidateRejectsMalformedTuples(t *testing.T) {
	base := func() *Program {
		return lowerSource(t, `
			fn pair() (Int, Str) {
				(1, "a")
			}

			fn main() {
				let (number, _) = pair()
			}
		`)
	}
	tupleTypeIndex := func(program *Program) int {
		for i, typ := range program.Types {
			if typ.Kind == TypeTuple {
				return i
			}
		}
		t.Fatal("lowered program has no tuple type")
		return -1
	}

	tests := []struct {
		name    string
		mutate  func(*Program)
		wantErr string
	}{
		{
			name: "one-element tuple type",
			mutate: func(program *Program) {
				index := tupleTypeIndex(program)
				program.Types[index].Elems = program.Types[index].Elems[:1]
			},
			wantErr: "want at least 2",
		},
		{
			name: "constructor with a missing element",
			mutate: func(program *Program) {
				pair := findFunction(t, program, "pair")
				pair.Body.Result.Args = pair.Body.Result.Args[:1]
			},
			wantErr: "tuple constructor has 1 elements, want 2",
		},
		{
			name: "constructor element of the wrong type",
			mutate: func(program *Program) {
				pair := findFunction(t, program, "pair")
				pair.Body.Result.Args[1] = pair.Body.Result.Args[0]
			},
			wantErr: "tuple constructor element 1 has type",
		},
		{
			name: "element access out of range",
			mutate: func(program *Program) {
				main := findFunction(t, program, "main")
				main.Body.Stmts[1].Value.Payload = &FieldExprPayload{Field: 2}
			},
			wantErr: "tuple element access has invalid index 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := base()
			tt.mutate(program)
			err := Validate(program)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
