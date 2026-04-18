package runtime

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/akonwi/ard/checker"
)

func SortedMapKeys(mapObj *Object) []*Object {
	rawMap := mapObj.AsMap()
	mapType := mapObj.MapType()
	if mapType != nil {
		switch mapType.Key() {
		case checker.Str, checker.Dynamic:
			keys := make([]string, 0, len(rawMap))
			for key := range rawMap {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			out := make([]*Object, len(keys))
			for i := range keys {
				out[i] = mapObj.Map_GetKey(keys[i])
			}
			return out
		case checker.Int:
			type intKey struct {
				raw string
				val int
			}
			keys := make([]intKey, 0, len(rawMap))
			for key := range rawMap {
				val, err := strconv.Atoi(key)
				if err != nil {
					panic(fmt.Errorf("Couldn't turn map key %s into int", key))
				}
				keys = append(keys, intKey{raw: key, val: val})
			}
			sort.Slice(keys, func(i, j int) bool { return keys[i].val < keys[j].val })
			out := make([]*Object, len(keys))
			for i := range keys {
				out[i] = mapObj.Map_GetKey(keys[i].raw)
			}
			return out
		case checker.Bool:
			hasFalse := false
			hasTrue := false
			for key := range rawMap {
				val, err := strconv.ParseBool(key)
				if err != nil {
					panic(fmt.Errorf("Couldn't turn map key %s into bool", key))
				}
				if val {
					hasTrue = true
				} else {
					hasFalse = true
				}
			}
			out := make([]*Object, 0, len(rawMap))
			if hasFalse {
				out = append(out, mapObj.Map_GetKey("false"))
			}
			if hasTrue {
				out = append(out, mapObj.Map_GetKey("true"))
			}
			return out
		case checker.Float:
			type floatKey struct {
				raw string
				val float64
			}
			keys := make([]floatKey, 0, len(rawMap))
			for key := range rawMap {
				val, err := strconv.ParseFloat(key, 64)
				if err != nil {
					panic(fmt.Errorf("Couldn't turn map key %s into float", key))
				}
				keys = append(keys, floatKey{raw: key, val: val})
			}
			sort.Slice(keys, func(i, j int) bool { return keys[i].val < keys[j].val })
			out := make([]*Object, len(keys))
			for i := range keys {
				out[i] = mapObj.Map_GetKey(keys[i].raw)
			}
			return out
		}
	}

	keys := make([]*Object, 0, len(rawMap))
	for key := range rawMap {
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
