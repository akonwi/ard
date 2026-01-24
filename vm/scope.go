package vm

import (
	"github.com/akonwi/ard/runtime"
)

type scopeData struct {
	bindings  map[string]*runtime.Object
	breakable bool
	broke     bool // true if broken via break statement in a loop
	stopped   bool // true if execution stopped via try expression or result type early return
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

// update replaces the binding to point to a new Object, rather than mutating in place.
// This is used for simple variable reassignment to prevent aliasing issues.
func (s *scope) update(name string, value *runtime.Object) {
	if _, ok := s.data.bindings[name]; ok {
		s.data.bindings[name] = value
	} else if s.parent != nil {
		s.parent.update(name, value)
	}
}

func (s *scope) _break() {
	if s.data.breakable {
		s.data.broke = true
	} else if s.parent != nil {
		s.parent._break()
	}
}

func (s *scope) stop() {
	s.data.stopped = true
}

func (s *scope) setBreakable(breakable bool) {
	s.data.breakable = breakable
}

func (s *scope) isBroke() bool {
	return s.data.broke
}

func (s *scope) isStopped() bool {
	return s.data.stopped
}
