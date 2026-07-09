package checker

import "sort"

// Builtin method kind→name tables. These are the single source of truth for
// tooling-facing names; keep them in sync with the checker's method
// resolution when adding builtin methods.
var (
	strMethodNames = map[StrMethodKind]string{
		StrSize: "size", StrAt: "at", StrBytes: "bytes", StrRunes: "runes",
		StrIsEmpty: "is_empty", StrContains: "contains", StrReplace: "replace",
		StrReplaceAll: "replace_all",
		StrStartsWith: "starts_with", StrEndsWith: "ends_with",
		StrToStr: "to_str", StrTrim: "trim",
	}
	byteMethodNames  = map[ByteMethodKind]string{ByteToInt: "to_int", ByteToStr: "to_str"}
	runeMethodNames  = map[RuneMethodKind]string{RuneToInt: "to_int", RuneToStr: "to_str"}
	intMethodNames   = map[IntMethodKind]string{IntToStr: "to_str", IntToF64: "to_f64"}
	floatMethodNames = map[FloatMethodKind]string{FloatToStr: "to_str", FloatToInt: "to_int"}
	boolMethodNames  = map[BoolMethodKind]string{BoolToStr: "to_str"}
	listMethodNames  = map[ListMethodKind]string{
		ListAt: "at", ListPrepend: "prepend", ListPush: "push", ListSet: "set",
		ListSize: "size", ListSort: "sort", ListSwap: "swap",
	}
	mapMethodNames = map[MapMethodKind]string{
		MapKeys: "keys", MapSize: "size", MapGet: "get", MapSet: "set",
		MapDelete: "delete", MapHas: "has",
	}
	maybeMethodNames = map[MaybeMethodKind]string{
		MaybeExpect: "expect", MaybeIsNone: "is_none", MaybeIsSome: "is_some",
		MaybeOr: "or", MaybeMap: "map", MaybeAndThen: "and_then",
		MaybeSet: "set", MaybeClear: "clear",
	}
	resultMethodNames = map[ResultMethodKind]string{
		ResultExpect: "expect", ResultOr: "or", ResultIsOk: "is_ok",
		ResultIsErr: "is_err", ResultMap: "map", ResultMapErr: "map_err",
		ResultAndThen: "and_then",
	}
)

// BuiltinMethodInfo introspects a checked builtin-method node, returning the
// receiver type and the method name. Tooling (LSP hover, signature help) uses
// this to render builtin method signatures. ok is false when the node is not
// a builtin method or its kind has no name entry.
func BuiltinMethodInfo(node Expression) (receiver Type, name string, ok bool) {
	switch m := node.(type) {
	case *StrMethod:
		receiver, name = Str, strMethodNames[m.Kind]
	case *ByteMethod:
		receiver, name = Byte, byteMethodNames[m.Kind]
	case *RuneMethod:
		receiver, name = Rune, runeMethodNames[m.Kind]
	case *IntMethod:
		receiver, name = Int, intMethodNames[m.Kind]
	case *FloatMethod:
		receiver, name = Float64, floatMethodNames[m.Kind]
	case *BoolMethod:
		receiver, name = Bool, boolMethodNames[m.Kind]
	case *ListMethod:
		receiver, name = m.Subject.Type(), listMethodNames[m.Kind]
	case *MapMethod:
		receiver, name = m.Subject.Type(), mapMethodNames[m.Kind]
	case *MaybeMethod:
		receiver, name = m.Subject.Type(), maybeMethodNames[m.Kind]
	case *ResultMethod:
		receiver, name = m.Subject.Type(), resultMethodNames[m.Kind]
	default:
		return nil, "", false
	}
	if name == "" {
		return nil, "", false
	}
	return receiver, name, true
}

// BuiltinMethodDef resolves the builtin method's definition on the receiver.
func BuiltinMethodDef(receiver Type, name string) *FunctionDef {
	if receiver == nil || name == "" {
		return nil
	}
	if def, ok := receiver.get(name).(*FunctionDef); ok {
		return def
	}
	return nil
}

// BuiltinMemberNames returns the builtin method names available on a checked
// receiver type, driven by the same kind→name tables tooling renders from.
// Nil when the type has no builtin method surface.
func BuiltinMemberNames(receiver Type) []string {
	var table map[string]bool
	collect := func(names ...string) {
		if table == nil {
			table = map[string]bool{}
		}
		for _, name := range names {
			table[name] = true
		}
	}
	switch receiver {
	case Str:
		for _, name := range strMethodNames {
			collect(name)
		}
	case Int:
		for _, name := range intMethodNames {
			collect(name)
		}
	case Float64:
		for _, name := range floatMethodNames {
			collect(name)
		}
	case Bool:
		for _, name := range boolMethodNames {
			collect(name)
		}
	case Byte:
		for _, name := range byteMethodNames {
			collect(name)
		}
	case Rune:
		for _, name := range runeMethodNames {
			collect(name)
		}
	default:
		switch receiver.(type) {
		case *List:
			for _, name := range listMethodNames {
				collect(name)
			}
		case *Map:
			for _, name := range mapMethodNames {
				collect(name)
			}
		case *Maybe:
			for _, name := range maybeMethodNames {
				collect(name)
			}
		case *Result:
			for _, name := range resultMethodNames {
				collect(name)
			}
		}
	}
	if table == nil {
		return nil
	}
	out := make([]string, 0, len(table))
	for name := range table {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
