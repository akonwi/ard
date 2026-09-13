package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/air"
)

// Go lowering for the tiered numeric conversions in ADR 0072.
//
// `from` lowers to a plain Go conversion and is handled with the other
// ExprScalarConvert forms. `fit` is a Go conversion too, except for a float
// source, which saturates through a runtime helper instead of inheriting Go's
// implementation-defined overflow. `try` always calls a runtime helper and
// evaluates to a Maybe.

// numericTarget describes a conversion endpoint: an integer with a width and
// signedness, or a float. A Rune is tracked separately because it carries a
// Unicode validity invariant that an Int32 does not.
type numericTarget struct {
	bits    int
	signed  bool
	isFloat bool
	isRune  bool
	ok      bool
}

// numericTargetOf resolves a type to its conversion endpoint, following a
// foreign named scalar to its Go underlying kind. Platform-sized types report
// zero bits, and callers emit math.MaxInt-style bounds for them.
func (l *lowerer) numericTargetOf(typeID air.TypeID) numericTarget {
	info, ok := l.typeInfo(typeID)
	if !ok {
		return numericTarget{}
	}
	if info.Kind == air.TypeForeignType {
		if info.ForeignPointer || info.ForeignInterface || info.Value == air.NoType {
			return numericTarget{}
		}
		// A foreign named scalar converts as its underlying Go kind. A
		// `type Color int32` is an Int32, never a Rune: Unicode validity is
		// never inferred from a type's width.
		underlying := l.numericTargetOf(info.Value)
		underlying.isRune = false
		return underlying
	}
	switch info.Kind {
	case air.TypeInt:
		return numericTarget{bits: 0, signed: true, ok: true}
	case air.TypeFloat64:
		return numericTarget{isFloat: true, ok: true}
	case air.TypeByte:
		return numericTarget{bits: 8, ok: true}
	case air.TypeRune:
		return numericTarget{bits: 32, signed: true, isRune: true, ok: true}
	case air.TypeScalar:
		return numericTargetByName(info.Name)
	}
	return numericTarget{}
}

func numericTargetByName(name string) numericTarget {
	switch name {
	case "Int":
		return numericTarget{bits: 0, signed: true, ok: true}
	case "Int8":
		return numericTarget{bits: 8, signed: true, ok: true}
	case "Int16":
		return numericTarget{bits: 16, signed: true, ok: true}
	case "Int32":
		return numericTarget{bits: 32, signed: true, ok: true}
	case "Int64":
		return numericTarget{bits: 64, signed: true, ok: true}
	case "Uint", "Uintptr":
		return numericTarget{bits: 0, ok: true}
	case "Uint8", "Byte":
		return numericTarget{bits: 8, ok: true}
	case "Uint16":
		return numericTarget{bits: 16, ok: true}
	case "Uint32":
		return numericTarget{bits: 32, ok: true}
	case "Uint64":
		return numericTarget{bits: 64, ok: true}
	case "Float32", "Float64":
		return numericTarget{isFloat: true, ok: true}
	}
	return numericTarget{}
}

// bitWidthExpr renders a target's width for a runtime helper. A platform-sized
// type has no constant width, so it passes math/bits.UintSize and the helper
// derives the bounds at run time.
func (l *lowerer) bitWidthExpr(target numericTarget) ast.Expr {
	if target.bits == 0 {
		return l.qualified("bits", "math/bits", "UintSize")
	}
	return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(target.bits)}
}

// lowerScalarTryConvert emits a checked conversion `T::try(x)`, evaluating to
// a Maybe of the target scalar (ADR 0072). expr.Type is the Maybe type, so the
// target scalar comes from its element.
func (l *lowerer) lowerScalarTryConvert(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("scalar try convert missing target")
	}
	maybeInfo, ok := l.typeInfo(expr.Type)
	if !ok || maybeInfo.Kind != air.TypeMaybe {
		return loweredExpr{}, fmt.Errorf("scalar try convert must produce a Maybe")
	}
	targetID := maybeInfo.Elem
	target := l.numericTargetOf(targetID)
	source := l.numericTargetOf(expr.Target.Type)
	if !target.ok || !source.ok {
		return loweredExpr{}, fmt.Errorf("scalar try convert between non-numeric types")
	}
	value, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetType, err := l.goType(targetID)
	if err != nil {
		return loweredExpr{}, err
	}

	// The helper takes the source widened to its widest carrier, so the
	// backend converts the value once and the helper compares in one domain.
	var helper string
	var args []ast.Expr
	switch {
	case target.isRune:
		switch {
		case source.isFloat:
			helper = "TryRuneFromFloat"
			args = []ast.Expr{l.convertTo("float64", value.expr)}
		case source.signed:
			helper = "TryRuneFromSigned"
			args = []ast.Expr{l.convertTo("int64", value.expr)}
		default:
			helper = "TryRuneFromUnsigned"
			args = []ast.Expr{l.convertTo("uint64", value.expr)}
		}
		// TryRune* is not generic: a Rune is always Go's rune.
		return loweredExpr{
			stmts: value.stmts,
			expr:  &ast.CallExpr{Fun: l.runtimeQualified(helper), Args: args},
		}, nil
	case target.isFloat:
		// Only Float64 -> Float32 is checked; every other float target is
		// lossless or fit-only, so the checker never emits them here.
		helper = "TryFloat64ToFloat32"
		args = []ast.Expr{l.convertTo("float64", value.expr)}
	case source.isFloat && target.signed:
		helper = "TryFloatToSigned"
		args = []ast.Expr{l.convertTo("float64", value.expr), l.bitWidthExpr(target)}
	case source.isFloat:
		helper = "TryFloatToUnsigned"
		args = []ast.Expr{l.convertTo("float64", value.expr), l.bitWidthExpr(target)}
	case source.signed && target.signed:
		helper = "TrySignedToSigned"
		args = []ast.Expr{l.convertTo("int64", value.expr), l.bitWidthExpr(target)}
	case source.signed:
		helper = "TrySignedToUnsigned"
		args = []ast.Expr{l.convertTo("int64", value.expr), l.bitWidthExpr(target)}
	case target.signed:
		helper = "TryUnsignedToSigned"
		args = []ast.Expr{l.convertTo("uint64", value.expr), l.bitWidthExpr(target)}
	default:
		helper = "TryUnsignedToUnsigned"
		args = []ast.Expr{l.convertTo("uint64", value.expr), l.bitWidthExpr(target)}
	}
	call := &ast.CallExpr{
		Fun:  &ast.IndexExpr{X: l.runtimeQualified(helper), Index: targetType},
		Args: args,
	}
	return loweredExpr{stmts: value.stmts, expr: call}, nil
}

// lowerScalarFitConvert emits a forced conversion `T::fit(x)` (ADR 0072). A
// float source saturates through a runtime helper; every other pair is Go's
// own defined wrapping or rounding, so it lowers to a plain conversion.
func (l *lowerer) lowerScalarFitConvert(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("scalar fit convert missing target")
	}
	target := l.numericTargetOf(expr.Type)
	source := l.numericTargetOf(expr.Target.Type)
	if !target.ok || !source.ok {
		return loweredExpr{}, fmt.Errorf("scalar fit convert between non-numeric types")
	}
	value, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	if source.isFloat && !target.isFloat {
		// Go leaves float-to-integer overflow implementation-defined, so this
		// saturates instead (ADR 0072).
		helper := "FitFloatToUnsigned"
		if target.signed {
			helper = "FitFloatToSigned"
		}
		call := &ast.CallExpr{
			Fun:  &ast.IndexExpr{X: l.runtimeQualified(helper), Index: targetType},
			Args: []ast.Expr{l.convertTo("float64", value.expr), l.bitWidthExpr(target)},
		}
		return loweredExpr{stmts: value.stmts, expr: call}, nil
	}
	return loweredExpr{
		stmts: value.stmts,
		expr:  &ast.CallExpr{Fun: targetType, Args: []ast.Expr{value.expr}},
	}, nil
}

// convertTo wraps value in a conversion to a predeclared Go type.
func (l *lowerer) convertTo(name string, value ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: l.ident(name), Args: []ast.Expr{value}}
}
