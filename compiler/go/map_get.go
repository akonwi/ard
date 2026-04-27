package ardgo

func MapGet[K comparable, V any](m map[K]V, key K) Maybe[V] {
	if value, ok := m[key]; ok {
		return Some(value)
	}
	return None[V]()
}
