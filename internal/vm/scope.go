package vm

type binding struct {
	mut      bool
	value    *object
	callable bool
}

type function func(args ...object) object

type scope struct {
	parent   *scope
	bindings map[string]*binding
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:   parent,
		bindings: make(map[string]*binding),
	}
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
