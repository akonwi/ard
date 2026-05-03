//go:build !vmnext_profile_detail

package vm_next

func (vm *VM) recordRefAccess(kind refAccessKind) {}

func (vm *VM) recordMaybeAccess(maybeValue *MaybeValue) {}

func (vm *VM) recordMaybeDetailAlloc(some bool) {}

func (vm *VM) recordZeroValue(kind zeroValueKind) {}
