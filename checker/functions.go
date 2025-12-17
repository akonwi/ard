package checker

import (
	"fmt"

	"github.com/akonwi/ard/ast"
)

type FiberExecution struct {
	module Module
	_type  Type
	fnName string
}

func (f FiberExecution) Type() Type {
	return f._type
}

func (f FiberExecution) GetModule() Module {
	return f.module
}

func (f FiberExecution) GetMainName() string {
	return f.fnName
}

func (c *Checker) validateFiberFunction(fnNode ast.Expression, fiberType Type) *FiberExecution {
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
		}, &SymbolTable{}, c.types)
		return &FiberExecution{
			module: module,
			_type:  fiberType,
			fnName: "main",
		}
	case *ast.StaticProperty:
		module := c.resolveModule(node.Target.String())

		if module == nil {
			c.addError(fmt.Sprintf("Module not found: %s", node.Target.String()), node.Location)
			return &FiberExecution{_type: fiberType}
		}

		return &FiberExecution{
			module: module,
			_type:  fiberType,
			fnName: node.Property.String(),
		}
	default:
		// probably need to handle when the function is a variable reference
		panic(fmt.Sprintf("Unhandled fiber function node: %T", node))
	}

}
