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

func New() *VM {
	vm := &VM{
		scope:          newScope(nil),
		moduleRegistry: NewModuleRegistry(),
	}
	vm.initModuleRegistry()
	return vm
}

// initModuleRegistry initializes all built-in module handlers
// todo: only register the explicitly imported ones + prelude
func (vm *VM) initModuleRegistry() {
	// Register Int module (handles both Int prelude and ard/ints)
	vm.moduleRegistry.Register(&IntModule{})
	// Register Float module (handles both Float prelude and ard/float)
	vm.moduleRegistry.Register(&FloatModule{})
	// Register IO module (handles ard/io)
	vm.moduleRegistry.Register(&IOModule{})
	// Register FS module (handles ard/fs)
	vm.moduleRegistry.Register(&FSModule{})
	// Register Maybe module (handles ard/maybe)
	vm.moduleRegistry.Register(&MaybeModule{})
	// Register HTTP module (handles ard/http)
	vm.moduleRegistry.Register(&HTTPModule{})
	// Register Result module (handles both Result prelude and ard/result)
	vm.moduleRegistry.Register(&ResultModule{})
	// Register JSON module (handles ard/json)
	vm.moduleRegistry.Register(&JSONModule{})
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
	switch o._type.(type) {
	default:
		if o._type == checker.Str || o._type == checker.Int || o._type == checker.Float || o._type == checker.Bool {
			return o.raw
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
}
