package checker

type symbol interface {
	GetName() string
	GetType() Type
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

type scope struct {
	symbols map[string]symbol
}

func NewScope() scope {
	return scope{
		symbols: map[string]symbol{},
	}
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
	return variable{}, false
}

func (s scope) find(name string) symbol {
	return s.symbols[name]
}