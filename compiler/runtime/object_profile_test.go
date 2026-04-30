package runtime

import (
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestObjectProfileTracksConstructorsAndAllocations(t *testing.T) {
	EnableObjectProfiling(true)
	ResetObjectProfile()
	defer EnableObjectProfiling(false)

	_ = MakeStr("hello")
	_ = MakeInt(7)
	_ = MakeInt(300)
	_ = MakeBool(true)
	_ = MakeDynamic("raw")
	_ = MakeList(checker.Dynamic)
	_ = Void()

	snapshot := SnapshotObjectProfile()
	if !snapshot.Enabled {
		t.Fatal("expected profiling to be enabled")
	}
	if snapshot.MakeStrCalls != 1 {
		t.Fatalf("expected 1 string constructor call, got %d", snapshot.MakeStrCalls)
	}
	if snapshot.MakeIntCalls != 2 {
		t.Fatalf("expected 2 int constructor calls, got %d", snapshot.MakeIntCalls)
	}
	if snapshot.IntCacheHits != 1 {
		t.Fatalf("expected 1 int cache hit, got %d", snapshot.IntCacheHits)
	}
	if snapshot.MakeBoolCalls != 1 || snapshot.BoolCacheHits != 1 {
		t.Fatalf("expected bool call/cache hit to be 1/1, got %d/%d", snapshot.MakeBoolCalls, snapshot.BoolCacheHits)
	}
	if snapshot.MakeDynamicCalls != 1 {
		t.Fatalf("expected 1 dynamic constructor call, got %d", snapshot.MakeDynamicCalls)
	}
	if snapshot.MakeListCalls != 1 {
		t.Fatalf("expected 1 list constructor call, got %d", snapshot.MakeListCalls)
	}
	if snapshot.VoidCalls != 1 || snapshot.VoidCacheHits != 1 {
		t.Fatalf("expected void call/cache hit to be 1/1, got %d/%d", snapshot.VoidCalls, snapshot.VoidCacheHits)
	}
	if snapshot.TotalAllocations != 4 {
		t.Fatalf("expected 4 fresh allocations, got %d", snapshot.TotalAllocations)
	}
}
