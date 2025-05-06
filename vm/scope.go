package vm

type function func(args ...object) object

type scope struct {
	parent    *scope
	bindings  map[string]*object
	breakable bool
	broken    bool
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:   parent,
		bindings: make(map[string]*object),
	}
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

func (s *scope) _break() {
	if s.breakable {
		s.broken = true
	} else if s.parent != nil {
		s.parent._break()
	}
}
