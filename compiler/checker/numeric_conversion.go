package checker

// Tiered numeric conversions (ADR 0072).
//
// Every numeric source/target pair admits exactly one of three tiers:
//
//	from  lossless: every source value is exactly representable in the target
//	try   fallible: returns the target wrapped in a Maybe
//	fit   lossy but total: defined truncation, wrapping, or saturation
//
// Choosing the wrong tier is a compile error that names the right one, so each
// conversion has a single spelling and lossy conversions announce themselves.

// ConversionTier identifies one of the three conversion spellings.
type ConversionTier int

const (
	// ConversionFrom is the lossless tier, spelled `T::from(x)`.
	ConversionFrom ConversionTier = iota
	// ConversionTry is the checked tier, spelled `T::try(x)`, returning `T?`.
	ConversionTry
	// ConversionFit is the forced tier, spelled `T::fit(x)`, returning `T`.
	ConversionFit
)

func (t ConversionTier) String() string {
	switch t {
	case ConversionTry:
		return "try"
	case ConversionFit:
		return "fit"
	default:
		return "from"
	}
}

// conversionTierByName maps a static function name to its tier. Only these
// three names are numeric conversions.
func conversionTierByName(name string) (ConversionTier, bool) {
	switch name {
	case "from":
		return ConversionFrom, true
	case "try":
		return ConversionTry, true
	case "fit":
		return ConversionFit, true
	}
	return 0, false
}

// numericClass is the conversion identity of a scalar. Byte and Uint8 share a
// class because they share a representation and an invariant. Rune has its own
// class even though it is an int32 in Go: a Rune is always a valid Unicode
// scalar value, and Int32 is not.
type numericClass int

const (
	classNone numericClass = iota
	classInt8
	classInt16
	classInt32
	classInt64
	classInt
	classUint8
	classUint16
	classUint32
	classUint64
	classUint
	classUintptr
	classRune
	classFloat32
	classFloat64
)

// numericClassOf reports the conversion class of t, resolving a foreign named
// scalar through its Go underlying kind. A Go `type Color int32` is an Int32,
// never a Rune: Unicode validity is never inferred from a type's width.
func numericClassOf(t Type) numericClass {
	if t == nil {
		return classNone
	}
	if prim := foreignScalarPrimitive(t); prim != nil {
		t = prim
	}
	switch t {
	case Int:
		return classInt
	case Int8:
		return classInt8
	case Int16:
		return classInt16
	case Int32:
		return classInt32
	case Int64:
		return classInt64
	case Uint:
		return classUint
	case Uint8, Byte:
		return classUint8
	case Uint16:
		return classUint16
	case Uint32:
		return classUint32
	case Uint64:
		return classUint64
	case Uintptr:
		return classUintptr
	case Rune:
		return classRune
	case Float32:
		return classFloat32
	case Float64:
		return classFloat64
	}
	return classNone
}

// numericShape describes a class for the purpose of deciding losslessness.
//
// Integers carry a bit width and signedness. Floats carry the number of
// significant mantissa bits (24 for Float32, 53 for Float64).
type numericShape struct {
	bits     int
	signed   bool
	isFloat  bool
	mantissa int
}

// Int, Uint, and Uintptr are platform-sized. Go guarantees only that they are
// at least 32 bits, so a conversion is lossless only if it holds on every
// supported platform: they count as 32 bits when they are a target (the
// target may be the narrow one) and 64 bits when they are a source (the
// source may be the wide one). That asymmetry is what makes Int64 -> Int
// fallible even though it never fails on a 64-bit host.
func numericShapeOf(class numericClass, asSource bool) (numericShape, bool) {
	platform := 32
	if asSource {
		platform = 64
	}
	switch class {
	case classInt8:
		return numericShape{bits: 8, signed: true}, true
	case classInt16:
		return numericShape{bits: 16, signed: true}, true
	case classInt32:
		return numericShape{bits: 32, signed: true}, true
	case classInt64:
		return numericShape{bits: 64, signed: true}, true
	case classInt:
		return numericShape{bits: platform, signed: true}, true
	case classUint8:
		return numericShape{bits: 8}, true
	case classUint16:
		return numericShape{bits: 16}, true
	case classUint32:
		return numericShape{bits: 32}, true
	case classUint64:
		return numericShape{bits: 64}, true
	case classUint, classUintptr:
		return numericShape{bits: platform}, true
	case classRune:
		// A Rune is a valid Unicode scalar value, so as a source its whole
		// range fits in 21 unsigned bits. As a target it is special-cased:
		// only Uint8 covers it losslessly (see losslessConversion).
		return numericShape{bits: 21}, true
	case classFloat32:
		return numericShape{isFloat: true, mantissa: 24}, true
	case classFloat64:
		return numericShape{isFloat: true, mantissa: 53}, true
	}
	return numericShape{}, false
}

// losslessConversion reports whether every value of src is exactly
// representable as dst.
func losslessConversion(src, dst numericClass) bool {
	if src == classNone || dst == classNone {
		return false
	}
	if src == dst {
		// Identity, including Byte <-> Uint8 and a foreign named scalar and
		// its underlying primitive in either direction.
		return true
	}
	if dst == classRune {
		// Nothing may manufacture an invalid Rune (ADR 0026, ADR 0072). Only
		// Uint8's whole range is valid Unicode.
		return src == classUint8
	}
	source, ok := numericShapeOf(src, true)
	if !ok {
		return false
	}
	target, ok := numericShapeOf(dst, false)
	if !ok {
		return false
	}
	switch {
	case source.isFloat && target.isFloat:
		// Float32 -> Float64 only; Float64 -> Float32 rounds.
		return source.mantissa <= target.mantissa
	case source.isFloat:
		// A float may be fractional or out of range, never lossless into an
		// integer.
		return false
	case target.isFloat:
		// The source's largest magnitude must fit in the mantissa. A signed
		// source spends one bit on the sign.
		magnitude := source.bits
		if source.signed {
			magnitude--
		}
		return magnitude <= target.mantissa
	case source.signed == target.signed:
		return source.bits <= target.bits
	case source.signed && !target.signed:
		// Negative values have no unsigned representation.
		return false
	default:
		// Unsigned into signed needs a bit for the sign.
		return source.bits < target.bits
	}
}

// conversionTierFor reports the single tier that is valid for src -> dst, and
// whether the pair is a numeric conversion at all.
//
// Lossless pairs take `from`. Every other pair takes `try` or `fit`:
//
//   - Nothing may produce an invalid Rune, so a Rune target has no `fit`.
//   - Integer -> float rounds but can never overflow or fail, so it has no
//     `try`; `fit` is the only spelling. This also keeps the compiler out of
//     proving float exactness, which cannot be done portably.
//   - Every other lossy pair admits both `try` and `fit`.
func conversionTierFor(src, dst numericClass) (tiers []ConversionTier, ok bool) {
	if src == classNone || dst == classNone {
		return nil, false
	}
	if losslessConversion(src, dst) {
		return []ConversionTier{ConversionFrom}, true
	}
	source, valid := numericShapeOf(src, true)
	if !valid {
		return nil, false
	}
	target, valid := numericShapeOf(dst, false)
	if !valid {
		return nil, false
	}
	if !source.isFloat && target.isFloat {
		return []ConversionTier{ConversionFit}, true
	}
	if dst == classRune {
		return []ConversionTier{ConversionTry}, true
	}
	return []ConversionTier{ConversionTry, ConversionFit}, true
}

// conversionTierAllowed reports whether tier is the valid spelling for
// src -> dst, along with the tiers that are valid when it is not.
func conversionTierAllowed(src, dst numericClass, tier ConversionTier) (bool, []ConversionTier) {
	tiers, ok := conversionTierFor(src, dst)
	if !ok {
		return false, nil
	}
	for _, t := range tiers {
		if t == tier {
			return true, tiers
		}
	}
	return false, tiers
}
