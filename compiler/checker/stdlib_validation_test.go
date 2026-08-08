package checker

import (
	"sync"
	"testing"

	"github.com/akonwi/ard/parse"
	std_lib "github.com/akonwi/ard/std_lib"
)

func TestFindEmbeddedModuleCachesConcurrentLoads(t *testing.T) {
	const callers = 16
	type result struct {
		module Module
		ok     bool
	}

	start := make(chan struct{})
	results := make(chan result, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			<-start
			module, ok := FindEmbeddedModule("ard/list")
			results <- result{module: module, ok: ok}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	var cachedProgram *Program
	for loaded := range results {
		if !loaded.ok || loaded.module == nil {
			t.Fatal("embedded ard/list module was not found")
		}
		if cachedProgram == nil {
			cachedProgram = loaded.module.Program()
			continue
		}
		if loaded.module.Program() != cachedProgram {
			t.Fatal("concurrent embedded module loads did not reuse the checked program")
		}
	}
}

func TestCachedEmbeddedModuleGenericBindingsAreCheckLocal(t *testing.T) {
	sources := []string{
		"use ard/list\nlet values: [Int] = list::new<Int>()\n",
		"use ard/list\nlet values: [Str] = list::new<Str>()\n",
	}
	programs := make([]*parse.Program, 16)
	for i := range programs {
		result := parse.Parse([]byte(sources[i%len(sources)]), "test.ard")
		if len(result.Errors) > 0 {
			t.Fatalf("parse errors: %v", result.Errors)
		}
		programs[i] = result.Program
	}

	start := make(chan struct{})
	diagnostics := make(chan []Diagnostic, len(programs))
	var workers sync.WaitGroup
	workers.Add(len(programs))
	for _, program := range programs {
		go func() {
			defer workers.Done()
			<-start
			checked := New("test.ard", program, nil)
			checked.Check()
			diagnostics <- checked.Diagnostics()
		}()
	}
	close(start)
	workers.Wait()
	close(diagnostics)

	for got := range diagnostics {
		if len(got) > 0 {
			t.Fatalf("cached embedded generic bindings leaked between checks: %v", got)
		}
	}
}

// TestEmbeddedStdLibModulesCheckCleanly gates the shipped standard library on
// the current checker rules. FindEmbeddedModule intentionally tolerates
// diagnostics at import time (issue #350), so without this gate an invalid
// stdlib body would silently flow into every importing program.
func TestEmbeddedStdLibModulesCheckCleanly(t *testing.T) {
	modules := std_lib.Modules()
	if len(modules) == 0 {
		t.Fatal("no embedded std_lib modules found")
	}
	for _, path := range modules {
		t.Run(path, func(t *testing.T) {
			content, err := std_lib.Find(path)
			if err != nil {
				t.Fatalf("read embedded module: %v", err)
			}
			result := parse.Parse(content, path)
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			_, diagnostics := check(result.Program, nil, path, path, CheckOptions{})
			for _, diagnostic := range diagnostics {
				if diagnostic.Kind == Error {
					t.Errorf("%s: %s", diagnostic.Primary.Span.Location.Start, diagnostic.Message)
				}
			}
		})
	}
}
