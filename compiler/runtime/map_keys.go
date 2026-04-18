package runtime

import (
	"fmt"
	"sort"
)

func SortedMapKeys(mapObj *Object) []*Object {
	keys := make([]*Object, 0, len(mapObj.AsMap()))
	for key := range mapObj.AsMap() {
		keys = append(keys, mapObj.Map_GetKey(key))
	}
	sort.Slice(keys, func(i, j int) bool {
		return runtimeMapKeyLess(keys[i], keys[j])
	})
	return keys
}

func runtimeMapKeyLess(left, right *Object) bool {
	switch l := left.Raw().(type) {
	case string:
		return l < right.Raw().(string)
	case int:
		return l < right.Raw().(int)
	case bool:
		r := right.Raw().(bool)
		return !l && r
	case float64:
		return l < right.Raw().(float64)
	default:
		leftType := ""
		rightType := ""
		if left.Type() != nil {
			leftType = left.Type().String()
		}
		if right.Type() != nil {
			rightType = right.Type().String()
		}
		if leftType != rightType {
			return leftType < rightType
		}
		return fmt.Sprint(left.Raw()) < fmt.Sprint(right.Raw())
	}
}
