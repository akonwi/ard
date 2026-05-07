package bytecode

import "github.com/akonwi/ard/air"

type ConstantKind uint8

const (
	ConstFloat ConstantKind = iota
	ConstStr
)

type Constant struct {
	Kind  ConstantKind
	Float float64
	Str   string
}

type Instruction struct {
	Op  Opcode
	A   int
	B   int
	C   int
	Imm int
}

type Function struct {
	ID       air.FunctionID
	Module   air.ModuleID
	Name     string
	Arity    int
	Return   air.TypeID
	Locals   int
	Captures []air.Capture
	Code     []Instruction
}

type Program struct {
	Constants []Constant
	Types     []air.TypeInfo
	Traits    []air.Trait
	Impls     []air.Impl
	Externs   []air.Extern
	Functions []Function
	Entry     air.FunctionID
	Script    air.FunctionID
}

func (p *Program) Function(id air.FunctionID) (*Function, bool) {
	if p == nil || id < 0 || int(id) >= len(p.Functions) {
		return nil, false
	}
	return &p.Functions[id], true
}

func (p *Program) TypeInfo(id air.TypeID) (air.TypeInfo, bool) {
	if p == nil || id <= 0 || int(id) > len(p.Types) {
		return air.TypeInfo{}, false
	}
	return p.Types[id-1], true
}
