package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

type VM struct {
	scope   *scope
	result  object
	imports map[string]checker.Module
}

func New() *VM {
	return &VM{scope: newScope(nil)}
}

func (vm *VM) pushScope() {
	vm.scope = newScope(vm.scope)
}

func (vm *VM) popScope() {
	vm.scope = vm.scope.parent
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
