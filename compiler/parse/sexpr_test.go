package parse

import (
	"fmt"
	"strings"
)

// sexpr renders an expression as a compact s-expression so tests can pin
// down how an ambiguous `<`/`>` sequence was grouped.
func sexpr(node Statement) string {
	switch n := node.(type) {
	case nil:
		return "<nil>"
	case *Identifier:
		return n.Name
	case *NumLiteral:
		return n.Value
	case *StrLiteral:
		return fmt.Sprintf("%q", n.Value)
	case *BoolLiteral:
		return fmt.Sprint(n.Value)
	case *VariableDeclaration:
		return fmt.Sprintf("(let %s %s)", n.Name, sexpr(n.Value))
	case *UnaryExpression:
		return fmt.Sprintf("(%s %s)", operatorSymbol(n.Operator), sexpr(n.Operand))
	case *BinaryExpression:
		return fmt.Sprintf("(%s %s %s)", operatorSymbol(n.Operator), sexpr(n.Left), sexpr(n.Right))
	case *ChainedComparison:
		parts := []string{"chain", sexpr(n.Operands[0])}
		for i, op := range n.Operators {
			parts = append(parts, operatorSymbol(op), sexpr(n.Operands[i+1]))
		}
		return "(" + strings.Join(parts, " ") + ")"
	case *FunctionCall:
		return callSexpr(*n)
	case *StaticFunction:
		return fmt.Sprintf("(:: %s %s)", sexpr(n.Target), callSexpr(n.Function))
	case *StaticProperty:
		return fmt.Sprintf("(:: %s %s)", sexpr(n.Target), sexpr(n.Property))
	case *InstanceMethod:
		return fmt.Sprintf("(. %s %s)", sexpr(n.Target), callSexpr(n.Method))
	case *InstanceProperty:
		return fmt.Sprintf("(. %s %s)", sexpr(n.Target), n.Property.Name)
	case *StructInstance:
		return fmt.Sprintf("(struct %s%s)", n.Name.Name, typeArgsSexpr(n.TypeArgs))
	default:
		return fmt.Sprintf("%T", node)
	}
}

func callSexpr(call FunctionCall) string {
	args := make([]string, 0, len(call.Args))
	for _, arg := range call.Args {
		args = append(args, sexpr(arg.Value))
	}
	return fmt.Sprintf("(call %s%s %s)", call.Name, typeArgsSexpr(call.TypeArgs), strings.Join(args, " "))
}

func typeArgsSexpr(typeArgs []DeclaredType) string {
	if len(typeArgs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(typeArgs))
	for _, typeArg := range typeArgs {
		parts = append(parts, typeSexpr(typeArg))
	}
	return "<" + strings.Join(parts, ", ") + ">"
}

func typeSexpr(declared DeclaredType) string {
	if declared.IsNullable() {
		return nonNullTypeSexpr(declared) + "?"
	}
	return nonNullTypeSexpr(declared)
}

func nonNullTypeSexpr(declared DeclaredType) string {
	switch ty := declared.(type) {
	case *CustomType:
		return ty.Name + typeArgsSexpr(ty.TypeArgs)
	case *List:
		return "[" + typeSexpr(ty.Element) + "]"
	case *Map:
		return "[" + typeSexpr(ty.Key) + ": " + typeSexpr(ty.Value) + "]"
	case *StringType:
		return "Str"
	case *ResultType:
		return typeSexpr(ty.Val) + "!" + typeSexpr(ty.Err)
	default:
		return declared.GetName()
	}
}

func operatorSymbol(op Operator) string {
	switch op {
	case GreaterThan:
		return ">"
	case GreaterThanOrEqual:
		return ">="
	case LessThan:
		return "<"
	case LessThanOrEqual:
		return "<="
	case Equal:
		return "=="
	case NotEqual:
		return "!="
	case And:
		return "and"
	case Or:
		return "or"
	case Not:
		return "not"
	case Minus:
		return "-"
	case Plus:
		return "+"
	case Multiply:
		return "*"
	case Divide:
		return "/"
	case ShiftLeft:
		return "<<"
	case ShiftRight:
		return ">>"
	case BitAnd:
		return "&"
	case BitOr:
		return "|"
	case BitXor:
		return "^"
	default:
		return fmt.Sprintf("op%d", op)
	}
}
