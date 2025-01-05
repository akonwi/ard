package checker

type symbol interface {
	GetName() string
	GetType() Type
	asFunction() (function, bool)
}

type variable struct {
	name  string
	mut   bool
	_type Type
}

func (v variable) GetName() string {
	return v.name
}
func (v variable) GetType() Type {
	return v._type
}
func (v variable) asFunction() (function, bool) {
	if fn, ok := v._type.(function); ok {
		return fn, ok
	}
	return function{}, false
}

func (f function) GetName() string {
	return f.name
}
func (f function) GetType() Type {
	return f.returns
}
func (f function) asFunction() (function, bool) {
	return f, true
}

type scope struct {
	parent  *scope
	symbols map[string]symbol
}

func newScope(parent *scope) *scope {
	return &scope{
		parent:  parent,
		symbols: map[string]symbol{},
	}
}

func (s *scope) declare(sym symbol) bool {
	if _, ok := s.symbols[sym.GetName()]; ok {
		return false
	}
	s.symbols[sym.GetName()] = sym
	return true
}

func (s *scope) addVariable(v variable) bool {
	if _, ok := s.symbols[v.GetName()]; ok {
		return false
	}
	s.symbols[v.GetName()] = v
	return true
}

func (s scope) findVariable(name string) (variable, bool) {
	if symbol, ok := s.symbols[name]; ok {
		if variable, ok := symbol.(variable); ok {
			return variable, ok
		}
	}
	if s.parent != nil {
		return s.parent.findVariable(name)
	}
	return variable{}, false
}

func (s scope) find(name string) symbol {
	sym := s.symbols[name]
	if sym != nil {
		return sym
	}

	if s.parent != nil {
		return s.parent.find(name)
	}
	return nil
}
