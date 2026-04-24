package transpile

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
)

func lowerModuleFileIR(module checker.Module, packageName string, entrypoint bool, projectName string) (goFileIR, error) {
	if module == nil || module.Program() == nil {
		return goFileIR{}, fmt.Errorf("module has no program")
	}

	e := &emitter{
		module:        module,
		packageName:   packageName,
		projectName:   projectName,
		entrypoint:    entrypoint,
		imports:       collectModuleImports(module.Program().Statements, projectName),
		functionNames: make(map[string]string),
		emittedTypes:  make(map[string]struct{}),
	}
	if entrypoint {
		e.imports[helperImportPath] = helperImportAlias
	}
	fileIR := lowerGoFileIR(packageName, e.imports)
	e.indexFunctions()

	for _, stmt := range module.Program().Statements {
		if stmt.Stmt == nil {
			continue
		}
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitStructDef(def) }); err != nil {
				return goFileIR{}, err
			}
		case checker.StructDef:
			defCopy := def
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitStructDef(&defCopy) }); err != nil {
				return goFileIR{}, err
			}
		case *checker.Enum:
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitEnumDef(def) }); err != nil {
				return goFileIR{}, err
			}
		case checker.Enum:
			defCopy := def
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitEnumDef(&defCopy) }); err != nil {
				return goFileIR{}, err
			}
		case *checker.VariableDef:
			if entrypoint {
				continue
			}
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitPackageVariable(def) }); err != nil {
				return goFileIR{}, err
			}
		case *checker.ExternType:
			continue
		default:
			if !entrypoint {
				return goFileIR{}, fmt.Errorf("unsupported top-level statement in imported module: %T", stmt.Stmt)
			}
		}
	}

	for _, stmt := range module.Program().Statements {
		if stmt.Expr == nil {
			continue
		}
		switch def := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if def.IsTest {
				continue
			}
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitFunction(def) }); err != nil {
				return goFileIR{}, err
			}
		case *checker.ExternalFunctionDef:
			if err := appendCapturedDecl(&fileIR, e, func() error { return e.emitExternFunction(def) }); err != nil {
				return goFileIR{}, err
			}
		}
	}

	if entrypoint {
		mainDecl, err := e.captureOutput(func() error {
			e.line("func main() {")
			e.indent++
			e.line(helperImportAlias + ".RegisterBuiltinExterns()")
			if mainExpr := entrypointMainExpr(module.Program().Statements); mainExpr != nil {
				mainName := e.functionNames["main"]
				if mainName == "" {
					mainName = "main"
				}
				switch typed := mainExpr.(type) {
				case *checker.FunctionDef:
					if effectiveFunctionReturnType(typed) == checker.Void {
						e.line(mainName + "()")
					} else {
						e.line("_ = " + mainName + "()")
					}
				case *checker.ExternalFunctionDef:
					if typed.ReturnType == checker.Void {
						e.line(mainName + "()")
					} else {
						e.line("_ = " + mainName + "()")
					}
				}
			} else {
				if err := e.withFreshLocals(func() error {
					return e.emitStatements(topLevelExecutableStatements(module.Program().Statements), nil)
				}); err != nil {
					return err
				}
			}
			e.indent--
			e.line("}")
			return nil
		})
		if err != nil {
			return goFileIR{}, err
		}
		if err := appendGoDeclIR(&fileIR, packageName, mainDecl); err != nil {
			return goFileIR{}, err
		}
	}

	return fileIR, nil
}

func appendCapturedDecl(fileIR *goFileIR, e *emitter, emit func() error) error {
	decl, err := e.captureOutput(emit)
	if err != nil {
		return err
	}
	return appendGoDeclIR(fileIR, e.packageName, decl)
}

func appendGoDeclIR(fileIR *goFileIR, packageName string, source string) error {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil
	}
	decls, err := parseGoDecls(packageName, trimmed)
	if err != nil {
		return err
	}
	fileIR.Decls = append(fileIR.Decls, goDeclIR{Decls: decls})
	return nil
}
