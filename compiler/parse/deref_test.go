package parse

import (
	"fmt"
	"testing"
)

func TestPostfixDerefExpressionPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "identifier", source: "reference.@", want: "deref(reference)"},
		{name: "deref then field", source: "reference.@.field", want: "field(deref(reference),field)"},
		{name: "field then deref", source: "reference.field.@", want: "deref(field(reference,field))"},
		{name: "binary binds less tightly", source: "reference.@ + value", want: "add(deref(reference),value)"},
		{name: "not remains broad", source: "not reference.@ == value", want: "not(equal(deref(reference),value))"},
		{name: "borrow then deref", source: "(mut value).@", want: "deref(mut(value))"},
		{name: "deref then borrow", source: "mut reference.@", want: "mut(deref(reference))"},
		{name: "repeated deref", source: "reference.@.@", want: "deref(deref(reference))"},
		{name: "call then deref", source: "load().@", want: "deref(call(load))"},
		{name: "deref then call", source: "reference.@()", want: "invoke(deref(reference))"},
		{name: "deref call then field", source: "reference.@().field", want: "field(invoke(deref(reference)),field)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte("let result = "+tt.source+"\n"), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			if len(result.Program.Statements) != 1 {
				t.Fatalf("statement count = %d, want 1", len(result.Program.Statements))
			}
			declaration, ok := result.Program.Statements[0].(*VariableDeclaration)
			if !ok {
				t.Fatalf("statement = %T, want VariableDeclaration", result.Program.Statements[0])
			}
			if got := derefExpressionShape(t, declaration.Value); got != tt.want {
				t.Fatalf("shape = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPostfixDerefLocationsIncludeOperator(t *testing.T) {
	result := Parse([]byte("let result = reference.@\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	dereference := result.Program.Statements[0].(*VariableDeclaration).Value.(*Deref)
	if want := (Location{Start: Point{Row: 1, Col: 14}, End: Point{Row: 1, Col: 24}}); dereference.Location != want {
		t.Fatalf("location = %#v, want %#v", dereference.Location, want)
	}
	if want := (Location{Start: Point{Row: 1, Col: 23}, End: Point{Row: 1, Col: 24}}); dereference.OperatorLocation != want {
		t.Fatalf("operator location = %#v, want %#v", dereference.OperatorLocation, want)
	}
}

func TestPostfixDerefSupportsDotLeadingChain(t *testing.T) {
	result := Parse([]byte("let result = reference\n  .@\n  .field\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*VariableDeclaration)
	if got := derefExpressionShape(t, declaration.Value); got != "field(deref(reference),field)" {
		t.Fatalf("shape = %q", got)
	}
}

func TestPostfixCallsStayOnTheSameSourceLine(t *testing.T) {
	sameLine := Parse([]byte("reference.@ ()\n"), "test.ard")
	if len(sameLine.Errors) > 0 {
		t.Fatalf("same-line parse errors: %v", sameLine.Errors)
	}
	if len(sameLine.Program.Statements) != 1 {
		t.Fatalf("same-line statement count = %d, want 1", len(sameLine.Program.Statements))
	}
	if got := derefExpressionShape(t, sameLine.Program.Statements[0]); got != "invoke(deref(reference))" {
		t.Fatalf("same-line shape = %q", got)
	}

	separateLines := Parse([]byte("reference.@\n()\n"), "test.ard")
	if len(separateLines.Errors) > 0 {
		t.Fatalf("separate-line parse errors: %v", separateLines.Errors)
	}
	if len(separateLines.Program.Statements) != 2 {
		t.Fatalf("separate-line statement count = %d, want 2", len(separateLines.Program.Statements))
	}
	if got := derefExpressionShape(t, separateLines.Program.Statements[0]); got != "deref(reference)" {
		t.Fatalf("first separate-line shape = %q", got)
	}
}

func TestPostfixDerefRequiresAdjacentDotAndAt(t *testing.T) {
	for _, source := range []string{
		"let result = reference. @\n",
		"let result = reference@\n",
		"let result = @reference\n",
	} {
		result := Parse([]byte(source), "test.ard")
		if len(result.Errors) == 0 {
			t.Fatalf("expected %q to be rejected", source)
		}
	}
}

func TestLegacyDerefSyntaxRemainsParsedForMigration(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: "deref reference", want: "deref(reference)"},
		{source: "deref reference.field", want: "deref(field(reference,field))"},
		{source: "(deref reference).field", want: "field(deref(reference),field)"},
		{source: "deref mut value", want: "deref(mut(value))"},
		{source: "mut deref reference", want: "mut(deref(reference))"},
		{source: "deref deref reference", want: "deref(deref(reference))"},
		{source: "deref load()", want: "deref(call(load))"},
	}

	for _, tt := range tests {
		result := Parse([]byte("let result = "+tt.source+"\n"), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse %q: %v", tt.source, result.Errors)
		}
		declaration := result.Program.Statements[0].(*VariableDeclaration)
		if got := derefExpressionShape(t, declaration.Value); got != tt.want {
			t.Fatalf("shape for %q = %q, want %q", tt.source, got, tt.want)
		}
		dereference := findDeref(declaration.Value)
		if dereference == nil || !dereference.LegacyPrefix {
			t.Fatalf("%q did not retain legacy-prefix metadata: %#v", tt.source, dereference)
		}
		if tt.source == "deref reference" {
			want := Location{Start: Point{Row: 1, Col: 14}, End: Point{Row: 1, Col: 18}}
			if dereference.OperatorLocation != want {
				t.Fatalf("legacy operator location = %#v, want %#v", dereference.OperatorLocation, want)
			}
		}
	}
}

func TestDerefKeywordRemainsReservedDuringMigration(t *testing.T) {
	result := Parse([]byte("let deref = 1\n"), "test.ard")
	if len(result.Errors) == 0 {
		t.Fatal("expected deref to remain reserved as a binding name")
	}

	for _, source := range []string{
		"reader.deref()\n",
		"Type::deref(value)\n",
		"impl Value {\n  fn deref() Int { 1 }\n}\n",
	} {
		result := Parse([]byte(source), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("expected member-position deref in %q to parse: %v", source, result.Errors)
		}
	}
}

func derefExpressionShape(t *testing.T, expression Expression) string {
	t.Helper()
	switch value := expression.(type) {
	case *Identifier:
		return value.Name
	case *MutRef:
		return "mut(" + derefExpressionShape(t, value.Operand) + ")"
	case *Deref:
		return "deref(" + derefExpressionShape(t, value.Operand) + ")"
	case *InstanceProperty:
		return "field(" + derefExpressionShape(t, value.Target) + "," + value.Property.Name + ")"
	case *FunctionCall:
		return "call(" + value.Name + ")"
	case *FunctionValueCall:
		return "invoke(" + derefExpressionShape(t, value.Callee) + ")"
	case *BinaryExpression:
		operator := fmt.Sprint(value.Operator)
		switch value.Operator {
		case Plus:
			operator = "add"
		case Equal:
			operator = "equal"
		}
		return operator + "(" + derefExpressionShape(t, value.Left) + "," + derefExpressionShape(t, value.Right) + ")"
	case *UnaryExpression:
		operator := fmt.Sprint(value.Operator)
		if value.Operator == Not {
			operator = "not"
		}
		return operator + "(" + derefExpressionShape(t, value.Operand) + ")"
	default:
		t.Fatalf("unexpected expression %T", expression)
		return ""
	}
}

func findDeref(expression Expression) *Deref {
	switch value := expression.(type) {
	case *Deref:
		return value
	case *MutRef:
		return findDeref(value.Operand)
	case *InstanceProperty:
		return findDeref(value.Target)
	default:
		return nil
	}
}
