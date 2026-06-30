package checker

import "testing"

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
	if got, want := MakeMaybe(functionType).String(), "(fn(Int) Void)?"; got != want {
		t.Fatalf("function maybe string = %q, want %q", got, want)
	}

	if got, want := MakeMaybe(MakeResult(Int, Str)).String(), "(Int!Str)?"; got != want {
		t.Fatalf("result maybe string = %q, want %q", got, want)
	}
	if got, want := MakeMaybe(MakeMutableRef(Int)).String(), "(mut Int)?"; got != want {
		t.Fatalf("mutable reference maybe string = %q, want %q", got, want)
	}
	if got, want := MakeMaybe(MakeResult(functionType, Str)).String(), "((fn(Int) Void)!Str)?"; got != want {
		t.Fatalf("function result maybe string = %q, want %q", got, want)
	}
	nestedFunctionType := &FunctionDef{
		Name:       "<function>",
		Parameters: []Parameter{{Name: "callback", Type: functionType, Mutable: true}},
		ReturnType: functionType,
	}
	if got, want := MakeMaybe(nestedFunctionType).String(), "(fn(mut fn(Int) Void) fn(Int) Void)?"; got != want {
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
