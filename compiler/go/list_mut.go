package ardgo

import "sort"

func AnySlice[T any](list []T) []any {
	out := make([]any, len(list))
	for i, value := range list {
		out[i] = value
	}
	return out
}

func ListPush[T any](list *[]T, value T) []T {
	*list = append(*list, value)
	return *list
}

func ListPrepend[T any](list *[]T, value T) []T {
	*list = append([]T{value}, (*list)...)
	return *list
}

func ListSet[T any](list []T, index int, value T) bool {
	if index >= 0 && index < len(list) {
		list[index] = value
		return true
	}
	return false
}

func ListSort[T any](list []T, less func(T, T) bool) {
	sort.SliceStable(list, func(i, j int) bool {
		return less(list[i], list[j])
	})
}

func ListSwap[T any](list []T, left int, right int) {
	list[left], list[right] = list[right], list[left]
}
