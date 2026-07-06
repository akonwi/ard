package gotarget

// This file centralizes Go identifier reservation for generated code. The
// lowerer uses these helpers to avoid emitting locals, functions, globals, or
// types that collide with Go predeclared names, runtime helper names, or other
// generated top-level declarations.

import "github.com/akonwi/ard/air"

func predeclaredGoIdentifiers() []string {
	return []string{"any", "bool", "byte", "comparable", "complex64", "complex128", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "true", "false", "iota", "nil", "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover"}
}

func runtimePreludeTopLevelNames() []string {
	return []string{"Maybe", "Result"}
}

func collectTopLevelReservedNames(program *air.Program) map[string]bool {
	reserved := map[string]bool{}
	if program == nil {
		return reserved
	}
	for _, typ := range program.Types {
		if typ.Name != "" {
			reserved[typeName(program, typ)] = true
		}
	}
	for _, global := range program.Globals {
		if global.Name != "" {
			reserved[globalName(program, global)] = true
		}
	}
	for _, fn := range program.Functions {
		if fn.Name != "" {
			reserved[functionName(program, fn)] = true
		}
	}
	return reserved
}
