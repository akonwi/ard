package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"
	"sort"

	"github.com/akonwi/ard/air"
)

type loweredExpr struct {
	stmts []ast.Stmt
	expr  ast.Expr
}

type lowerer struct {
	program        *air.Program
	packageName    string
	tempCounter    int
	currentImports map[string]string
	declaredLocals map[air.LocalID]bool
}

func lowerProgram(program *air.Program, options Options) (map[string]*ast.File, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	l := &lowerer{program: program, packageName: defaultPackageName(options.PackageName)}
	files := map[string]*ast.File{}
	for _, module := range program.Modules {
		file, err := l.lowerModule(module)
		if err != nil {
			return nil, err
		}
		files[moduleFileName(module)] = file
	}
	return files, nil
}

func (l *lowerer) lowerModule(module air.Module) (*ast.File, error) {
	l.currentImports = map[string]string{}
	decls := []ast.Decl{}
	rootID, err := rootFunction(l.program)
	if err != nil {
		return nil, err
	}
	if module.ID == l.program.Functions[rootID].Module {
		for _, typ := range l.program.Types {
			typeDecls, err := l.lowerTypeDecls(typ)
			if err != nil {
				return nil, fmt.Errorf("module %s type %s: %w", module.Path, typ.Name, err)
			}
			decls = append(decls, typeDecls...)
		}
	}
	functionIDs := append([]air.FunctionID(nil), module.Functions...)
	sort.Slice(functionIDs, func(i, j int) bool { return functionIDs[i] < functionIDs[j] })
	for _, functionID := range functionIDs {
		fn := l.program.Functions[functionID]
		decl, err := l.lowerFunction(fn)
		if err != nil {
			return nil, fmt.Errorf("module %s function %s: %w", module.Path, fn.Name, err)
		}
		decls = append(decls, decl)
	}
	if module.ID == l.program.Functions[rootID].Module {
		mainDecl, err := l.lowerMainWrapper(rootID)
		if err != nil {
			return nil, err
		}
		decls = append(decls, mainDecl)
	}
	if len(l.currentImports) > 0 {
		importDecl := &ast.GenDecl{Tok: token.IMPORT}
		aliases := make([]string, 0, len(l.currentImports))
		for alias := range l.currentImports {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
				Name: ast.NewIdent(alias),
				Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", l.currentImports[alias])},
			})
		}
		decls = append([]ast.Decl{importDecl}, decls...)
	}
	return &ast.File{Name: ast.NewIdent(l.packageName), Decls: decls}, nil
}

func (l *lowerer) lowerTypeDecls(typ air.TypeInfo) ([]ast.Decl, error) {
	switch typ.Kind {
	case air.TypeStruct:
		fields := make([]*ast.Field, 0, len(typ.Fields))
		for _, field := range typ.Fields {
			fieldType, err := l.goType(field.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(field.Name)}, Type: fieldType})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeEnum:
		specs := []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: ast.NewIdent("int")}}
		for _, variant := range typ.Variants {
			specs = append(specs, &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(enumVariantName(l.program, typ, variant))}, Type: ast.NewIdent(typeName(l.program, typ)), Values: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", variant.Discriminant)}}})
		}
		decls := []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: specs[:1]}}
		if len(specs) > 1 {
			decls = append(decls, &ast.GenDecl{Tok: token.CONST, Specs: specs[1:]})
		}
		return decls, nil
	default:
		return nil, nil
	}
}

func (l *lowerer) lowerMainWrapper(root air.FunctionID) (ast.Decl, error) {
	fn := l.program.Functions[root]
	call := &ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, fn))}
	body := []ast.Stmt{}
	for _, param := range fn.Signature.Params {
		_ = param
		return nil, fmt.Errorf("entry function parameters are not supported yet")
	}
	if l.isVoidType(fn.Signature.Return) {
		body = append(body, &ast.ExprStmt{X: call})
	} else {
		body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent("main"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: body},
	}, nil
}

func (l *lowerer) lowerFunction(fn air.Function) (ast.Decl, error) {
	l.declaredLocals = map[air.LocalID]bool{}
	params := []*ast.Field{}
	for _, param := range fn.Signature.Params {
		paramType, err := l.goType(param.Type)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(sanitizeName(param.Name))},
			Type:  paramType,
		})
	}
	for _, local := range fn.Locals {
		if int(local.ID) < len(fn.Signature.Params) {
			l.declaredLocals[local.ID] = true
		}
	}
	body, err := l.lowerBlock(fn, fn.Body, fn.Signature.Return)
	if err != nil {
		return nil, err
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !l.isVoidType(fn.Signature.Return) {
		returnType, err := l.goType(fn.Signature.Return)
		if err != nil {
			return nil, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(functionName(l.program, fn)),
		Type: funcType,
		Body: body,
	}, nil
}

func (l *lowerer) lowerBlock(fn air.Function, block air.Block, returnType air.TypeID) (*ast.BlockStmt, error) {
	stmts := []ast.Stmt{}
	for _, stmt := range block.Stmts {
		lowered, err := l.lowerStmt(fn, stmt)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, lowered...)
	}
	if block.Result != nil {
		result, err := l.lowerExpr(fn, *block.Result)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, result.stmts...)
		if returnType == air.NoType || l.isVoidType(returnType) {
			if !isVoidExpr(result.expr) {
				stmts = append(stmts, &ast.ExprStmt{X: result.expr})
			}
		} else {
			stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{result.expr}})
		}
	}
	return &ast.BlockStmt{List: stmts}, nil
}

func (l *lowerer) lowerStmt(fn air.Function, stmt air.Stmt) ([]ast.Stmt, error) {
	switch stmt.Kind {
	case air.StmtLet:
		if stmt.Value == nil {
			return nil, fmt.Errorf("let statement missing value")
		}
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return nil, err
		}
		name := localName(fn, stmt.Local)
		out := append([]ast.Stmt{}, value.stmts...)
		tok := token.DEFINE
		if l.declaredLocals[stmt.Local] {
			tok = token.ASSIGN
		} else {
			l.declaredLocals[stmt.Local] = true
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(name)},
			Tok: tok,
			Rhs: []ast.Expr{value.expr},
		})
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(name)}})
		return out, nil
	case air.StmtAssign:
		if stmt.Value == nil {
			return nil, fmt.Errorf("assign statement missing value")
		}
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, value.stmts...)
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(localName(fn, stmt.Local))},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value.expr},
		})
		return out, nil
	case air.StmtExpr:
		if stmt.Expr == nil {
			return nil, fmt.Errorf("expr statement missing expression")
		}
		expr, err := l.lowerExpr(fn, *stmt.Expr)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, expr.stmts...)
		if l.isVoidType(stmt.Expr.Type) || isVoidExpr(expr.expr) {
			out = append(out, &ast.ExprStmt{X: expr.expr})
		} else {
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr.expr}})
		}
		return out, nil
	case air.StmtWhile:
		if stmt.Condition == nil {
			return nil, fmt.Errorf("while statement missing condition")
		}
		condition, err := l.lowerExpr(fn, *stmt.Condition)
		if err != nil {
			return nil, err
		}
		if len(condition.stmts) != 0 {
			return nil, fmt.Errorf("while conditions with setup statements are not supported yet")
		}
		body, err := l.lowerBlock(fn, stmt.Body, air.NoType)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.ForStmt{Cond: condition.expr, Body: body}}, nil
	case air.StmtBreak:
		return []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}, nil
	default:
		return nil, fmt.Errorf("unsupported statement kind %d", stmt.Kind)
	}
}

func (l *lowerer) lowerExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	switch expr.Kind {
	case air.ExprConstVoid:
		return loweredExpr{expr: ast.NewIdent("nil")}, nil
	case air.ExprConstInt:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", expr.Int)}}, nil
	case air.ExprConstFloat:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.FLOAT, Value: fmt.Sprintf("%v", expr.Float)}}, nil
	case air.ExprConstBool:
		if expr.Bool {
			return loweredExpr{expr: ast.NewIdent("true")}, nil
		}
		return loweredExpr{expr: ast.NewIdent("false")}, nil
	case air.ExprConstStr:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", expr.Str)}}, nil
	case air.ExprLoadLocal:
		return loweredExpr{expr: ast.NewIdent(localName(fn, expr.Local))}, nil
	case air.ExprTraitUpcast:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("trait upcast missing target")
		}
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprCallTrait:
		return l.lowerTraitCall(fn, expr)
	case air.ExprToStr:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to_str missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprCallExtern:
		return l.lowerExternCall(fn, expr)
	case air.ExprCopy:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("copy missing target")
		}
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprMakeList:
		return l.lowerMakeList(fn, expr)
	case air.ExprListSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("list size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprListAt:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("list at missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("list at expects one arg")
		}
		index, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, index.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: target.expr, Index: index.expr}}, nil
	case air.ExprListPush:
		return l.lowerListPush(fn, expr)
	case air.ExprEnumVariant:
		if !validTypeID(l.program, expr.Type) {
			return loweredExpr{}, fmt.Errorf("invalid enum type id %d", expr.Type)
		}
		typ := l.program.Types[expr.Type-1]
		if typ.Kind != air.TypeEnum || expr.Variant < 0 || expr.Variant >= len(typ.Variants) {
			return loweredExpr{}, fmt.Errorf("invalid enum variant %d for type %s", expr.Variant, typ.Name)
		}
		return loweredExpr{expr: ast.NewIdent(enumVariantName(l.program, typ, typ.Variants[expr.Variant]))}, nil
	case air.ExprMakeStruct:
		if !validTypeID(l.program, expr.Type) {
			return loweredExpr{}, fmt.Errorf("invalid struct type id %d", expr.Type)
		}
		typ := l.program.Types[expr.Type-1]
		if typ.Kind != air.TypeStruct {
			return loweredExpr{}, fmt.Errorf("make struct with non-struct type %s", typ.Name)
		}
		stmts := []ast.Stmt{}
		elts := make([]ast.Expr, 0, len(expr.Fields))
		for _, field := range expr.Fields {
			value, err := l.lowerExpr(fn, field.Value)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, value.stmts...)
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(field.Name), Value: value.expr})
		}
		return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, typ)), Elts: elts}}, nil
	case air.ExprGetField:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("get field missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if !validTypeID(l.program, expr.Target.Type) {
			return loweredExpr{}, fmt.Errorf("invalid target type id %d", expr.Target.Type)
		}
		targetType := l.program.Types[expr.Target.Type-1]
		if targetType.Kind != air.TypeStruct || expr.Field < 0 || expr.Field >= len(targetType.Fields) {
			return loweredExpr{}, fmt.Errorf("invalid field index %d", expr.Field)
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(targetType.Fields[expr.Field].Name)}}, nil
	case air.ExprBlock:
		return l.lowerBlockExpr(fn, expr)
	case air.ExprIf:
		return l.lowerIfExpr(fn, expr)
	case air.ExprCall:
		args := make([]ast.Expr, 0, len(expr.Args))
		stmts := []ast.Stmt{}
		for _, arg := range expr.Args {
			loweredArg, err := l.lowerExpr(fn, arg)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, loweredArg.stmts...)
			args = append(args, loweredArg.expr)
		}
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid function id %d", expr.Function)
		}
		target := l.program.Functions[expr.Function]
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent(functionName(l.program, target)), Args: args}}, nil
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprEq, air.ExprNotEq, air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte,
		air.ExprAnd, air.ExprOr, air.ExprStrConcat:
		left, err := l.lowerExpr(fn, *expr.Left)
		if err != nil {
			return loweredExpr{}, err
		}
		right, err := l.lowerExpr(fn, *expr.Right)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{
			stmts: append(left.stmts, right.stmts...),
			expr:  &ast.BinaryExpr{X: left.expr, Op: l.binaryToken(expr.Kind), Y: right.expr},
		}, nil
	case air.ExprNot:
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.NOT, X: target.expr}}, nil
	case air.ExprNeg:
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.SUB, X: target.expr}}, nil
	default:
		return loweredExpr{}, fmt.Errorf("unsupported expression kind %d", expr.Kind)
	}
}

func (l *lowerer) lowerBlockExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if l.isVoidType(expr.Type) {
		body, err := l.lowerValueBlock(fn, expr.Body, expr.Type, nil)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: body, expr: ast.NewIdent("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	body, err := l.lowerValueBlock(fn, expr.Body, expr.Type, ast.NewIdent(temp))
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: append(decls, body...), expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerIfExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Condition == nil {
		return loweredExpr{}, fmt.Errorf("if expression missing condition")
	}
	condition, err := l.lowerExpr(fn, *expr.Condition)
	if err != nil {
		return loweredExpr{}, err
	}
	if len(condition.stmts) != 0 {
		return loweredExpr{}, fmt.Errorf("if conditions with setup statements are not supported yet")
	}
	resultExpr := ast.NewIdent("nil")
	stmts := []ast.Stmt{}
	var target ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		target = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	thenBody, err := l.lowerValueBlock(fn, expr.Then, expr.Type, target)
	if err != nil {
		return loweredExpr{}, err
	}
	elseBody, err := l.lowerValueBlock(fn, expr.Else, expr.Type, target)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: condition.expr,
		Body: &ast.BlockStmt{List: thenBody},
		Else: &ast.BlockStmt{List: elseBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerValueBlock(fn air.Function, block air.Block, resultType air.TypeID, target ast.Expr) ([]ast.Stmt, error) {
	stmts := []ast.Stmt{}
	for _, stmt := range block.Stmts {
		lowered, err := l.lowerStmt(fn, stmt)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, lowered...)
	}
	if block.Result != nil {
		result, err := l.lowerExpr(fn, *block.Result)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, result.stmts...)
		if l.isVoidType(resultType) {
			if !isVoidExpr(result.expr) {
				stmts = append(stmts, &ast.ExprStmt{X: result.expr})
			}
		} else {
			if target == nil {
				return nil, fmt.Errorf("non-void block result missing target")
			}
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
		}
	}
	return stmts, nil
}

func (l *lowerer) declareTemp(typeID air.TypeID, name string) ([]ast.Stmt, error) {
	if l.isVoidType(typeID) {
		return nil, nil
	}
	typ, err := l.goType(typeID)
	if err != nil {
		return nil, err
	}
	return []ast.Stmt{&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(name)}, Type: typ}}}}}, nil
}

func (l *lowerer) nextTemp() string {
	name := fmt.Sprintf("_tmp_%d", l.tempCounter)
	l.tempCounter++
	return name
}

func (l *lowerer) binaryToken(kind air.ExprKind) token.Token {
	switch kind {
	case air.ExprIntAdd, air.ExprFloatAdd, air.ExprStrConcat:
		return token.ADD
	case air.ExprIntSub, air.ExprFloatSub:
		return token.SUB
	case air.ExprIntMul, air.ExprFloatMul:
		return token.MUL
	case air.ExprIntDiv, air.ExprFloatDiv:
		return token.QUO
	case air.ExprIntMod:
		return token.REM
	case air.ExprEq:
		return token.EQL
	case air.ExprNotEq:
		return token.NEQ
	case air.ExprLt:
		return token.LSS
	case air.ExprLte:
		return token.LEQ
	case air.ExprGt:
		return token.GTR
	case air.ExprGte:
		return token.GEQ
	case air.ExprAnd:
		return token.LAND
	case air.ExprOr:
		return token.LOR
	default:
		return token.ILLEGAL
	}
}

func (l *lowerer) goType(typeID air.TypeID) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeVoid:
		return nil, nil
	case air.TypeInt:
		return ast.NewIdent("int"), nil
	case air.TypeFloat:
		return ast.NewIdent("float64"), nil
	case air.TypeBool:
		return ast.NewIdent("bool"), nil
	case air.TypeStr:
		return ast.NewIdent("string"), nil
	case air.TypeList:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Elt: elem}, nil
	case air.TypeMap:
		key, err := l.goType(info.Key)
		if err != nil {
			return nil, err
		}
		value, err := l.goType(info.Value)
		if err != nil {
			return nil, err
		}
		return &ast.MapType{Key: key, Value: value}, nil
	case air.TypeStruct, air.TypeEnum:
		return ast.NewIdent(typeName(l.program, info)), nil
	case air.TypeTraitObject:
		return ast.NewIdent("any"), nil
	default:
		return nil, fmt.Errorf("unsupported Go type kind %d", info.Kind)
	}
}

func (l *lowerer) isVoidType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeVoid
}

func (l *lowerer) qualified(alias string, importPath string, name string) ast.Expr {
	l.currentImports[alias] = importPath
	return &ast.SelectorExpr{X: ast.NewIdent(alias), Sel: ast.NewIdent(name)}
}

func (l *lowerer) lowerMakeList(fn air.Function, expr air.Expr) (loweredExpr, error) {
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	elts := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	for _, arg := range expr.Args {
		loweredArg, err := l.lowerExpr(fn, arg)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		elts = append(elts, loweredArg.expr)
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: typ, Elts: elts}}, nil
}

func (l *lowerer) lowerListPush(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("list push missing target")
	}
	if expr.Target.Kind != air.ExprLoadLocal {
		return loweredExpr{}, fmt.Errorf("list push currently requires local target")
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list push expects one arg")
	}
	value, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	name := localName(fn, expr.Target.Local)
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{ast.NewIdent(name), value.expr}}},
	}
	stmts := append([]ast.Stmt{}, value.stmts...)
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{ast.NewIdent(name)}}}, nil
}

func (l *lowerer) lowerTraitCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("trait call missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if expr.Trait < 0 || int(expr.Trait) >= len(l.program.Traits) {
		return loweredExpr{}, fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := l.program.Traits[expr.Trait]
	if expr.Method < 0 || expr.Method >= len(trait.Methods) {
		return loweredExpr{}, fmt.Errorf("invalid trait method %d for %s", expr.Method, trait.Name)
	}
	method := trait.Methods[expr.Method]
	switch {
	case trait.Name == "ToString" && method.Name == "to_str":
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{target.expr}}}, nil
	default:
		return loweredExpr{}, fmt.Errorf("unsupported trait call %s.%s", trait.Name, method.Name)
	}
}

func (l *lowerer) lowerExternCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Extern < 0 || int(expr.Extern) >= len(l.program.Externs) {
		return loweredExpr{}, fmt.Errorf("invalid extern id %d", expr.Extern)
	}
	ext := l.program.Externs[expr.Extern]
	args := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	for _, arg := range expr.Args {
		loweredArg, err := l.lowerExpr(fn, arg)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		args = append(args, loweredArg.expr)
	}
	binding := ext.Name
	if goBinding, ok := ext.Bindings["go"]; ok && goBinding != "" {
		binding = goBinding
	}
	switch binding {
	case "Print":
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Println"), Args: args}}, nil
	default:
		return loweredExpr{}, fmt.Errorf("unsupported go extern binding %q", binding)
	}
}

func isVoidExpr(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func validFunctionID(program *air.Program, id air.FunctionID) bool {
	return id >= 0 && int(id) < len(program.Functions)
}

func validTypeID(program *air.Program, id air.TypeID) bool {
	return id > 0 && int(id) <= len(program.Types)
}
