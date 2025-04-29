package vm

import (
	"github.com/akonwi/ard/checker"
)

type function func(args ...object) object

type scope struct {
	parent    *scope
	bindings  map[string]*object
	enums     map[string]checker.Enum
	structs   map[string]*checker.Struct
	breakable bool
	broken    bool
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:   parent,
		bindings: make(map[string]*object),
		enums:    make(map[string]checker.Enum),
		structs:  make(map[string]*checker.Struct),
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

func (s scope) addStruct(strct *checker.Struct) {
	s.structs[strct.Name] = strct
}

func (s scope) getStruct(name string) (*checker.Struct, bool) {
	v, ok := s.structs[name]
	if !ok && s.parent != nil {
		return s.parent.getStruct(name)
	}
	return v, ok
}

func (s *scope) add(name string, value *object) {
	s.bindings[name] = value
}

func (s scope) get(name string) (*object, bool) {
	v, ok := s.bindings[name]
	if !ok && s.parent != nil {
		return s.parent.get(name)
	}
	return v, ok
}

func (s *scope) set(name string, value *object) {
	if binding, ok := s.get(name); ok {
		*binding = *value
	}
}

func (s scope) getFunction(name string) (function, bool) {
	if b, ok := s.get(name); ok && b.isCallable() {
		// can't cast w/ .(function) because `function` is a type alias not interface
		return b.raw.(func(args ...object) object), true
	}
	return nil, false
}

func (s *scope) _break() {
	if s.breakable {
		s.broken = true
	} else if s.parent != nil {
		s.parent._break()
	}
}

func (s *scope) isBroken() bool {
	if s.broken {
		return true
	}
	if s.parent != nil {
		return s.parent.isBroken()
	}
	return false
}
