package checker_v2

type scope struct {
	parent  *scope
	symbols map[string]symbol
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
func (s *scope) getVar(name string) symbol {
	if sym, ok := s.symbols[name]; ok {
		return sym
	}
	if s.parent != nil {
		return s.parent.getVar(name)
	}
	return nil
}
