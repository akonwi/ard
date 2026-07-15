package checker

func isReservedBuiltinTypeName(name string) bool {
	if scalarTypeByName(name) != nil {
		return true
	}
	switch name {
	case "Any", "Bool", "Byte", "Chan", "Float64", "Int", "Receiver", "Rune", "Sender", "Str", "Void":
		return true
	default:
		return false
	}
}
