package checker

import (
	"fmt"

	"github.com/akonwi/ard/ast"
)

type FiberExecution struct {
	module Module
	_type  Type
}

func (f FiberExecution) Type() Type {
	return f._type
}

func (f FiberExecution) GetModule() Module {
	return f.module
}

func (c *checker) validateFiberFunction(fnNode ast.Expression, fiberType Type) *FiberExecution {
	switch node := fnNode.(type) {
	case *ast.AnonymousFunction:
		block := c.checkBlock(node.Body, func() {
			// do not inherit parent scope
			c.scope.isolate()
		})
		main := &FunctionDef{
			Name:       "main",
			Parameters: []Parameter{},
			ReturnType: Void,
			Body:       block,
		}
		module := NewUserModule(c.filePath, &Program{
			Imports: c.program.Imports,
			Statements: []Statement{
				{Expr: main},
			},
		}, &SymbolTable{})
		return &FiberExecution{
			module: module,
			_type:  fiberType,
		}
	default:
		// probably need to handle when the function is a variable reference
		panic(fmt.Sprintf("Unhandled fiber function node: %T", node))
	}

}
