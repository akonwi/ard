package vm

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func runBytecodeErr(input string) error {
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		return fmt.Errorf("parse errors: %v", result.Errors[0].Message)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return err
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		return fmt.Errorf("diagnostics found: %v", c.Diagnostics())
	}
	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		return err
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		return err
	}
	_, err = New(program).Run("main")
	return err
}

func TestBytecodeConcurrentMethodAccess(t *testing.T) {
	const workers = 50
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runBytecodeErr(`
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

func TestBytecodeConcurrentMethodAccessWithMultipleMethods(t *testing.T) {
	const workers = 60
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				errCh <- runBytecodeErr(`
					mut list = [1,2,3]
					list.push(4)
					list.size()
				`)
			} else {
				errCh <- runBytecodeErr(`
					let s = "hello world"
					s.replace_all("world", "ard")
				`)
			}
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

func TestBytecodeConcurrentModuleAccess(t *testing.T) {
	const workers = 50
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- runBytecodeErr(`
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
