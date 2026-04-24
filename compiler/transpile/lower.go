package transpile

import (
	"fmt"
	"go/ast"
	"go/token"
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
	emittedMethods := map[string]struct{}{}
	methodDefs := map[*checker.FunctionDef]struct{}{}
	methodBodies := map[*checker.Block]struct{}{}
	methodNames := map[string]struct{}{}
	for _, stmt := range module.Program().Statements {
		if stmt.Stmt == nil {
			continue
		}
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			for _, methodName := range sortedStringKeys(def.Methods) {
				method := def.Methods[methodName]
				methodDefs[method] = struct{}{}
				methodBodies[method.Body] = struct{}{}
				methodNames[method.Name] = struct{}{}
			}
		case checker.StructDef:
			for _, methodName := range sortedStringKeys(def.Methods) {
				method := def.Methods[methodName]
				methodDefs[method] = struct{}{}
				methodBodies[method.Body] = struct{}{}
				methodNames[method.Name] = struct{}{}
			}
		case *checker.Enum:
			for _, methodName := range sortedStringKeys(def.Methods) {
				method := def.Methods[methodName]
				methodDefs[method] = struct{}{}
				methodBodies[method.Body] = struct{}{}
				methodNames[method.Name] = struct{}{}
			}
		case checker.Enum:
			for _, methodName := range sortedStringKeys(def.Methods) {
				method := def.Methods[methodName]
				methodDefs[method] = struct{}{}
				methodBodies[method.Body] = struct{}{}
				methodNames[method.Name] = struct{}{}
			}
		}
	}

	for _, stmt := range module.Program().Statements {
		if stmt.Stmt == nil {
			continue
		}
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			decl, err := e.lowerStructTypeDeclNode(def)
			if err != nil {
				return goFileIR{}, err
			}
			appendASTDecl(&fileIR, decl)
			order, mapping, _ := structTypeParams(def)
			receiverType := ast.Expr(ast.NewIdent(goName(def.Name, true)))
			if len(order) > 0 {
				args := make([]ast.Expr, 0, len(order))
				for _, name := range order {
					args = append(args, ast.NewIdent(mapping[name]))
				}
				receiverType = indexExpr(receiverType, args)
			}
			for _, methodName := range sortedStringKeys(def.Methods) {
				methodKey := "struct:" + def.Name + "." + methodName
				if _, seen := emittedMethods[methodKey]; seen {
					continue
				}
				emittedMethods[methodKey] = struct{}{}
				decl, err := e.lowerReceiverMethodDeclNode(def.Name, receiverType, mapping, def.Methods[methodName])
				if err != nil {
					return goFileIR{}, err
				}
				appendASTDecl(&fileIR, decl)
			}
		case checker.StructDef:
			defCopy := def
			decl, err := e.lowerStructTypeDeclNode(&defCopy)
			if err != nil {
				return goFileIR{}, err
			}
			appendASTDecl(&fileIR, decl)
			order, mapping, _ := structTypeParams(&defCopy)
			receiverType := ast.Expr(ast.NewIdent(goName(defCopy.Name, true)))
			if len(order) > 0 {
				args := make([]ast.Expr, 0, len(order))
				for _, name := range order {
					args = append(args, ast.NewIdent(mapping[name]))
				}
				receiverType = indexExpr(receiverType, args)
			}
			for _, methodName := range sortedStringKeys(defCopy.Methods) {
				methodKey := "struct:" + defCopy.Name + "." + methodName
				if _, seen := emittedMethods[methodKey]; seen {
					continue
				}
				emittedMethods[methodKey] = struct{}{}
				decl, err := e.lowerReceiverMethodDeclNode(defCopy.Name, receiverType, mapping, defCopy.Methods[methodName])
				if err != nil {
					return goFileIR{}, err
				}
				appendASTDecl(&fileIR, decl)
			}
		case *checker.Enum:
			appendASTDecl(&fileIR, e.lowerEnumTypeDeclNode(def))
			receiverType := ast.Expr(ast.NewIdent(goName(def.Name, true)))
			for _, methodName := range sortedStringKeys(def.Methods) {
				methodKey := "enum:" + def.Name + "." + methodName
				if _, seen := emittedMethods[methodKey]; seen {
					continue
				}
				emittedMethods[methodKey] = struct{}{}
				decl, err := e.lowerReceiverMethodDeclNode(def.Name, receiverType, nil, def.Methods[methodName])
				if err != nil {
					return goFileIR{}, err
				}
				appendASTDecl(&fileIR, decl)
			}
		case checker.Enum:
			defCopy := def
			appendASTDecl(&fileIR, e.lowerEnumTypeDeclNode(&defCopy))
			receiverType := ast.Expr(ast.NewIdent(goName(defCopy.Name, true)))
			for _, methodName := range sortedStringKeys(defCopy.Methods) {
				methodKey := "enum:" + defCopy.Name + "." + methodName
				if _, seen := emittedMethods[methodKey]; seen {
					continue
				}
				emittedMethods[methodKey] = struct{}{}
				decl, err := e.lowerReceiverMethodDeclNode(defCopy.Name, receiverType, nil, defCopy.Methods[methodName])
				if err != nil {
					return goFileIR{}, err
				}
				appendASTDecl(&fileIR, decl)
			}
		case *checker.VariableDef:
			if entrypoint {
				continue
			}
			decl, ok, err := e.lowerPackageVariableDeclNode(def)
			if err != nil {
				return goFileIR{}, err
			}
			if !ok {
				return goFileIR{}, fmt.Errorf("unsupported package variable: %s", def.Name)
			}
			appendASTDecl(&fileIR, decl)
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
			if def.IsTest || def.Receiver != "" {
				continue
			}
			if _, isMethod := methodDefs[def]; isMethod {
				continue
			}
			if _, namedMethod := methodNames[def.Name]; namedMethod && def.Body != nil {
				if _, sameBody := methodBodies[def.Body]; sameBody {
					continue
				}
			}
			decl, err := e.lowerFunctionDeclNode(def)
			if err != nil {
				return goFileIR{}, err
			}
			appendASTDecl(&fileIR, decl)
		case *checker.ExternalFunctionDef:
			decl, err := e.lowerExternFunctionDeclNode(def)
			if err != nil {
				return goFileIR{}, err
			}
			appendASTDecl(&fileIR, decl)
		}
	}

	if entrypoint {
		if mainExpr := entrypointMainExpr(module.Program().Statements); mainExpr != nil {
			body := []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "RegisterBuiltinExterns")}},
			}
			mainName := e.functionNames["main"]
			if mainName == "" {
				mainName = "main"
			}
			call := &ast.CallExpr{Fun: ast.NewIdent(mainName)}
			switch typed := mainExpr.(type) {
			case *checker.FunctionDef:
				if effectiveFunctionReturnType(typed) == checker.Void {
					body = append(body, &ast.ExprStmt{X: call})
				} else {
					body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
				}
			case *checker.ExternalFunctionDef:
				if typed.ReturnType == checker.Void {
					body = append(body, &ast.ExprStmt{X: call})
				} else {
					body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
				}
			}
			appendASTDecl(&fileIR, &ast.FuncDecl{
				Name: ast.NewIdent("main"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{List: body},
			})
		} else {
			body := []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{Fun: selectorExpr(ast.NewIdent(helperImportAlias), "RegisterBuiltinExterns")}},
			}
			var block *ast.BlockStmt
			var ok bool
			var err error
			err = e.withFreshLocals(func() error {
				block, ok, err = e.lowerStatementsBlockAST(topLevelExecutableStatements(module.Program().Statements), nil)
				if err != nil {
					return err
				}
				if !ok {
					return errStructuredLoweringUnsupported
				}
				return nil
			})
			if err != nil {
				return goFileIR{}, err
			}
			body = append(body, block.List...)
			appendASTDecl(&fileIR, &ast.FuncDecl{
				Name: ast.NewIdent("main"),
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{List: body},
			})
		}
	}

	return fileIR, nil
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
