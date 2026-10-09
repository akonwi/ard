package parse

import (
	"testing"
)

// ADR 0073: pointer types, address-of expressions, and postfix `.*`
// dereference.

func TestPointerTypeAnnotations(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		want string
	}{
		{name: "read-only pointer", typ: "&User", want: "ptr(User)"},
		{name: "writable pointer", typ: "&mut User", want: "ptrmut(User)"},
		{name: "pointer to nullable", typ: "&User?", want: "ptr(User?)"},
		{name: "nullable pointer", typ: "(&User)?", want: "ptr(User)?"},
		{name: "pointer to list", typ: "&mut [Int]", want: "ptrmut([Int])"},
		{name: "pointer to generic", typ: "&$T", want: "ptr($T)"},
		{name: "pointer to foreign type", typ: "&mut http::Request", want: "ptrmut(http::Request)"},
		{name: "list of pointers", typ: "[&mut User]", want: "[ptrmut(User)]"},
		{name: "legacy mutable type", typ: "mut User", want: "mut(User)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte("let value: "+tt.typ+" = other\n"), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			declaration, ok := result.Program.Statements[0].(*VariableDeclaration)
			if !ok {
				t.Fatalf("statement = %T, want VariableDeclaration", result.Program.Statements[0])
			}
			if got := pointerTypeShape(t, declaration.Type); got != tt.want {
				t.Fatalf("shape = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPointerTypesInSignaturesAndFields(t *testing.T) {
	result := Parse([]byte(`struct Node {
  value: Int,
  parent: &Node,
}

fn rename(user: &mut User, name: Str) &User {
  user
}

let callback: fn(&mut User) &User = rename
`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}

	node := result.Program.Statements[0].(*StructDefinition)
	if got := pointerTypeShape(t, node.Fields[1].Type); got != "ptr(Node)" {
		t.Fatalf("field shape = %q", got)
	}

	fn := result.Program.Statements[1].(*FunctionDeclaration)
	if got := pointerTypeShape(t, fn.Parameters[0].Type); got != "ptrmut(User)" {
		t.Fatalf("parameter shape = %q", got)
	}
	if got := pointerTypeShape(t, fn.ReturnType); got != "ptr(User)" {
		t.Fatalf("return shape = %q", got)
	}

	callback := result.Program.Statements[2].(*VariableDeclaration)
	fnType, ok := callback.Type.(*FunctionType)
	if !ok {
		t.Fatalf("callback type = %T, want FunctionType", callback.Type)
	}
	if got := pointerTypeShape(t, fnType.Params[0]); got != "ptrmut(User)" {
		t.Fatalf("function type parameter shape = %q", got)
	}
	if got := pointerTypeShape(t, fnType.Return); got != "ptr(User)" {
		t.Fatalf("function type return shape = %q", got)
	}
}

func TestAddressOfAndPointerDerefExpressions(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "read-only address", source: "&user", want: "ref(user)"},
		{name: "writable address", source: "&mut user", want: "refmut(user)"},
		{name: "address of field", source: "&user.profile", want: "ref(field(user,profile))"},
		{name: "address of struct literal", source: "&mut User{name: name}", want: "refmut(struct(User))"},
		{name: "address of call", source: "&mut load()", want: "refmut(call(load))"},
		{name: "address comparison", source: "&a == &b", want: "equal(ref(a),ref(b))"},
		{name: "deref", source: "pointer.*", want: "deref*(pointer)"},
		{name: "deref then field", source: "pointer.*.field", want: "field(deref*(pointer),field)"},
		{name: "field then deref", source: "pointer.field.*", want: "deref*(field(pointer,field))"},
		{name: "call then deref", source: "load().*", want: "deref*(call(load))"},
		{name: "deref times value", source: "pointer.* * 2", want: "mul(deref*(pointer),2)"},
		{name: "address of deref", source: "&mut pointer.*", want: "refmut(deref*(pointer))"},
		{name: "deref of address", source: "(&value).*", want: "deref*(ref(value))"},
		{name: "legacy borrow", source: "mut value", want: "mut(value)"},
		{name: "legacy deref", source: "reference.@", want: "deref(reference)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte("let result = "+tt.source+"\n"), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			declaration := result.Program.Statements[0].(*VariableDeclaration)
			if got := pointerExpressionShape(t, declaration.Value); got != tt.want {
				t.Fatalf("shape = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPointerDerefIsAnAssignmentTarget(t *testing.T) {
	result := Parse([]byte("pointer.* = replacement\npointer.*.name = name\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	if len(result.Program.Statements) != 2 {
		t.Fatalf("statement count = %d, want 2", len(result.Program.Statements))
	}
	whole, ok := result.Program.Statements[0].(*VariableAssignment)
	if !ok {
		t.Fatalf("statement 0 = %T, want VariableAssignment", result.Program.Statements[0])
	}
	if got := pointerExpressionShape(t, whole.Target); got != "deref*(pointer)" {
		t.Fatalf("whole target shape = %q", got)
	}
	field, ok := result.Program.Statements[1].(*VariableAssignment)
	if !ok {
		t.Fatalf("statement 1 = %T, want VariableAssignment", result.Program.Statements[1])
	}
	if got := pointerExpressionShape(t, field.Target); got != "field(deref*(pointer),name)" {
		t.Fatalf("field target shape = %q", got)
	}
}

func TestPointerDerefLocationsIncludeOperator(t *testing.T) {
	result := Parse([]byte("let result = pointer.*\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	dereference := result.Program.Statements[0].(*VariableDeclaration).Value.(*Deref)
	if want := (Location{Start: Point{Row: 1, Col: 14}, End: Point{Row: 1, Col: 22}}); dereference.Location != want {
		t.Fatalf("location = %#v, want %#v", dereference.Location, want)
	}
	if want := (Location{Start: Point{Row: 1, Col: 21}, End: Point{Row: 1, Col: 22}}); dereference.OperatorLocation != want {
		t.Fatalf("operator location = %#v, want %#v", dereference.OperatorLocation, want)
	}
}

func TestPointerDerefRequiresAdjacentDotAndStar(t *testing.T) {
	result := Parse([]byte("let result = pointer. *\n"), "test.ard")
	if len(result.Errors) == 0 {
		t.Fatalf("expected a parse error for a separated `. *`")
	}
}

func TestMultiplicationIsNotDereference(t *testing.T) {
	result := Parse([]byte("let result = left * right\n"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
	}
	declaration := result.Program.Statements[0].(*VariableDeclaration)
	if got := pointerExpressionShape(t, declaration.Value); got != "mul(left,right)" {
		t.Fatalf("shape = %q", got)
	}
}

func pointerTypeShape(t *testing.T, declared DeclaredType) string {
	t.Helper()
	suffix := ""
	if declared != nil && declared.IsNullable() {
		suffix = "?"
	}
	switch typ := declared.(type) {
	case *MutableType:
		inner := pointerTypeShape(t, typ.Inner)
		switch {
		case !typ.Pointer:
			return "mut(" + inner + ")" + suffix
		case typ.ReadOnly:
			return "ptr(" + inner + ")" + suffix
		default:
			return "ptrmut(" + inner + ")" + suffix
		}
	case *CustomType:
		return typ.Name + suffix
	case *GenericType:
		return "$" + typ.Name + suffix
	case *List:
		return "[" + pointerTypeShape(t, typ.Element) + "]" + suffix
	case *IntType:
		return "Int" + suffix
	default:
		t.Fatalf("unexpected type %T", declared)
		return ""
	}
}

func pointerExpressionShape(t *testing.T, expression Expression) string {
	t.Helper()
	switch value := expression.(type) {
	case *Identifier:
		return value.Name
	case *NumLiteral:
		return value.Value
	case *MutRef:
		operand := pointerExpressionShape(t, value.Operand)
		switch {
		case !value.Ampersand:
			return "mut(" + operand + ")"
		case value.ReadOnly:
			return "ref(" + operand + ")"
		default:
			return "refmut(" + operand + ")"
		}
	case *Deref:
		if value.Star {
			return "deref*(" + pointerExpressionShape(t, value.Operand) + ")"
		}
		return "deref(" + pointerExpressionShape(t, value.Operand) + ")"
	case *InstanceProperty:
		return "field(" + pointerExpressionShape(t, value.Target) + "," + value.Property.Name + ")"
	case *FunctionCall:
		return "call(" + value.Name + ")"
	case *StructInstance:
		return "struct(" + value.Name.Name + ")"
	case *BinaryExpression:
		switch value.Operator {
		case Equal:
			return "equal(" + pointerExpressionShape(t, value.Left) + "," + pointerExpressionShape(t, value.Right) + ")"
		case Multiply:
			return "mul(" + pointerExpressionShape(t, value.Left) + "," + pointerExpressionShape(t, value.Right) + ")"
		}
		t.Fatalf("unexpected operator %v", value.Operator)
		return ""
	default:
		t.Fatalf("unexpected expression %T", expression)
		return ""
	}
}
