package runtime

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type objectProfileCounters struct {
	enabled               atomic.Bool
	totalConstructorCalls atomic.Uint64
	totalAllocations      atomic.Uint64
	makeStrCalls          atomic.Uint64
	makeIntCalls          atomic.Uint64
	intCacheHits          atomic.Uint64
	makeFloatCalls        atomic.Uint64
	makeBoolCalls         atomic.Uint64
	boolCacheHits         atomic.Uint64
	makeNoneCalls         atomic.Uint64
	makeListCalls         atomic.Uint64
	makeMapCalls          atomic.Uint64
	makeStructCalls       atomic.Uint64
	makeDynamicCalls      atomic.Uint64
	makeGenericCalls      atomic.Uint64
	genericAllocations    atomic.Uint64
	copyCalls             atomic.Uint64
	voidCalls             atomic.Uint64
	voidCacheHits         atomic.Uint64
}

var objectProfile objectProfileCounters

type ObjectProfileSnapshot struct {
	Enabled               bool
	TotalConstructorCalls uint64
	TotalAllocations      uint64
	MakeStrCalls          uint64
	MakeIntCalls          uint64
	IntCacheHits          uint64
	MakeFloatCalls        uint64
	MakeBoolCalls         uint64
	BoolCacheHits         uint64
	MakeNoneCalls         uint64
	MakeListCalls         uint64
	MakeMapCalls          uint64
	MakeStructCalls       uint64
	MakeDynamicCalls      uint64
	MakeGenericCalls      uint64
	GenericAllocations    uint64
	CopyCalls             uint64
	VoidCalls             uint64
	VoidCacheHits         uint64
}

func EnableObjectProfiling(enabled bool) {
	objectProfile.enabled.Store(enabled)
}

func ObjectProfilingEnabled() bool {
	return objectProfile.enabled.Load()
}

func ResetObjectProfile() {
	objectProfile.totalConstructorCalls.Store(0)
	objectProfile.totalAllocations.Store(0)
	objectProfile.makeStrCalls.Store(0)
	objectProfile.makeIntCalls.Store(0)
	objectProfile.intCacheHits.Store(0)
	objectProfile.makeFloatCalls.Store(0)
	objectProfile.makeBoolCalls.Store(0)
	objectProfile.boolCacheHits.Store(0)
	objectProfile.makeNoneCalls.Store(0)
	objectProfile.makeListCalls.Store(0)
	objectProfile.makeMapCalls.Store(0)
	objectProfile.makeStructCalls.Store(0)
	objectProfile.makeDynamicCalls.Store(0)
	objectProfile.makeGenericCalls.Store(0)
	objectProfile.genericAllocations.Store(0)
	objectProfile.copyCalls.Store(0)
	objectProfile.voidCalls.Store(0)
	objectProfile.voidCacheHits.Store(0)
}

func SnapshotObjectProfile() ObjectProfileSnapshot {
	return ObjectProfileSnapshot{
		Enabled:               ObjectProfilingEnabled(),
		TotalConstructorCalls: objectProfile.totalConstructorCalls.Load(),
		TotalAllocations:      objectProfile.totalAllocations.Load(),
		MakeStrCalls:          objectProfile.makeStrCalls.Load(),
		MakeIntCalls:          objectProfile.makeIntCalls.Load(),
		IntCacheHits:          objectProfile.intCacheHits.Load(),
		MakeFloatCalls:        objectProfile.makeFloatCalls.Load(),
		MakeBoolCalls:         objectProfile.makeBoolCalls.Load(),
		BoolCacheHits:         objectProfile.boolCacheHits.Load(),
		MakeNoneCalls:         objectProfile.makeNoneCalls.Load(),
		MakeListCalls:         objectProfile.makeListCalls.Load(),
		MakeMapCalls:          objectProfile.makeMapCalls.Load(),
		MakeStructCalls:       objectProfile.makeStructCalls.Load(),
		MakeDynamicCalls:      objectProfile.makeDynamicCalls.Load(),
		MakeGenericCalls:      objectProfile.makeGenericCalls.Load(),
		GenericAllocations:    objectProfile.genericAllocations.Load(),
		CopyCalls:             objectProfile.copyCalls.Load(),
		VoidCalls:             objectProfile.voidCalls.Load(),
		VoidCacheHits:         objectProfile.voidCacheHits.Load(),
	}
}

func ObjectProfileReport() string {
	snapshot := SnapshotObjectProfile()
	if !snapshot.Enabled {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "[ard runtime object profile]\n")
	fmt.Fprintf(&out, "constructor_calls=%d fresh_allocations=%d\n", snapshot.TotalConstructorCalls, snapshot.TotalAllocations)
	fmt.Fprintf(&out, "make_str=%d make_int=%d int_cache_hits=%d make_float=%d\n", snapshot.MakeStrCalls, snapshot.MakeIntCalls, snapshot.IntCacheHits, snapshot.MakeFloatCalls)
	fmt.Fprintf(&out, "make_bool=%d bool_cache_hits=%d make_none=%d void_calls=%d void_cache_hits=%d\n", snapshot.MakeBoolCalls, snapshot.BoolCacheHits, snapshot.MakeNoneCalls, snapshot.VoidCalls, snapshot.VoidCacheHits)
	fmt.Fprintf(&out, "make_dynamic=%d make_list=%d make_map=%d make_struct=%d\n", snapshot.MakeDynamicCalls, snapshot.MakeListCalls, snapshot.MakeMapCalls, snapshot.MakeStructCalls)
	fmt.Fprintf(&out, "make_generic=%d generic_allocations=%d copy_calls=%d\n", snapshot.MakeGenericCalls, snapshot.GenericAllocations, snapshot.CopyCalls)
	return strings.TrimRight(out.String(), "\n")
}

func recordObjectConstructor(counter *atomic.Uint64) bool {
	if !objectProfile.enabled.Load() {
		return false
	}
	objectProfile.totalConstructorCalls.Add(1)
	counter.Add(1)
	return true
}

func recordObjectAllocation() {
	if !objectProfile.enabled.Load() {
		return
	}
	objectProfile.totalAllocations.Add(1)
}
