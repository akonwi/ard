package vm

import (
	"maps"
	"sync"

	"github.com/akonwi/ard/runtime"
)

type scope struct {
	parent    *scope
	bindings  map[string]*runtime.Object
	breakable bool
	broken    bool
	mu        sync.RWMutex
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:   parent,
		bindings: make(map[string]*runtime.Object),
	}
}

func (s scope) clone() scope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	new := &scope{
		parent:    s.parent,
		bindings:  make(map[string]*runtime.Object),
		breakable: s.breakable,
		broken:    s.broken,
	}

	maps.Copy(new.bindings, s.bindings)
	if s.parent != nil {
		*new.parent = s.parent.clone()
	}

	return *new
}

func (s *scope) add(name string, value *runtime.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bindings[name] = value
}

func (s scope) get(name string) (*runtime.Object, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.bindings[name]
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
	if s.breakable {
		s.broken = true
	} else if s.parent != nil {
		s.parent._break()
	}
}
