package vm

import (
	"errors"

	"github.com/akonwi/ard/checker"
)

// Runs the given program
func Run(program *checker.Program) error {
	hasMain := false
	for _, stmt := range program.Statements {
		if stmt.Expr == nil {
			continue
		}
		if fn, ok := stmt.Expr.Type().(*checker.FunctionDef); ok {
			if fn.Name == "main" && len(fn.Parameters) == 0 && fn.ReturnType == checker.Void {
				hasMain = true
				break
			}
		}
	}

	if !hasMain {
		return errors.New("No main function found")
	}

	vm := New(program.Imports)
	// setup module scope
	if _, err := vm.Interpret(program); err != nil {
		return err
	}

	return vm.callMain()
}
