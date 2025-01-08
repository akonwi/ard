package vm

import "github.com/akonwi/ard/internal/checker"

type binding struct {
	mut      bool
	value    *object
	callable bool
}

type function func(args ...object) object

type scope struct {
	parent   *scope
	bindings map[string]*binding
	enums    map[string]checker.Enum
	structs  map[string]checker.Struct
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:   parent,
		bindings: make(map[string]*binding),
		enums:    make(map[string]checker.Enum),
		structs:  make(map[string]checker.Struct),
	}
}

func (s scope) addEnum(enum checker.Enum) {
	s.enums[enum.Name] = enum
}

func (s scope) getEnum(name string) (checker.Enum, bool) {
	v, ok := s.enums[name]
	if !ok && s.parent != nil {
		return s.parent.getEnum(name)
	}
	return v, ok
}

func (s scope) addStruct(strct checker.Struct) {
	s.structs[strct.Name] = strct
}

func (s scope) getStruct(name string) (checker.Struct, bool) {
	v, ok := s.structs[name]
	if !ok && s.parent != nil {
		return s.parent.getStruct(name)
	}
	return v, ok
}

func (s scope) get(name string) (*binding, bool) {
	v, ok := s.bindings[name]
	if !ok && s.parent != nil {
		return s.parent.get(name)
	}
	return v, ok
}

func (s scope) getFunction(name string) (function, bool) {
	if b, ok := s.get(name); ok && b.callable {
		// can't cast w/ .(function) because function is a type alias not interface
		return b.value.raw.(func(args ...object) object), true
	}
	return nil, false
}
