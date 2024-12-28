package vm

type variable struct {
	mut   bool
	value any
}

type scope struct {
	parent    *scope
	variables map[string]*variable
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:    parent,
		variables: make(map[string]*variable),
	}
}

func (s scope) getVariable(name string) (*variable, bool) {
	v, ok := s.variables[name]
	if !ok && s.parent != nil {
		return s.parent.getVariable(name)
	}
	return v, ok
}
