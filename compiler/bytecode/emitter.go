package bytecode

import "github.com/akonwi/ard/checker"

type Emitter struct {
	program   Program
	typeIndex map[string]TypeID
}

func NewEmitter() *Emitter {
	return &Emitter{
		program: Program{
			Constants: []Constant{},
			Types:     []TypeEntry{},
			Functions: []Function{},
		},
		typeIndex: map[string]TypeID{},
	}
}

func (e *Emitter) EmitProgram(module checker.Module) (Program, error) {
	_ = module
	return e.program, nil
}

func (e *Emitter) addType(t checker.Type) TypeID {
	if t == nil {
		return 0
	}
	name := t.String()
	if id, ok := e.typeIndex[name]; ok {
		return id
	}
	id := TypeID(len(e.program.Types) + 1)
	e.program.Types = append(e.program.Types, TypeEntry{ID: id, Name: name})
	e.typeIndex[name] = id
	return id
}
