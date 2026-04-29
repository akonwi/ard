package ardgo

// todo: delete this during FFI rework
func AnySlice[T any](list []T) []any {
	out := make([]any, len(list))
	for i, value := range list {
		out[i] = value
	}
	return out
}
