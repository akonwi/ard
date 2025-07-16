package checker

type SymbolTable struct {
	parent  *SymbolTable
	symbols map[string]*Symbol

	// for scopes that expect a return value
	returnType Type

	// isolated means only read-only references in outer scopes are allowed
	isolated bool
}

type Symbol struct {
	Name    string
	Type    Type
	mutable bool
	private bool
}

// temporarily implement legacy symbol interface while that interface gets refactored out
func (s *Symbol) name() string {
	return s.Name
}

func (s *Symbol) _type() Type {
	return s.Type
}

func makeScope(parent *SymbolTable) SymbolTable {
	return SymbolTable{
		parent:  parent,
		symbols: map[string]*Symbol{},
	}
}

func (st *SymbolTable) add(name string, type_ Type, mutable bool) {
	sym := Symbol{
		Name:    name,
		Type:    type_,
		mutable: mutable,
	}
	st.symbols[name] = &sym
}

func (st *SymbolTable) addPrivate(name string, type_ Type, mutable bool) {
	st.add(name, type_, mutable)
	st.symbols[name].private = true
}

func (st *SymbolTable) get(name string) (*Symbol, bool) {
	if sym, ok := st.symbols[name]; ok {
		return sym, true
	}

	if st.isolated {
		return nil, false
	}

	if st.parent != nil {
		return st.parent.get(name)
	}
	return nil, false
}

func (st *SymbolTable) expectReturn(returnType Type) {
	st.returnType = returnType
}

func (st *SymbolTable) isolate() {
	st.isolated = true
}

type symbol interface {
	name() string
	_type() Type
}
