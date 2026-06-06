package checker

import "github.com/akonwi/ard/parse"

func recursiveFieldHasInfiniteSize(t parse.DeclaredType, self string, indirect bool) bool {
	if t == nil {
		return false
	}
	if t.IsNullable() {
		indirect = true
	}
	switch typ := t.(type) {
	case *parse.MutableType:
		return recursiveFieldHasInfiniteSize(typ.Inner, self, true)
	case parse.MutableType:
		return recursiveFieldHasInfiniteSize(typ.Inner, self, true)
	case *parse.CustomType:
		return typ.GetName() == self && !indirect
	case parse.CustomType:
		return typ.GetName() == self && !indirect
	case *parse.List:
		return recursiveFieldHasInfiniteSize(typ.Element, self, true)
	case parse.List:
		return recursiveFieldHasInfiniteSize(typ.Element, self, true)
	case *parse.Map:
		return recursiveFieldHasInfiniteSize(typ.Key, self, false) || recursiveFieldHasInfiniteSize(typ.Value, self, true)
	case parse.Map:
		return recursiveFieldHasInfiniteSize(typ.Key, self, false) || recursiveFieldHasInfiniteSize(typ.Value, self, true)
	case *parse.ResultType:
		return recursiveFieldHasInfiniteSize(typ.Val, self, indirect) || recursiveFieldHasInfiniteSize(typ.Err, self, indirect)
	case parse.ResultType:
		return recursiveFieldHasInfiniteSize(typ.Val, self, indirect) || recursiveFieldHasInfiniteSize(typ.Err, self, indirect)
	case *parse.FunctionType:
		if recursiveFieldHasInfiniteSize(typ.Return, self, indirect) {
			return true
		}
		for _, param := range typ.Params {
			if recursiveFieldHasInfiniteSize(param, self, indirect) {
				return true
			}
		}
		return false
	case parse.FunctionType:
		if recursiveFieldHasInfiniteSize(typ.Return, self, indirect) {
			return true
		}
		for _, param := range typ.Params {
			if recursiveFieldHasInfiniteSize(param, self, indirect) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
