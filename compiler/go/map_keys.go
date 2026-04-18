package ardgo

import (
	"fmt"
	"sort"
)

func MapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return mapKeyLess(keys[i], keys[j])
	})
	return keys
}

func mapKeyLess[K comparable](left, right K) bool {
	switch l := any(left).(type) {
	case string:
		return l < any(right).(string)
	case int:
		return l < any(right).(int)
	case bool:
		r := any(right).(bool)
		return !l && r
	case float64:
		return l < any(right).(float64)
	default:
		return fmt.Sprint(left) < fmt.Sprint(right)
	}
}
