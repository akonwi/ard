package ardgo

func MapSet[K comparable, V any](m map[K]V, key K, value V) bool {
	m[key] = value
	return true
}

func MapDrop[K comparable, V any](m map[K]V, key K) {
	delete(m, key)
}
