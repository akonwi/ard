package checker

import (
	"testing"

	"github.com/akonwi/ard/parse"
)

func checkGenericContextSource(t *testing.T, source string) *Checker {
	t.Helper()
	result := parse.Parse([]byte(source), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	checker := New("test.ard", result.Program, nil)
	checker.Check()
	return checker
}

func TestAnonymousClosuresInheritEnclosingGenericContext(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "contextual closure in generic receiver method",
			source: `
				fn identity(value: $T) $T { value }

				struct Box<$T> { value: $T }

				impl Box {
					fn callback() fn() {
						fn() {
							let value = identity<$T>(self.value)
							let _ = value
						}
					}
				}
			`,
		},
		{
			name: "explicit receiver generic absent from fields",
			source: `
				fn empty() $T? { Maybe::new() }

				struct Marker<$T> {}

				impl Marker {
					fn callback() fn() {
						fn() {
							let value = empty<$T>()
							let _ = value
						}
					}
				}
			`,
		},
		{
			name: "local closure in generic function",
			source: `
				fn identity(value: $T) $T { value }

				fn invoke(value: $T) {
					let callback = fn() {
						let copy = identity<$T>(value)
						let _ = copy
					}
					callback()
				}
			`,
		},
		{
			name: "nested anonymous closures",
			source: `
				fn identity(value: $T) $T { value }

				fn callback(value: $T) fn() fn() {
					fn() fn() {
						fn() {
							let copy = identity<$T>(value)
							let _ = copy
						}
					}
				}
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := checkGenericContextSource(t, test.source)
			if checker.HasErrors() {
				t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
			}
		})
	}
}

func TestAnonymousClosureDoesNotExposeContextualCallGenericAsExplicitTypeArg(t *testing.T) {
	checker := checkGenericContextSource(t, `
		fn identity(value: $T) $T { value }
		fn consume(callback: fn($T)) {}

		fn main() {
			consume(fn(value) {
				let copy = identity<$T>(value)
				let _ = copy
			})
		}
	`)

	for _, diagnostic := range checker.Diagnostics() {
		if diagnostic.Code == DiagnosticCodeUnboundGenericTypeArg {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want unbound generic type argument", checker.Diagnostics())
}

func TestAnonymousClosurePrefersOuterGenericOverSameNamedContextualGeneric(t *testing.T) {
	checker := checkGenericContextSource(t, `
		fn identity(value: $T) $T { value }
		fn consume(callback: fn($T)) {}

		fn outer(value: $T) {
			consume(fn(_contextual) {
				let copy = identity<$T>(value)
				let _ = copy
			})
		}
	`)

	diagnostics := checker.Diagnostics()
	if len(diagnostics) != 1 || diagnostics[0].Code != DiagnosticCodeUnresolvedGeneric {
		t.Fatalf("diagnostics = %v, want only the unresolved consume generic", diagnostics)
	}
}

func TestAnonymousClosureExplicitTypeArgUsesOuterGenericInsteadOfContextualGeneric(t *testing.T) {
	checker := checkGenericContextSource(t, `
		fn identity(value: $T) $T { value }
		fn consume(callback: fn($T) $T, seed: $T) $T { callback(seed) }

		fn outer(value: $T) Int {
			consume(fn(_contextual) Int {
				let copy = identity<$T>(value)
				let _ = copy
				1
			}, 1)
		}
	`)
	if checker.HasErrors() {
		t.Fatalf("checker diagnostics: %v", checker.Diagnostics())
	}

	var outer *FunctionDef
	for _, statement := range checker.program.Statements {
		if function, ok := statement.Expr.(*FunctionDef); ok && function.Name == "outer" {
			outer = function
			break
		}
	}
	if outer == nil {
		t.Fatal("outer declaration not found")
	}
	outerGeneric, ok := outer.Parameters[0].Type.(*TypeVar)
	if !ok {
		t.Fatalf("outer parameter type = %T, want declaration TypeVar", outer.Parameters[0].Type)
	}
	consumeCall, ok := outer.Body.Stmts[0].Expr.(*FunctionCall)
	if !ok {
		t.Fatalf("outer body expression = %T, want FunctionCall", outer.Body.Stmts[0].Expr)
	}
	closure, ok := consumeCall.Args[0].(*FunctionDef)
	if !ok {
		t.Fatalf("consume callback = %T, want FunctionDef", consumeCall.Args[0])
	}
	if len(closure.CallGenericParams) != 0 {
		t.Fatalf("closure call generics = %v, want inherited generics to remain outer-owned", closure.CallGenericParams)
	}
	binding, ok := closure.Body.Stmts[0].Stmt.(*VariableDef)
	if !ok {
		t.Fatalf("closure statement = %T, want VariableDef", closure.Body.Stmts[0].Stmt)
	}
	identityCall, ok := binding.Value.(*FunctionCall)
	if !ok {
		t.Fatalf("binding value = %T, want FunctionCall", binding.Value)
	}
	explicitGeneric, ok := identityCall.TypeArgs[0].(*TypeVar)
	if !ok {
		t.Fatalf("explicit type argument = %T, want TypeVar", identityCall.TypeArgs[0])
	}
	if explicitGeneric != outerGeneric {
		t.Fatalf("explicit type argument = %p, want outer declaration generic %p", explicitGeneric, outerGeneric)
	}
	if explicitGeneric.owner != 0 || explicitGeneric.provisional {
		t.Fatalf("explicit type argument owner = %d provisional = %t, want declaration-owned", explicitGeneric.owner, explicitGeneric.provisional)
	}
}

func TestAnonymousClosureExplicitTypeArgDoesNotCrossNamedFunctionGenericBoundary(t *testing.T) {
	checker := checkGenericContextSource(t, `
		fn identity(value: $T) $T { value }

		fn outer(value: $T) {
			fn inner() {
				let copy = identity<$T>(value)
				let _ = copy
			}
			inner()
		}
	`)

	for _, diagnostic := range checker.Diagnostics() {
		if diagnostic.Code == DiagnosticCodeUnboundGenericTypeArg {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want named function boundary to reject outer generic", checker.Diagnostics())
}

func TestFindDeclarationGenericIgnoresInferenceVariables(t *testing.T) {
	declaration := &TypeVar{name: "T"}
	outer := makeScope(nil)
	outer.add("declaration", declaration, false)

	tests := []struct {
		name      string
		inference *TypeVar
	}{
		{name: "call-owned", inference: &TypeVar{name: "T", owner: 1}},
		{name: "provisional", inference: &TypeVar{name: "T", provisional: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inner := makeScope(&outer)
			inner.add("inference", test.inference, false)
			if got := inner.findDeclarationGeneric("T"); got != declaration {
				t.Fatalf("findDeclarationGeneric(T) = %p, want declaration generic %p", got, declaration)
			}
		})
	}
}
