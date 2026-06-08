package runtime

import "sort"

type StructuralMap[K any, V any] map[string][]StructuralMapEntry[K, V]

type StructuralMapEntry[K any, V any] struct {
	Key   K
	Value V
}

func NewStructuralMap[K any, V any]() StructuralMap[K, V] {
	return make(StructuralMap[K, V])
}

func NewStructuralMapWithEntries[K any, V any](entries ...StructuralMapEntry[K, V]) StructuralMap[K, V] {
	m := NewStructuralMap[K, V]()
	for _, entry := range entries {
		m.Set(entry.Key, entry.Value)
	}
	return m
}

func (m StructuralMap[K, V]) Len() int {
	count := 0
	for _, bucket := range m {
		count += len(bucket)
	}
	return count
}

func (m StructuralMap[K, V]) Has(key K) bool {
	_, ok := m.Get(key)
	return ok
}

func (m StructuralMap[K, V]) Get(key K) (V, bool) {
	bucket := m[structuralMapHash(key)]
	for _, entry := range bucket {
		if Equal(entry.Key, key) {
			return entry.Value, true
		}
	}
	var zero V
	return zero, false
}

func (m StructuralMap[K, V]) Set(key K, value V) {
	hash := structuralMapHash(key)
	bucket := m[hash]
	for i, entry := range bucket {
		if Equal(entry.Key, key) {
			bucket[i].Value = value
			m[hash] = bucket
			return
		}
	}
	m[hash] = append(bucket, StructuralMapEntry[K, V]{Key: key, Value: value})
}

func (m StructuralMap[K, V]) Drop(key K) {
	hash := structuralMapHash(key)
	bucket := m[hash]
	for i, entry := range bucket {
		if Equal(entry.Key, key) {
			bucket = append(bucket[:i], bucket[i+1:]...)
			if len(bucket) == 0 {
				delete(m, hash)
			} else {
				m[hash] = bucket
			}
			return
		}
	}
}

func (m StructuralMap[K, V]) Keys() []K {
	hashes := make([]string, 0, len(m))
	for hash := range m {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	keys := make([]K, 0, m.Len())
	for _, hash := range hashes {
		for _, entry := range m[hash] {
			keys = append(keys, entry.Key)
		}
	}
	return keys
}

func (m StructuralMap[K, V]) KeyAt(index int) K {
	return m.Keys()[index]
}

func (m StructuralMap[K, V]) ValueAt(index int) V {
	key := m.KeyAt(index)
	value, _ := m.Get(key)
	return value
}

func structuralMapHash(_ any) string {
	return "_"
}
