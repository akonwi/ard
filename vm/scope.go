package vm

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/akonwi/ard/vm/runtime"
)

type scope struct {
	parent    *scope
	bindings  map[string]*runtime.Object
	breakable bool
	broken    bool
}

func newScope(parent *scope) *scope {
	if parent != nil {
		fmt.Printf("creating new scope from parent\n")
		names := slices.Collect(maps.Keys(parent.bindings))
		fmt.Printf("\tparent.bindings: %+v\n\tparent.parent: %v\n", strings.Join(names, ", "), parent.parent != nil)
	}
	return &scope{
		parent:   parent,
		bindings: make(map[string]*runtime.Object),
	}
}

func (s *scope) add(name string, value *runtime.Object) {
	s.bindings[name] = value
}

func (s scope) get(name string) (*runtime.Object, bool) {
	v, ok := s.bindings[name]
	fmt.Printf("looking for %s: %v\n", name, ok)
	if !ok && s.parent != nil {
		fmt.Printf("\tlooking in parent\n")
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
