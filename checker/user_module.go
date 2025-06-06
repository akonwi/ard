package checker

// UserModule represents a user-defined module that implements the Module interface
type UserModule struct {
	filePath      string
	publicSymbols map[string]symbol // only public symbols from the checked program
}

// path returns the file path for this module
func (m *UserModule) path() string {
	return m.filePath
}

// buildScope adds this module's public symbols to the given scope
func (m *UserModule) buildScope(scope *scope) {
	for _, sym := range m.publicSymbols {
		scope.add(sym)
	}
}

// get returns a public symbol by name, or nil if not found or private
func (m *UserModule) get(name string) symbol {
	return m.publicSymbols[name] // returns nil if not found
}

// Get is the exported version of get for testing
func (m *UserModule) Get(name string) symbol {
	return m.get(name)
}

// setFilePath sets the file path for this module
func (m *UserModule) setFilePath(path string) {
	m.filePath = path
}

// NewUserModule creates a UserModule from a checked program, extracting only public symbols
func NewUserModule(filePath string, program *Program, globalScope *scope) *UserModule {
	publicSymbols := make(map[string]symbol)
	
	// Extract public symbols from the global scope
	for _, sym := range globalScope.symbols {
		switch s := sym.(type) {
		case *FunctionDef:
			if s.Public {
				publicSymbols[s.Name] = s
			}
		case *StructDef:
			if s.Public {
				publicSymbols[s.Name] = s
			}
		// TODO: Add other symbol types (TypeDef, TraitDef, etc.) when they have Public fields
		}
	}
	
	return &UserModule{
		filePath:      filePath,
		publicSymbols: publicSymbols,
	}
}
