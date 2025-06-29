package vm

import "github.com/akonwi/ard/checker"

func enforceSchema(vm *VM, val any, as checker.Type) *object {
	if as == checker.Int {
		num, ok := val.(float64)
		if !ok {
			return nil
		}
		return &object{int(num), as}
	}
	if as == checker.Float {
		num, ok := val.(float64)
		if !ok {
			return nil
		}
		return &object{num, as}
	}
	if as == checker.Str {
		str, ok := val.(string)
		if !ok {
			return nil
		}
		return &object{str, as}
	}
	if as == checker.Bool {
		is_ok, ok := val.(bool)
		if !ok {
			return nil
		}
		return &object{is_ok, as}
	}

	switch as := as.(type) {
	case *checker.StructDef:
		{
			jMap, ok := val.(map[string]any)
			if !ok {
				return nil
			}

			fields := make(map[string]*object)
			for name, fType := range as.Fields {
				val := enforceSchema(vm, jMap[name], fType)
				if val == nil {
					return nil
				}
				fields[name] = val
			}

			return &object{fields, as}
		}
	case *checker.Maybe:
		if val == nil {
			return &object{nil, as}
		}
		return enforceSchema(vm, val, as.Of())
	default:
		// todo: return as error
		panic("There's an unhandled ard Type in decoding: " + as.String())
	}

}
