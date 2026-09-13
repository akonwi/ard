package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/akonwi/ard/air"
)

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

// numericTargetOf resolves foreign named scalars to their underlying Go kind.
// Zero bits denotes a platform-sized integer.
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

// bitWidthExpr uses bits.UintSize for platform-sized targets.
func (l *lowerer) bitWidthExpr(target numericTarget) ast.Expr {
	if target.bits == 0 {
		return l.qualified("bits", "math/bits", "UintSize")
	}
	return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(target.bits)}
}

// lowerScalarTryConvert emits `T::try(x)` as a runtime helper call.
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

// lowerScalarFitConvert uses a saturating helper for float-to-integer; other
// forced conversions use Go's defined wrapping or rounding.
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

func (l *lowerer) convertTo(name string, value ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: l.ident(name), Args: []ast.Expr{value}}
}
