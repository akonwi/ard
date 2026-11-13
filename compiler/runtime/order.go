package runtime

// Read returns value. Go orders the calls in an expression but not its other
// operands, so the backend wraps a read in Read to evaluate it before a call
// to its right that could change it.
func Read[T any](value T) T {
	return value
}
