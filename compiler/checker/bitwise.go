package checker

import (
	"fmt"
	"math/big"

	"github.com/akonwi/ard/parse"
)

// Bitwise operators on integers (ADR 0080).

// isBitwiseIntegerLike reports whether t supports the bitwise operators:
// Int, every sized integer, Uintptr, Byte, and foreign named types over one
// of those. Rune is excluded: it must stay a valid Unicode scalar value.
func isBitwiseIntegerLike(t Type) bool {
	if isBitwiseIntegerScalar(t) {
		return true
	}
	return isBitwiseIntegerScalar(foreignScalarPrimitive(t))
}

func isBitwiseIntegerScalar(t Type) bool {
	return t != Rune && isIntegerScalar(t)
}

// bitwiseOperandHint suggests the operator meant for a non-integer operand.
func bitwiseOperandHint(operator string, operand Type) string {
	if operand == Bool {
		switch operator {
		case "&":
			return "use 'and' for Bool"
		case "|":
			return "use 'or' for Bool"
		case "^":
			return "use '!=' for Bool"
		case "~":
			return "use 'not' for Bool"
		}
	}
	if operand == Rune || foreignScalarPrimitive(operand) == Rune {
		return "convert a Rune with Int32::from"
	}
	return ""
}

func withHint(message, hint string) string {
	if hint == "" {
		return message
	}
	return message + "; " + hint
}

func bitwiseOperatorString(operator parse.Operator) string {
	switch operator {
	case parse.BitAnd:
		return "&"
	case parse.BitOr:
		return "|"
	case parse.BitXor:
		return "^"
	case parse.ShiftLeft:
		return "<<"
	case parse.ShiftRight:
		return ">>"
	}
	return "?"
}

func bitwiseOp(operator parse.Operator) BitwiseOp {
	switch operator {
	case parse.BitOr:
		return BitOr
	case parse.BitXor:
		return BitXor
	case parse.ShiftLeft:
		return ShiftLeft
	case parse.ShiftRight:
		return ShiftRight
	default:
		return BitAnd
	}
}

func (c *Checker) checkBitNot(value Expression, loc parse.Location) Expression {
	value = observeReference(value)
	if IsNever(value.Type()) {
		value = &NeverCoercion{Value: value, TargetType: Int}
	}
	if !isBitwiseIntegerLike(value.Type()) {
		legacy := withHint("Only integers can be complemented with '~'", bitwiseOperandHint("~", value.Type()))
		c.addDiagnostic(invalidUnaryOperatorDiagnostic{Operator: "~", Operand: value.Type(), Span: c.sourceSpan(loc), LegacyMessage: legacy}.build())
		return nil
	}
	return &IntBitNot{Value: value}
}

// checkBitwiseBinary checks `&`, `|` and `^`, which follow the arithmetic
// operand rules: both sides have the same type, and an untyped literal adopts
// the other side's type.
func (c *Checker) checkBitwiseBinary(s *parse.BinaryExpression) Expression {
	operator := bitwiseOperatorString(s.Operator)
	left, right := c.checkScalarOperands(s.Left, s.Right)
	if left == nil || right == nil {
		return nil
	}
	if !validationEqualTypes(left.Type(), right.Type()) {
		c.addInvalidBitwise(operator, left, right, s.Left.GetLocation(), s.Right.GetLocation(), fmt.Sprintf("Cannot apply '%s' to different types", operator), false)
		return nil
	}
	if !isBitwiseIntegerLike(left.Type()) {
		legacy := withHint(fmt.Sprintf("The '%s' operator can only be used for integers", operator), bitwiseOperandHint(operator, left.Type()))
		c.addInvalidBitwise(operator, left, right, s.Left.GetLocation(), s.Right.GetLocation(), legacy, true)
		return nil
	}
	return &IntBitwise{Op: bitwiseOp(s.Operator), Left: left, Right: right}
}

// checkShift checks `<<` and `>>`. The operands are typed independently: the
// count may be any integer type, and the result has the left operand's type.
// Counts follow Go: a count past the width shifts every bit out, and a
// negative count panics at run time. Negative literal counts and literal
// shifts that overflow are rejected here, where Go would reject the
// generated code.
func (c *Checker) checkShift(s *parse.BinaryExpression) Expression {
	operator := bitwiseOperatorString(s.Operator)
	left := c.checkExpr(s.Left)
	if left == nil {
		return nil
	}
	right := c.checkExpr(s.Right)
	if right == nil {
		return nil
	}
	left, right = observeReference(left), observeReference(right)
	if IsNever(left.Type()) {
		left = &NeverCoercion{Value: left, TargetType: Int}
	}
	if IsNever(right.Type()) {
		right = &NeverCoercion{Value: right, TargetType: Int}
	}
	if !isBitwiseIntegerLike(left.Type()) {
		legacy := withHint(fmt.Sprintf("The '%s' operator can only be used for integers", operator), bitwiseOperandHint(operator, left.Type()))
		c.addDiagnostic(invalidShiftDiagnostic{
			LegacyMessage: legacy,
			Span:          c.sourceSpan(s.Left.GetLocation()),
			Label:         fmt.Sprintf("operator `%s` cannot be applied to `%s`", operator, left.Type()),
		}.build())
		return nil
	}
	if !isBitwiseIntegerLike(right.Type()) {
		c.addDiagnostic(invalidShiftDiagnostic{
			LegacyMessage: "Shift count must be an integer",
			Span:          c.sourceSpan(s.Right.GetLocation()),
			Label:         fmt.Sprintf("this count has type `%s`", right.Type()),
		}.build())
		return nil
	}
	count, countIsConstant := constantIntValue(right)
	if countIsConstant && count.Sign() < 0 {
		c.addDiagnostic(invalidShiftDiagnostic{
			LegacyMessage: "Shift count must not be negative",
			Span:          c.sourceSpan(s.Right.GetLocation()),
			Label:         "a negative count panics",
		}.build())
		return nil
	}
	if value, ok := constantIntValue(left); ok && countIsConstant {
		// A shift of two literals is a Go constant expression, evaluated at
		// compile time: it must fit its type, and Go rejects large counts.
		if s.Operator == parse.ShiftLeft && !c.constantShiftFits(value, count, left.Type()) {
			legacy := fmt.Sprintf("Constant %s << %s overflows %s", value, count, left.Type())
			c.addDiagnostic(numericLiteralOverflowDiagnostic{LegacyMessage: legacy, Span: c.sourceSpan(s.GetLocation()), Target: left.Type()}.build())
			return nil
		}
		if bits := c.integerBits(left.Type()); count.Cmp(big.NewInt(int64(bits))) >= 0 {
			c.addDiagnostic(invalidShiftDiagnostic{
				LegacyMessage: fmt.Sprintf("Constant shift count %s is too large for %s", count, left.Type()),
				Span:          c.sourceSpan(s.Right.GetLocation()),
				Label:         fmt.Sprintf("`%s` has %d bits", left.Type(), bits),
			}.build())
			return nil
		}
	}
	return &IntBitwise{Op: bitwiseOp(s.Operator), Left: left, Right: right}
}

// integerBits returns the width of an integer type that supports bitwise
// operators.
func (c *Checker) integerBits(t Type) int {
	if prim := foreignScalarPrimitive(t); prim != nil {
		t = prim
	}
	switch t {
	case Int8, Uint8, Byte:
		return 8
	case Int16, Uint16:
		return 16
	case Int32, Uint32:
		return 32
	case Int64, Uint64:
		return 64
	case Uint:
		return c.targetUintBits()
	case Uintptr:
		return c.targetUintptrBits()
	default:
		return c.targetIntBits()
	}
}

// constantShiftFits reports whether value << count is representable in t.
func (c *Checker) constantShiftFits(value, count *big.Int, t Type) bool {
	if value.Sign() == 0 {
		return true
	}
	// No integer type is wider than 64 bits; avoid building huge numbers.
	if count.Cmp(big.NewInt(64)) > 0 {
		return false
	}
	shifted := new(big.Int).Lsh(value, uint(count.Uint64()))
	primitive := t
	if prim := foreignScalarPrimitive(t); prim != nil {
		primitive = prim
	}
	if isUnsignedScalar(primitive) || primitive == Byte {
		if primitive == Byte {
			primitive = Uint8
		}
		return c.uintLiteralFitsType(shifted, primitive)
	}
	return shifted.IsInt64() && c.intLiteralFitsType(shifted.Int64(), primitive)
}

// constantIntValue returns the value of an integer literal, possibly negated.
func constantIntValue(expr Expression) (*big.Int, bool) {
	switch e := expr.(type) {
	case *IntLiteral:
		return big.NewInt(int64(e.Value)), true
	case *TypedIntLiteral:
		value, ok := new(big.Int).SetString(e.Text, 0)
		return value, ok
	case *Negation:
		value, ok := constantIntValue(e.Value)
		if !ok {
			return nil, false
		}
		return value.Neg(value), true
	}
	return nil, false
}

func (c *Checker) addInvalidBitwise(operator string, left, right Expression, leftLoc, rightLoc parse.Location, legacy string, unsupported bool) {
	c.addDiagnostic(invalidBitwiseDiagnostic{
		Operator: operator, LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(leftLoc), RightSpan: c.sourceSpan(rightLoc), LegacyMessage: legacy, Unsupported: unsupported,
	}.build())
}
