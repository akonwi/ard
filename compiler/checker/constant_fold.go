package checker

import (
	"fmt"
	"math/big"

	"github.com/akonwi/ard/compiler/parse"
)

// Constant integer expressions.
//
// The Go backend emits integer literals and Go constants as Go constants, so
// an expression built only from them is a Go constant expression: Go
// evaluates it while compiling and rejects a value that does not fit its type,
// a division by a constant zero, and a constant conversion that would lose
// the value. The checker evaluates these expressions first and reports those
// cases in Ard. Every intermediate value must fit its type, as for Go's typed
// constants. T::fit of a constant wraps at compile time.

// constantValue returns the value of an integer expression Go treats as a
// constant. Arithmetic nodes are folded once, when created (foldConstant),
// so this is a lookup rather than a walk.
func (c *Checker) constantValue(expr Expression) (*big.Int, bool) {
	switch e := expr.(type) {
	case *IntLiteral:
		return big.NewInt(int64(e.Value)), true
	case *TypedIntLiteral:
		if !isIntegerScalar(e.Typed) && !isIntegerScalar(foreignScalarPrimitive(e.Typed)) {
			return nil, false
		}
		return new(big.Int).SetString(e.String(), 0)
	case *RuneLiteral:
		return big.NewInt(int64(e.Value)), true
	case *ScalarFrom:
		// A conversion of a constant that keeps its value is a constant. A
		// `fit` of a constant that does not fit was replaced by a literal.
		if e.Tier == ConversionTry || !isIntegerScalar(e.Target) && !isIntegerScalar(foreignScalarPrimitive(e.Target)) {
			return nil, false
		}
		value, ok := c.constantValue(e.Value)
		if !ok || !c.constantFits(value, e.Target) {
			return nil, false
		}
		return value, true
	case *ForeignScalarConvert:
		return c.constantValue(e.Value)
	case *ForeignValue, *IntAddition, *IntSubtraction, *IntMultiplication, *IntDivision,
		*IntModulo, *IntBitwise, *IntBitNot, *Negation:
		value, ok := c.constants[expr]
		return value, ok
	}
	return nil, false
}

func (c *Checker) recordConstant(expr Expression, value *big.Int) {
	if c.constants == nil {
		c.constants = map[Expression]*big.Int{}
	}
	c.constants[expr] = value
}

// foldConstant evaluates an integer operation whose operands are constants,
// and reports a division by a constant zero even when the dividend is not
// constant. It returns nil after reporting an error.
func (c *Checker) foldConstant(node Expression, loc parse.Location) Expression {
	var left, right Expression
	switch n := node.(type) {
	case *IntAddition:
		left, right = n.Left, n.Right
	case *IntSubtraction:
		left, right = n.Left, n.Right
	case *IntMultiplication:
		left, right = n.Left, n.Right
	case *IntDivision:
		left, right = n.Left, n.Right
	case *IntModulo:
		left, right = n.Left, n.Right
	case *IntBitwise:
		left, right = n.Left, n.Right
	case *IntBitNot:
		left = n.Value
	case *Negation:
		left = n.Value
	default:
		return node
	}

	var rightValue *big.Int
	if right != nil {
		value, ok := c.constantValue(right)
		if !ok {
			return node
		}
		rightValue = value
		_, isDivision := node.(*IntDivision)
		_, isModulo := node.(*IntModulo)
		if (isDivision || isModulo) && value.Sign() == 0 {
			c.addDiagnostic(divisionByZeroDiagnostic{Span: c.sourceSpan(loc)}.build())
			return nil
		}
	}
	leftValue, ok := c.constantValue(left)
	if !ok {
		return node
	}

	result := new(big.Int)
	switch n := node.(type) {
	case *IntAddition:
		result.Add(leftValue, rightValue)
	case *IntSubtraction:
		result.Sub(leftValue, rightValue)
	case *IntMultiplication:
		result.Mul(leftValue, rightValue)
	case *IntDivision:
		// Go's integer division truncates toward zero, as Quo does.
		result.Quo(leftValue, rightValue)
	case *IntModulo:
		result.Rem(leftValue, rightValue)
	case *Negation:
		if !isIntegerScalar(n.Type()) && !isIntegerScalar(foreignScalarPrimitive(n.Type())) {
			return node
		}
		result.Neg(leftValue)
	case *IntBitNot:
		if c.isUnsignedInteger(n.Type()) {
			// Go complements an unsigned constant within its width.
			mask := new(big.Int).Lsh(big.NewInt(1), uint(c.integerBits(n.Type())))
			mask.Sub(mask, big.NewInt(1))
			result.Xor(leftValue, mask)
		} else {
			result.Not(leftValue)
		}
	case *IntBitwise:
		switch n.Op {
		case BitAnd:
			result.And(leftValue, rightValue)
		case BitOr:
			result.Or(leftValue, rightValue)
		case BitXor:
			result.Xor(leftValue, rightValue)
		case ShiftLeft:
			// checkShift has bounded the count by maxConstantShiftCount.
			result.Lsh(leftValue, uint(rightValue.Uint64()))
		case ShiftRight:
			result.Rsh(leftValue, uint(rightValue.Uint64()))
		}
	}
	if !c.constantFits(result, node.Type()) {
		legacy := fmt.Sprintf("Constant value %s overflows %s", result, node.Type())
		c.addDiagnostic(constantOverflowDiagnostic{LegacyMessage: legacy, Span: c.sourceSpan(loc), Target: node.Type()}.build())
		return nil
	}
	c.recordConstant(node, result)
	return node
}

// constantFits reports whether value is representable in the integer type t.
func (c *Checker) constantFits(value *big.Int, t Type) bool {
	primitive := t
	if prim := foreignScalarPrimitive(t); prim != nil {
		primitive = prim
	}
	if primitive == Byte {
		primitive = Uint8
	}
	if isUnsignedScalar(primitive) {
		return c.uintLiteralFitsType(value, primitive)
	}
	return value.IsInt64() && c.intLiteralFitsType(value.Int64(), primitive)
}

func (c *Checker) isUnsignedInteger(t Type) bool {
	if prim := foreignScalarPrimitive(t); prim != nil {
		t = prim
	}
	return t == Byte || isUnsignedScalar(t)
}

// wrapConstant converts value to the integer type t the way Go's T(x) does
// for a run-time value: it keeps the low bits.
func (c *Checker) wrapConstant(value *big.Int, t Type) *big.Int {
	bits := uint(c.integerBits(t))
	modulus := new(big.Int).Lsh(big.NewInt(1), bits)
	wrapped := new(big.Int).Mod(value, modulus)
	if !c.isUnsignedInteger(t) && wrapped.Cmp(new(big.Int).Rsh(modulus, 1)) >= 0 {
		wrapped.Sub(wrapped, modulus)
	}
	return wrapped
}

// constantLiteral builds the literal node for a constant integer of type t.
func constantLiteral(value *big.Int, t Type) Expression {
	if t == Int {
		return &IntLiteral{Value: int(value.Int64())}
	}
	return &TypedIntLiteral{Value: int(value.Int64()), Text: value.String(), Typed: t}
}
