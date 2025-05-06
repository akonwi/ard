package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker_v2"
)

type VM struct {
	scope  *scope
	result object
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

func (vm *VM) addVariable(name string, value *object) {
	vm.scope.bindings[name] = value
}

type object struct {
	raw   any
	_type any
}

func areEqual(a, b *object) bool {
	if a.raw == b.raw {
		if a._type == b._type {
			return true
		}

		return a._type.(checker_v2.Type).String() == b._type.(checker_v2.Type).String()
	}
	return false
}

func (o *object) isCallable() bool {
	_, isFunc := o.raw.(func(args ...object) object)
	return isFunc
}

func (o object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o *object) premarshal() any {
	switch o._type.(type) {
	default:
		if o._type == checker_v2.Str || o._type == checker_v2.Int || o._type == checker_v2.Float || o._type == checker_v2.Bool {
			return o.raw
		}
		if _, isStruct := o._type.(*checker_v2.StructDef); isStruct {
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
