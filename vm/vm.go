package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

type VM struct {
	scope          *scope
	result         object
	imports        map[string]checker.Module
	moduleRegistry *ModuleRegistry
}

func New(imports map[string]checker.Module) *VM {
	vm := &VM{
		scope:          newScope(nil),
		moduleRegistry: NewModuleRegistry(),
		imports:        imports,
	}
	vm.initModuleRegistry()
	return vm
}

func (vm *VM) initModuleRegistry() {
	// <prelude>
	vm.moduleRegistry.Register(&IntModule{})
	vm.moduleRegistry.Register(&FloatModule{})
	vm.moduleRegistry.Register(&ResultModule{})
	vm.moduleRegistry.Register(&ListModule{})
	// </prelude>

	for path := range vm.imports {
		switch path {
		case "ard/io":
			vm.moduleRegistry.Register(&IOModule{})
		case "ard/fs":
			vm.moduleRegistry.Register(&FSModule{})
		case "ard/maybe":
			vm.moduleRegistry.Register(&MaybeModule{})
		case "ard/http":
			vm.moduleRegistry.Register(&HTTPModule{})
		case "ard/json":
			vm.moduleRegistry.Register(&JSONModule{})
		case "ard/sqlite":
			vm.moduleRegistry.Register(&SQLiteModule{})
		case "ard/async":
			vm.moduleRegistry.Register(&AsyncModule{})
		}
	}
}

func (vm *VM) pushScope() {
	vm.scope = newScope(vm.scope)
}

func (vm *VM) popScope() {
	vm.scope = vm.scope.parent
}

// Eval implements VMEvaluator interface
func (vm *VM) Eval(expr checker.Expression) *object {
	return vm.eval(expr)
}

type object struct {
	raw   any
	_type checker.Type
}

func (o object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o *object) premarshal() any {
	if o._type == checker.Void || o._type == nil {
		return nil
	}
	if o._type == checker.Str || o._type == checker.Int || o._type == checker.Float || o._type == checker.Bool {
		return o.raw
	}

	switch o._type.(type) {
	case *checker.Enum:
		return o.raw
	case *checker.Maybe:
		if o.raw == nil {
			return nil
		}
		if inner, ok := o.raw.(*object); ok {
			return inner.premarshal()
		}
		return o.raw
	case *checker.List:
		raw := o.raw.([]*object)
		_array := make([]any, len(raw))
		for i, item := range raw {
			_array[i] = item.premarshal()
		}
		return _array
	}

	if _, isStruct := o._type.(*checker.StructDef); isStruct {
		m := o.raw.(map[string]*object)
		rawMap := make(map[string]any)
		for key, value := range m {
			rawMap[key] = value.premarshal()
		}
		return rawMap
	}
	panic(fmt.Sprintf("Cannot marshall type: %T", o._type))
}
