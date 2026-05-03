//go:build vmnext_profile_detail

package vm_next

func (vm *VM) recordRefAccess(kind refAccessKind) {
	if vm == nil || vm.profile == nil {
		return
	}
	vm.profile.RecordRefAccess(kind)
}

func (vm *VM) recordMaybeAccess(maybeValue *MaybeValue) {
	if vm == nil || vm.profile == nil || maybeValue == nil {
		return
	}
	vm.profile.RecordMaybeAccess(maybeValue.Some)
}

func (vm *VM) recordMaybeDetailAlloc(some bool) {
	vm.recordMaybeAlloc(some)
}

func (vm *VM) recordZeroValue(kind zeroValueKind) {
	if vm == nil || vm.profile == nil {
		return
	}
	vm.profile.RecordZeroValue(kind)
}
