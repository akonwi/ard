package ffi

import "testing"

func TestNewHostFunctionsInjectsOSArgs(t *testing.T) {
	injected := []string{"ard", "run", "main.ard", "one"}
	functions := NewHostFunctions(HostConfig{Args: injected})
	injected[0] = "mutated"
	osArgs, ok := functions["OsArgs"].(func() []string)
	if !ok {
		t.Fatalf("OsArgs has type %T, want func() []string", functions["OsArgs"])
	}

	args := osArgs()
	want := []string{"ard", "run", "main.ard", "one"}
	if len(args) != len(want) {
		t.Fatalf("OsArgs len = %d, want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("OsArgs[%d] = %q, want %q (%v)", i, args[i], want[i], args)
		}
	}

	args[0] = "mutated"
	if got := osArgs()[0]; got != "ard" {
		t.Fatalf("OsArgs returned mutable backing storage; got first arg %q", got)
	}
}
