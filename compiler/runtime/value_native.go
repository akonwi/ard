package runtime

import (
	"sort"

	"github.com/akonwi/ard/bytecode"
)

type VoidValue struct{}

var NativeVoid = VoidValue{}

type MaybeValue struct {
	Value any
	None  bool
}

func SomeValue(value any) MaybeValue {
	return MaybeValue{Value: value}
}

func NoneValue() MaybeValue {
	return MaybeValue{None: true}
}

type ResultValue struct {
	Ok    any
	Err   any
	IsErr bool
}

func OkValue(value any) ResultValue {
	return ResultValue{Ok: value}
}

func ErrValue(err any) ResultValue {
	return ResultValue{Err: err, IsErr: true}
}

type ListValue []any

type ScalarKey interface {
	~string | ~int | ~float64 | ~bool
}

type VMMap interface {
	Len() int
	GetAny(key any) (any, bool)
	SetAny(key any, value any) bool
	DropAny(key any) bool
	HasAny(key any) bool
	Keys() []any
	Copy() VMMap
}

type Map[K ScalarKey] struct {
	Entries map[K]any
}

func NewMap[K ScalarKey]() *Map[K] {
	return &Map[K]{Entries: make(map[K]any)}
}

func (m *Map[K]) Len() int {
	if m == nil {
		return 0
	}
	return len(m.Entries)
}

func (m *Map[K]) GetAny(key any) (any, bool) {
	if m == nil {
		return nil, false
	}
	typed, ok := key.(K)
	if !ok {
		return nil, false
	}
	value, exists := m.Entries[typed]
	return value, exists
}

func (m *Map[K]) SetAny(key any, value any) bool {
	if m == nil {
		return false
	}
	typed, ok := key.(K)
	if !ok {
		return false
	}
	m.Entries[typed] = value
	return true
}

func (m *Map[K]) DropAny(key any) bool {
	if m == nil {
		return false
	}
	typed, ok := key.(K)
	if !ok {
		return false
	}
	if _, exists := m.Entries[typed]; !exists {
		return false
	}
	delete(m.Entries, typed)
	return true
}

func (m *Map[K]) HasAny(key any) bool {
	if m == nil {
		return false
	}
	typed, ok := key.(K)
	if !ok {
		return false
	}
	_, exists := m.Entries[typed]
	return exists
}

func (m *Map[K]) Keys() []any {
	if m == nil {
		return nil
	}
	keys := make([]K, 0, len(m.Entries))
	for key := range m.Entries {
		keys = append(keys, key)
	}
	sortScalarKeys(keys)
	out := make([]any, len(keys))
	for i, key := range keys {
		out[i] = any(key)
	}
	return out
}

func (m *Map[K]) Copy() VMMap {
	if m == nil {
		return NewMap[K]()
	}
	copied := NewMap[K]()
	for key, value := range m.Entries {
		copied.Entries[key] = value
	}
	return copied
}

func sortScalarKeys[K ScalarKey](keys []K) {
	if len(keys) < 2 {
		return
	}
	switch any(keys[0]).(type) {
	case string:
		sort.Slice(keys, func(i, j int) bool { return any(keys[i]).(string) < any(keys[j]).(string) })
	case int:
		sort.Slice(keys, func(i, j int) bool { return any(keys[i]).(int) < any(keys[j]).(int) })
	case float64:
		sort.Slice(keys, func(i, j int) bool { return any(keys[i]).(float64) < any(keys[j]).(float64) })
	case bool:
		sort.Slice(keys, func(i, j int) bool {
			left := any(keys[i]).(bool)
			right := any(keys[j]).(bool)
			return !left && right
		})
	}
}

type MapValue struct {
	KeyType   bytecode.TypeID
	ValueType bytecode.TypeID
	Storage   VMMap
}

type StructValue struct {
	TypeID bytecode.TypeID
	Fields []any
}

type EnumValue struct {
	TypeID bytecode.TypeID
	Tag    int
	Value  any
}
