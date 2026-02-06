package checker

import (
	"fmt"

	"github.com/akonwi/ard/parse"
)

type FiberExecution struct {
	module    Module
	_type     Type
	fnName    string
	FiberType Type
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

type FiberEval struct {
	fn        Expression
	_type     Type
	FiberType Type
}

func (f FiberEval) Type() Type {
	return f._type
}

func (f FiberEval) GetFn() Expression {
	return f.fn
}

func (c *Checker) validateFiberFunction(fnNode parse.Expression, fiberType Type) *FiberExecution {
	switch node := fnNode.(type) {
	case *parse.AnonymousFunction:
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

		// Specialize Fiber<Void> since async::start takes fn() Void
		specializedFiberType := c.specializeFiber(fiberType, Void)

		return &FiberExecution{
			module:    module,
			_type:     specializedFiberType,
			fnName:    "main",
			FiberType: specializedFiberType,
		}
	case *parse.StaticProperty:
		module := c.resolveModule(node.Target.String())

		if module == nil {
			c.addError(fmt.Sprintf("Module not found: %s", node.Target.String()), node.Location)
			return &FiberExecution{_type: fiberType, FiberType: fiberType}
		}

		// Specialize Fiber<Void> since async::start takes fn() Void
		specializedFiberType := c.specializeFiber(fiberType, Void)

		return &FiberExecution{
			module:    module,
			_type:     specializedFiberType,
			fnName:    node.Property.String(),
			FiberType: specializedFiberType,
		}
	default:
		// probably need to handle when the function is a variable reference
		panic(fmt.Sprintf("Unhandled fiber function node: %T", node))
	}

}

func (c *Checker) specializeFiber(fiberType Type, returnType Type) Type {
	// If the Fiber struct is generic, specialize $T with returnType
	// Use replaceGeneric to be consistent with specializeAliasedType
	return replaceGeneric(fiberType, "T", returnType)
}

func (c *Checker) validateAsyncEval(fnNode parse.Expression) *FiberEval {
	// For anonymous functions, check with isolation to prevent mutable variable capture
	// (same rules as async::start)
	var checkedFn Expression
	var fnDef *FunctionDef

	if anonFn, ok := fnNode.(*parse.AnonymousFunction); ok {
		// Check anonymous function with isolated scope (no mutable variable access)
		block := c.checkBlock(anonFn.Body, func() {
			c.scope.isolate()
		})
		params := c.resolveParametersWithContext(anonFn.Parameters, nil)
		returnType := c.resolveReturnTypeWithContext(anonFn.ReturnType, nil)

		uniqueName := fmt.Sprintf("eval_func_%p", fnNode)
		fnDef = &FunctionDef{
			Name:       uniqueName,
			Parameters: params,
			ReturnType: returnType,
			Body:       block,
		}
		checkedFn = fnDef
		c.scope.add(uniqueName, fnDef, false)
	} else {
		// For non-anonymous functions (variable references, etc.), check normally
		checkedFn = c.checkExpr(fnNode)
	}

	if checkedFn == nil {
		return &FiberEval{fn: nil, _type: Void, FiberType: Void}
	}

	// fnNode should be a function type that returns T
	// Extract the return type from the function type
	fnType, ok := checkedFn.Type().(*FunctionDef)
	if !ok {
		c.addError(fmt.Sprintf("async::eval expects a function argument, got %T", checkedFn.Type()), fnNode.GetLocation())
		return &FiberEval{fn: checkedFn, _type: Void, FiberType: Void}
	}

	returnType := fnType.ReturnType

	// If the anonymous function has no explicit return type (Void default) but has a body,
	// infer the return type from the body
	if returnType == Void && fnType.Body != nil {
		bodyType := fnType.Body.Type()
		if bodyType != Void {
			returnType = bodyType
		}
	}

	// Dereference the return type in case it's a TypeVar binding
	returnType = derefType(returnType)

	// FiberEval returns Fiber where result field has the closure's return type ($T)
	// Get the actual Fiber struct from the async module and specialize $T with returnType
	asyncMod := c.resolveModule("async")
	var fiberType Type = &StructDef{Name: "Fiber"}

	if asyncMod != nil {
		if fiberSym := asyncMod.Get("Fiber"); fiberSym.Type != nil {
			if fiberStructDef, ok := fiberSym.Type.(*StructDef); ok {
				// Create a specialized copy of the Fiber struct where $T is bound to returnType
				typeVarMap := make(map[string]*TypeVar)
				typeVarMap["T"] = &TypeVar{name: "T", actual: returnType, bound: true}

				fiberCopy := &StructDef{
					Name:    fiberStructDef.Name,
					Fields:  make(map[string]Type),
					Methods: make(map[string]*FunctionDef),
					Private: fiberStructDef.Private,
				}

				// Copy fields, replacing $T with the closure's return type
				for fieldName, fieldType := range fiberStructDef.Fields {
					fiberCopy.Fields[fieldName] = copyTypeWithTypeVarMap(fieldType, typeVarMap)
				}

				// Copy and specialize methods
				for methodName, methodDef := range fiberStructDef.Methods {
					methodCopy := copyFunctionWithTypeVarMap(methodDef, typeVarMap)
					fiberCopy.Methods[methodName] = methodCopy
				}

				fiberType = fiberCopy
			}
		}
	}

	return &FiberEval{
		fn:        checkedFn,
		_type:     fiberType,
		FiberType: fiberType,
	}
}
