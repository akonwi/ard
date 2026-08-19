package checker

import "testing"

func TestGenericBindingRejectsStructuralOccursCycle(t *testing.T) {
	root := makeScope(nil)
	scope := root.createGenericScope([]string{"T"})
	typeVar := (*scope.genericContext)["T"]

	err := scope.bindGeneric("T", MakeList(typeVar))
	if _, ok := err.(*genericOccursCheckError); !ok {
		t.Fatalf("bind error = %T %v, want genericOccursCheckError", err, err)
	}
	if typeVar.bound || typeVar.actual != nil {
		t.Fatalf("cyclic binding mutated type variable: %+v", typeVar)
	}
}

func TestBindInferredTypeVarsTerminatesAfterRecursiveFieldBinding(t *testing.T) {
	inferred := &TypeVar{name: "T"}
	expected := &StructDef{Name: "Node"}
	actual := &StructDef{Name: "Node"}
	expected.Fields = map[string]Type{
		"children": MakeList(expected),
		"value":    inferred,
	}
	actual.Fields = map[string]Type{
		"children": MakeList(actual),
		"value":    Int,
	}

	bindInferredTypeVars(expected, actual)
	// A second traversal starts with the recursive field variable bound. It must
	// retain stable graph identities and terminate without rebuilding the cycle.
	bindInferredTypeVars(expected, actual)

	if inferred.actual != Int || !inferred.bound {
		t.Fatalf("recursive field binding = %+v, want Int", inferred)
	}
}

func TestBindInferredTypeVarsVisitsTypeArgsBeforeSharedDefinitionGuard(t *testing.T) {
	definition := &StructDef{Name: "Box", GenericParams: []string{"T"}}
	first := &TypeVar{name: "First"}
	second := &TypeVar{name: "Second"}
	expected := &StructDef{Name: "Pair", Fields: map[string]Type{
		"first":  newStructApplication(definition, []Type{first}),
		"second": newStructApplication(definition, []Type{second}),
	}}
	actual := &StructDef{Name: "Pair", Fields: map[string]Type{
		"first":  newStructApplication(definition, []Type{Int}),
		"second": newStructApplication(definition, []Type{Str}),
	}}

	bindInferredTypeVars(expected, actual)

	if first.actual != Int || !first.bound {
		t.Fatalf("first type argument binding = %+v, want Int", first)
	}
	if second.actual != Str || !second.bound {
		t.Fatalf("second type argument binding = %+v, want Str", second)
	}
}

func TestTraitEqualityIncludesModulePath(t *testing.T) {
	left := &Trait{Name: "Drawable", ModulePath: "ui/drawable.ard"}
	right := &Trait{Name: "Drawable", ModulePath: "svg/drawable.ard"}
	if equalTypes(left, right) {
		t.Fatal("same-named traits from different modules should not be equal")
	}
}
func TestUnresolvedTypeVarGetReturnsNil(t *testing.T) {
	unknown := &TypeVar{name: "unknown"}

	if got := unknown.get("to_str"); got != nil {
		t.Fatalf("unresolved TypeVar.get() = %v, want nil", got)
	}
}
func TestMaybeStringParenthesizesCompositeTypes(t *testing.T) {
	functionType := &FunctionDef{
		Name: "<function>",
		Parameters: []Parameter{
			{Name: "arg0", Type: Int},
		},
		ReturnType: Void,
	}
	if got, want := MakeMaybe(functionType).String(), "(fn(Int))?"; got != want {
		t.Fatalf("function maybe string = %q, want %q", got, want)
	}

	if got, want := MakeMaybe(MakeResult(Int, Str)).String(), "(Int!Str)?"; got != want {
		t.Fatalf("result maybe string = %q, want %q", got, want)
	}
	if got, want := MakeMaybe(MakeMutableRef(Int)).String(), "(mut Int)?"; got != want {
		t.Fatalf("mutable reference maybe string = %q, want %q", got, want)
	}
	if got, want := MakeMaybe(MakeResult(functionType, Str)).String(), "((fn(Int))!Str)?"; got != want {
		t.Fatalf("function result maybe string = %q, want %q", got, want)
	}
	nestedFunctionType := &FunctionDef{
		Name:       "<function>",
		Parameters: []Parameter{{Name: "callback", Type: functionType, Mutable: true}},
		ReturnType: functionType,
	}
	if got, want := MakeMaybe(nestedFunctionType).String(), "(fn(mut fn(Int)) fn(Int))?"; got != want {
		t.Fatalf("nested function maybe string = %q, want %q", got, want)
	}
}
func TestTypeEquality(t *testing.T) {
	var tests = []struct {
		left   Type
		right  Type
		expect bool
	}{
		{
			left:   &TypeVar{name: "T"},
			right:  Str,
			expect: true,
		},
		{
			left:   Str,
			right:  &TypeVar{name: "T"},
			expect: true,
		},
		{
			left:   MakeResult(&TypeVar{name: "T"}, Void),
			right:  MakeResult(Str, Void),
			expect: true,
		},
		{
			left:   MakeResult(Str, Void),
			right:  MakeResult(&TypeVar{name: "T"}, Void),
			expect: true,
		},
		{
			left: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(Str, Void),
			},
			right: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(&TypeVar{name: "T"}, Void),
			},
			expect: true,
		},
		{
			left: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(Str, Void),
			},
			right: &FunctionDef{
				Parameters: []Parameter{},
				ReturnType: MakeResult(&TypeVar{name: "T"}, Void),
			},
			expect: true,
		},
	}

	for _, test := range tests {
		got := test.left.equal(test.right)
		if got != test.expect {
			t.Errorf("%s == %s: got %v, want %v", test.left, test.right, got, test.expect)
		}
	}
}

// Diagnostics render types in formatter-canonical Ard syntax: parameters are
// comma-space separated and map entries colon-space separated, matching the
// formatter's output (`ard format` emits `[Str: Int]` for map annotations).
func TestTypeRenderingIsFormatterCanonical(t *testing.T) {
	fn := &FunctionDef{
		Name: "<function>",
		Parameters: []Parameter{
			{Name: "a", Type: Str},
			{Name: "b", Type: Int, Mutable: true},
		},
		ReturnType: Bool,
	}
	if got, want := fn.String(), "fn(Str, mut Int) Bool"; got != want {
		t.Fatalf("function type = %q, want %q", got, want)
	}
	if got, want := MakeMap(Str, Int).String(), "[Str: Int]"; got != want {
		t.Fatalf("map type = %q, want %q", got, want)
	}
	if got, want := typeSyntaxString(MakeMap(Str, MakeList(Int))), "[Str: [Int]]"; got != want {
		t.Fatalf("map syntax = %q, want %q", got, want)
	}
}
