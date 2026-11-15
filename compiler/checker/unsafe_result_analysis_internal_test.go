package checker

import (
	"slices"
	"sort"
	"testing"
)

func TestUnsafeCatchValueTypesCoversResultExpressionFamilies(t *testing.T) {
	result := func(ok, err Type) *Block {
		return unsafeCatchTestBlock(&FunctionCall{ReturnType: MakeResult(ok, err)})
	}
	first := result(Int, Str)
	second := result(Bool, Rune)
	third := result(Byte, Float64)

	tests := []struct {
		name    string
		expr    Expression
		wantOk  []string
		wantErr []string
	}{
		{
			name:    "result expression",
			expr:    &FunctionCall{ReturnType: MakeResult(Int, Str)},
			wantOk:  []string{"Int"},
			wantErr: []string{"Str"},
		},
		{
			name: "canonical result constructors",
			expr: &PatternMatch{Arms: []PatternArm{
				{Body: unsafeCatchTestBlock(unsafeCatchTestConstructor("ok", &IntLiteral{Value: 1}))},
				{Body: unsafeCatchTestBlock(unsafeCatchTestConstructor("err", &StrLiteral{Value: "error"}))},
			}},
			wantOk:  []string{"Int"},
			wantErr: []string{"Str"},
		},
		{
			name: "user result module",
			expr: &ModuleFunctionCall{
				Module: "user/result",
				Call:   &FunctionCall{Name: "ok", ReturnType: MakeResult(Bool, Rune)},
			},
			wantOk:  []string{"Bool"},
			wantErr: []string{"Rune"},
		},
		{
			name:    "block",
			expr:    first,
			wantOk:  []string{"Int"},
			wantErr: []string{"Str"},
		},
		{
			name: "if",
			expr: &If{
				Branches: []IfBranch{{Body: first}, {Body: second}},
				Else:     third,
			},
			wantOk:  []string{"Bool", "Byte", "Int"},
			wantErr: []string{"Float64", "Rune", "Str"},
		},
		{
			name:    "pattern match",
			expr:    &PatternMatch{Arms: []PatternArm{{Body: first}, {Body: second}, {Body: third}}},
			wantOk:  []string{"Bool", "Byte", "Int"},
			wantErr: []string{"Float64", "Rune", "Str"},
		},
		{
			name: "conditional match",
			expr: &ConditionalMatch{
				Cases:    []ConditionalCase{{Body: first}},
				CatchAll: second,
			},
			wantOk:  []string{"Bool", "Int"},
			wantErr: []string{"Rune", "Str"},
		},
		{
			name:    "option match",
			expr:    &OptionMatch{Some: &Match{Body: first}, None: second},
			wantOk:  []string{"Bool", "Int"},
			wantErr: []string{"Rune", "Str"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unsafeCatchValueTypes(unsafeCatchTestBlock(tt.expr))
			if diff := unsafeCatchTypeNames(got.ok); !slices.Equal(diff, tt.wantOk) {
				t.Fatalf("ok types = %v, want %v", diff, tt.wantOk)
			}
			if diff := unsafeCatchTypeNames(got.err); !slices.Equal(diff, tt.wantErr) {
				t.Fatalf("error types = %v, want %v", diff, tt.wantErr)
			}
		})
	}
}

func TestUnsafeCatchValueTypesPreservesAliasesPerPath(t *testing.T) {
	valueType := MakeResult(Int, Str)
	variable := func() *Variable {
		return &Variable{sym: Symbol{Name: "value", Type: valueType}}
	}
	block := &Block{Stmts: []Statement{
		{Stmt: &VariableDef{
			Name:  "value",
			Value: unsafeCatchTestConstructor("ok", &IntLiteral{Value: 1}),
		}},
		{Expr: &If{
			Branches: []IfBranch{{Body: &Block{Stmts: []Statement{
				{Stmt: &Reassignment{
					Target: variable(),
					Value:  unsafeCatchTestConstructor("err", &StrLiteral{Value: "error"}),
				}},
				{Expr: variable()},
			}}}},
			Else: unsafeCatchTestBlock(variable()),
		}},
	}}

	got := unsafeCatchValueTypes(block)
	if names := unsafeCatchTypeNames(got.ok); !slices.Equal(names, []string{"Int"}) {
		t.Fatalf("ok types = %v, want [Int]", names)
	}
	if names := unsafeCatchTypeNames(got.err); !slices.Equal(names, []string{"Str"}) {
		t.Fatalf("error types = %v, want [Str]", names)
	}
}

func TestUnsafeCatchValueTypesInvalidatesAliasesAfterBreak(t *testing.T) {
	valueType := MakeResult(Int, Str)
	block := &Block{Stmts: []Statement{
		{Stmt: &VariableDef{
			Name:  "value",
			Value: unsafeCatchTestConstructor("ok", &IntLiteral{Value: 1}),
		}},
		{Break: true},
		{Expr: &Variable{sym: Symbol{Name: "value", Type: valueType}}},
	}}

	got := unsafeCatchValueTypes(block)
	if names := unsafeCatchTypeNames(got.ok); !slices.Equal(names, []string{"Int"}) {
		t.Fatalf("ok types = %v, want [Int]", names)
	}
	if names := unsafeCatchTypeNames(got.err); !slices.Equal(names, []string{"Str"}) {
		t.Fatalf("error types = %v, want [Str]", names)
	}
}

func unsafeCatchTestBlock(expr Expression) *Block {
	return &Block{Stmts: []Statement{{Expr: expr}}}
}

func unsafeCatchTestConstructor(name string, arg Expression) *ModuleFunctionCall {
	return &ModuleFunctionCall{
		Module: "ard/result",
		Call: &FunctionCall{
			Name:       name,
			Args:       []Expression{arg},
			ReturnType: MakeResult(Int, Str),
		},
	}
}

func unsafeCatchTypeNames(types []Type) []string {
	names := make([]string, len(types))
	for i, typ := range types {
		names[i] = typ.String()
	}
	sort.Strings(names)
	return names
}
