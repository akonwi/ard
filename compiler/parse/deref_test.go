package parse

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestDerefExpressionPrecedence locks ADR 0057's source-level dereference
// trees before the syntax implementation lands. Reflection keeps this red test
// buildable until parse.Deref is introduced in Phase 2.
func TestDerefLexesAsKeyword(t *testing.T) {
	tokens := NewLexer([]byte("deref value")).Scan()
	if len(tokens) < 2 {
		t.Fatalf("tokens = %#v", tokens)
	}
	if tokens[0].kind == identifier {
		t.Fatalf("first token = %#v, want reserved deref keyword", tokens[0])
	}
}

func TestDerefExpressionPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "identifier", source: "deref reference", want: "deref(reference)"},
		{name: "postfix binds tighter", source: "deref reference.field", want: "deref(field(reference,field))"},
		{name: "parentheses select from result", source: "(deref reference).field", want: "field(deref(reference),field)"},
		{name: "binary binds less tightly", source: "deref reference + value", want: "add(deref(reference),value)"},
		{name: "not remains broad", source: "not deref reference == value", want: "not(equal(deref(reference),value))"},
		{name: "deref composes with mut", source: "deref mut value", want: "deref(mut(value))"},
		{name: "mut composes with deref", source: "mut deref reference", want: "mut(deref(reference))"},
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

func TestDerefKeywordIsReservedOnlyInExpressionPosition(t *testing.T) {
	result := Parse([]byte("let deref = 1\n"), "test.ard")
	if len(result.Errors) == 0 {
		t.Fatal("expected deref to be reserved as a binding name")
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
	case *InstanceProperty:
		return "field(" + derefExpressionShape(t, value.Target) + "," + value.Property.Name + ")"
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
	}

	if text := fmt.Sprint(expression); strings.HasPrefix(text, "deref ") || strings.HasPrefix(text, "(deref ") {
		children := directParseExpressionChildren(reflect.ValueOf(expression))
		if len(children) != 1 {
			t.Fatalf("dereference expression %T has %d direct expression children, want 1", expression, len(children))
		}
		return "deref(" + derefExpressionShape(t, children[0]) + ")"
	}

	t.Fatalf("unexpected expression %T", expression)
	return ""
}

func directParseExpressionChildren(value reflect.Value) []Expression {
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil
	}
	children := []Expression{}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if !field.CanInterface() {
			continue
		}
		// The embedded source span satisfies Expression's method set but is
		// positional metadata, not an operand.
		if field.Type() == reflect.TypeOf(Location{}) {
			continue
		}
		if child, ok := field.Interface().(Expression); ok {
			children = append(children, child)
		}
	}
	return children
}
