package vm_next

import (
	"fmt"
	"sync"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestVMNextBytecodeParityConcurrentMethodAccess(t *testing.T) {
	const workers = 50
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runVMNextErr(`
				let nums = [3,1,2]
				List::map(nums, fn(n: Int) Int { n + 1 }).size()
			`)
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent run failed: %v", err)
		}
	}
}

func TestVMNextBytecodeParityConcurrentMethodAccessWithMultipleMethods(t *testing.T) {
	const workers = 60
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				errCh <- runVMNextErr(`
					mut list = [1,2,3]
					list.push(4)
					list.size()
				`)
				return
			}
			errCh <- runVMNextErr(`
				let s = "hello world"
				s.replace_all("world", "ard")
			`)
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent mixed run failed: %v", err)
		}
	}
}

func TestVMNextBytecodeParityConcurrentModuleAccess(t *testing.T) {
	const workers = 50
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runVMNextErr(`
				use ard/decode
				let d = decode::from_json("[1,2,3]").expect("")
				decode::run(d, decode::list(decode::int)).expect("").size()
			`)
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent module run failed: %v", err)
		}
	}
}

func runVMNextErr(input string) error {
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		return fmt.Errorf("parse errors: %v", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		return fmt.Errorf("diagnostics found: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		return err
	}
	vm, err := NewWithExterns(program, nil)
	if err != nil {
		return err
	}
	_, err = vm.RunScript()
	return err
}
