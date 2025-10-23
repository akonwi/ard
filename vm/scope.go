package vm

import (
	"github.com/akonwi/ard/runtime"
)

type scopeData struct {
	bindings  map[string]*runtime.Object
	breakable bool
	broken    bool
}

type scope struct {
	parent *scope
	data   *scopeData
}

func newScope(parent *scope) *scope {
	return &scope{
		parent: parent,
		data: &scopeData{
			bindings: make(map[string]*runtime.Object),
		},
	}
}

func (s *scope) add(name string, value *runtime.Object) {
	s.data.bindings[name] = value
}

func (s *scope) get(name string) (*runtime.Object, bool) {
	v, ok := s.data.bindings[name]
	if !ok && s.parent != nil {
		return s.parent.get(name)
	}
	return v, ok
}

func (s *scope) set(name string, value *runtime.Object) {
	if binding, ok := s.get(name); ok {
		*binding = *value
	}
}

func (s *scope) _break() {
	if s.data.breakable {
		s.data.broken = true
	} else if s.parent != nil {
		s.parent._break()
	}
}

func (s *scope) setBroken(broken bool) {
	s.data.broken = broken
}

func (s *scope) setBreakable(breakable bool) {
	s.data.breakable = breakable
}



func (s *scope) isBroken() bool {
	return s.data.broken
}
