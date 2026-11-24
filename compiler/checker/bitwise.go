package checker

import (
	"fmt"
	"math/big"

	"github.com/akonwi/ard/compiler/parse"
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
	return c.foldConstant(&IntBitNot{Value: value}, loc)
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
	return c.foldConstant(&IntBitwise{Op: bitwiseOp(s.Operator), Left: left, Right: right}, s.GetLocation())
}

// checkShift checks `<<` and `>>`. The operands are typed independently: the
// count may be any integer type, and the result has the left operand's type.
// Counts follow Go: a count past the width shifts every bit out, and a
// negative count panics at run time. Negative literal counts and literal
// shifts that overflow are rejected, as Go would reject the generated code.
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
	count, countIsConstant := c.constantValue(right)
	if countIsConstant && count.Sign() < 0 {
		c.addDiagnostic(invalidShiftDiagnostic{
			LegacyMessage: "Shift count must not be negative",
			Span:          c.sourceSpan(s.Right.GetLocation()),
			Label:         "a negative count panics",
		}.build())
		return nil
	}
	if _, ok := c.constantValue(left); ok && countIsConstant && count.Cmp(big.NewInt(maxConstantShiftCount)) > 0 {
		// A shift of two constants is evaluated while compiling (see
		// constant_fold.go); its result must fit its type. Go rejects a
		// constant count above this bound whatever the result.
		c.addDiagnostic(invalidShiftDiagnostic{
			LegacyMessage: fmt.Sprintf("Constant shift count %s is too large", count),
			Span:          c.sourceSpan(s.Right.GetLocation()),
			Label:         fmt.Sprintf("a constant shift count must be at most %d", maxConstantShiftCount),
		}.build())
		return nil
	}
	return c.foldConstant(&IntBitwise{Op: bitwiseOp(s.Operator), Left: left, Right: right}, s.GetLocation())
}

// maxConstantShiftCount is the largest count Go accepts in a shift of two
// constants (go/types' shiftBound).
const maxConstantShiftCount = 1074

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

func (c *Checker) addInvalidBitwise(operator string, left, right Expression, leftLoc, rightLoc parse.Location, legacy string, unsupported bool) {
	c.addDiagnostic(invalidBitwiseDiagnostic{
		Operator: operator, LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(leftLoc), RightSpan: c.sourceSpan(rightLoc), LegacyMessage: legacy, Unsupported: unsupported,
	}.build())
}
