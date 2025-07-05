package checker

type scope struct {
	parent     *scope
	symbols    map[string]symbol
	returnType Type
	isolated   bool
}

type symbol interface {
	name() string
	_type() Type
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:  parent,
		symbols: map[string]symbol{},
	}
}

func (s *scope) add(sym symbol) {
	s.symbols[sym.name()] = sym
}
func (s *scope) get(name string) symbol {
	if sym, ok := s.symbols[name]; ok {
		return sym
	}
	if s.isolated {
		return nil
	}
	if s.parent != nil {
		return s.parent.get(name)
	}
	return nil
}
