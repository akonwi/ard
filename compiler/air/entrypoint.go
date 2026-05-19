package air

import "fmt"

func ValidateEntrypointSignature(program *Program) error {
	if program == nil || program.Entry == NoFunction {
		return nil
	}
	if int(program.Entry) < 0 || int(program.Entry) >= len(program.Functions) {
		return fmt.Errorf("entrypoint function %d out of range", program.Entry)
	}
	entry := program.Functions[program.Entry]
	if len(entry.Signature.Params) != 0 {
		return fmt.Errorf("main entrypoint cannot have parameters")
	}
	if int(entry.Signature.Return) <= 0 || int(entry.Signature.Return) > len(program.Types) {
		return fmt.Errorf("main entrypoint return type %d out of range", entry.Signature.Return)
	}
	returnType := program.Types[entry.Signature.Return-1]
	if returnType.Kind != TypeVoid {
		return fmt.Errorf("main entrypoint must return Void, got %s", returnType.Name)
	}
	return nil
}
