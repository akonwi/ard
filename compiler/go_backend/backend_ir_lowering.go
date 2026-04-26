package go_backend

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
	backendir "github.com/akonwi/ard/go_backend/ir"
)

func lowerModuleToBackendIR(module checker.Module, packageName string, entrypoint bool) (*backendir.Module, error) {
	if module == nil || module.Program() == nil {
		return nil, fmt.Errorf("module has no program")
	}

	out := &backendir.Module{
		Path:        module.Path(),
		PackageName: packageName,
		Decls:       make([]backendir.Decl, 0, len(module.Program().Statements)),
	}
	seenDecls := make(map[string]struct{})

	for _, stmt := range module.Program().Statements {
		if stmt.Stmt != nil {
			switch def := stmt.Stmt.(type) {
			case *checker.StructDef:
				key := "struct:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerStructDeclToBackendIR(def))
			case checker.StructDef:
				key := "struct:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				defCopy := def
				out.Decls = append(out.Decls, lowerStructDeclToBackendIR(&defCopy))
			case *checker.Enum:
				key := "enum:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerEnumDeclToBackendIR(def))
			case checker.Enum:
				key := "enum:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				defCopy := def
				out.Decls = append(out.Decls, lowerEnumDeclToBackendIR(&defCopy))
			case *checker.Union:
				key := "union:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(def))
			case checker.Union:
				key := "union:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				defCopy := def
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(&defCopy))
			case *checker.ExternType:
				key := "extern_type:" + def.Name_
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerExternTypeDeclToBackendIR(def))
			case *checker.VariableDef:
				if entrypoint {
					break
				}
				key := "var:" + def.Name
				if _, exists := seenDecls[key]; exists {
					break
				}
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerVariableDeclToBackendIR(def))
			}
		}

		if stmt.Expr != nil {
			switch def := stmt.Expr.(type) {
			case *checker.FunctionDef:
				if def.Receiver != "" || def.IsTest {
					continue
				}
				out.Decls = append(out.Decls, lowerFunctionDeclToBackendIR(def))
			case *checker.ExternalFunctionDef:
				out.Decls = append(out.Decls, lowerExternFunctionDeclToBackendIR(def))
			}
		}
	}

	// The checker keeps certain type-declared identifiers in scope
	// without emitting a Statement entry for them. Two cases matter:
	//
	//   1. Enum declarations without an associated `impl` block.
	//   2. Union/type-alias declarations (`type X = A | B`), which the
	//      checker registers in scope but never returns as a Statement.
	//
	// To support native backend IR emission of signatures that reference
	// these types, we still need to surface them as IR declarations so
	// the Go emitter generates the corresponding type definitions
	// (`type X struct { Tag int }` for enums, `type X interface {}` for
	// unions). Walk type references in the program and synthesize the
	// missing declarations for any orphan enum or union we have not yet
	// seen.
	collectOrphanTypeDecls(module.Program(), out, seenDecls)

	out.Entrypoint = lowerEntrypointStatementsToBackendIRBlock(topLevelExecutableStatements(module.Program().Statements))

	if err := backendir.ValidateModule(out); err != nil {
		return nil, err
	}
	return out, nil
}

func collectOrphanTypeDecls(program *checker.Program, out *backendir.Module, seenDecls map[string]struct{}) {
	if program == nil {
		return
	}
	visited := make(map[checker.Type]struct{})
	collect := func(t checker.Type) {
		visitOrphanTypeDeclsInType(t, out, seenDecls, visited)
	}
	for _, stmt := range program.Statements {
		if stmt.Expr != nil {
			collect(stmt.Expr.Type())
			switch def := stmt.Expr.(type) {
			case *checker.FunctionDef:
				for _, param := range def.Parameters {
					collect(param.Type)
				}
				collect(effectiveFunctionReturnType(def))
			case *checker.ExternalFunctionDef:
				for _, param := range def.Parameters {
					collect(param.Type)
				}
				collect(def.ReturnType)
			}
		}
		if stmt.Stmt != nil {
			switch def := stmt.Stmt.(type) {
			case *checker.VariableDef:
				if def != nil {
					collect(def.Type())
				}
			case *checker.StructDef:
				if def != nil {
					for _, fieldType := range def.Fields {
						collect(fieldType)
					}
					for _, method := range def.Methods {
						for _, param := range method.Parameters {
							collect(param.Type)
						}
						collect(effectiveFunctionReturnType(method))
					}
				}
			case checker.StructDef:
				for _, fieldType := range def.Fields {
					collect(fieldType)
				}
				for _, method := range def.Methods {
					for _, param := range method.Parameters {
						collect(param.Type)
					}
					collect(effectiveFunctionReturnType(method))
				}
			}
		}
	}
}

func visitOrphanTypeDeclsInType(t checker.Type, out *backendir.Module, seenDecls map[string]struct{}, visited map[checker.Type]struct{}) {
	if t == nil {
		return
	}
	if _, seen := visited[t]; seen {
		return
	}
	visited[t] = struct{}{}
	switch typed := t.(type) {
	case *checker.Enum:
		if typed == nil {
			return
		}
		key := "enum:" + typed.Name
		if _, exists := seenDecls[key]; exists {
			return
		}
		seenDecls[key] = struct{}{}
		out.Decls = append(out.Decls, lowerEnumDeclToBackendIR(typed))
	case *checker.Union:
		if typed == nil {
			return
		}
		name := strings.TrimSpace(typed.Name)
		if name != "" {
			key := "union:" + name
			if _, exists := seenDecls[key]; !exists {
				seenDecls[key] = struct{}{}
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(typed))
			}
		}
		for _, memberType := range typed.Types {
			visitOrphanTypeDeclsInType(memberType, out, seenDecls, visited)
		}
	case checker.Union:
		name := strings.TrimSpace(typed.Name)
		if name != "" {
			key := "union:" + name
			if _, exists := seenDecls[key]; !exists {
				seenDecls[key] = struct{}{}
				defCopy := typed
				out.Decls = append(out.Decls, lowerUnionDeclToBackendIR(&defCopy))
			}
		}
		for _, memberType := range typed.Types {
			visitOrphanTypeDeclsInType(memberType, out, seenDecls, visited)
		}
	case *checker.TypeVar:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Actual(), out, seenDecls, visited)
		}
	case *checker.List:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Of(), out, seenDecls, visited)
		}
	case *checker.Map:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Key(), out, seenDecls, visited)
			visitOrphanTypeDeclsInType(typed.Value(), out, seenDecls, visited)
		}
	case *checker.Maybe:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Of(), out, seenDecls, visited)
		}
	case *checker.Result:
		if typed != nil {
			visitOrphanTypeDeclsInType(typed.Val(), out, seenDecls, visited)
			visitOrphanTypeDeclsInType(typed.Err(), out, seenDecls, visited)
		}
	case *checker.FunctionDef:
		if typed != nil {
			for _, param := range typed.Parameters {
				visitOrphanTypeDeclsInType(param.Type, out, seenDecls, visited)
			}
			visitOrphanTypeDeclsInType(effectiveFunctionReturnType(typed), out, seenDecls, visited)
		}
	case *checker.ExternalFunctionDef:
		if typed != nil {
			for _, param := range typed.Parameters {
				visitOrphanTypeDeclsInType(param.Type, out, seenDecls, visited)
			}
			visitOrphanTypeDeclsInType(typed.ReturnType, out, seenDecls, visited)
		}
	}
}

func lowerStatementsToBackendIRBlock(stmts []checker.Statement) *backendir.Block {
	block := &backendir.Block{Stmts: []backendir.Stmt{}}
	for _, stmt := range stmts {
		block.Stmts = append(block.Stmts, lowerStatementToBackendIR(stmt)...)
	}
	return block
}

func lowerEntrypointStatementsToBackendIRBlock(stmts []checker.Statement) *backendir.Block {
	block := &backendir.Block{Stmts: []backendir.Stmt{}}
	for i, stmt := range stmts {
		block.Stmts = append(block.Stmts, lowerStatementToBackendIR(stmt)...)
		variableDef, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok || variableDef == nil {
			continue
		}
		if variableDef.Name == "_" || strings.TrimSpace(variableDef.Name) == "" {
			continue
		}
		if usesNameInStatements(stmts[i+1:], variableDef.Name) {
			continue
		}
		block.Stmts = append(block.Stmts, &backendir.AssignStmt{
			Target: "_",
			Value:  &backendir.IdentExpr{Name: variableDef.Name},
		})
	}
	return block
}

func lowerStructDeclToBackendIR(def *checker.StructDef) backendir.Decl {
	fields := make([]backendir.Field, 0, len(def.Fields))
	for _, fieldName := range sortedStringKeys(def.Fields) {
		fields = append(fields, backendir.Field{
			Name: fieldName,
			Type: lowerCheckerTypeToBackendIR(def.Fields[fieldName]),
		})
	}
	return &backendir.StructDecl{
		Name:   def.Name,
		Fields: fields,
	}
}

func lowerEnumDeclToBackendIR(def *checker.Enum) backendir.Decl {
	values := make([]backendir.EnumValue, 0, len(def.Values))
	for _, value := range def.Values {
		values = append(values, backendir.EnumValue{
			Name:  value.Name,
			Value: value.Value,
		})
	}
	return &backendir.EnumDecl{
		Name:   def.Name,
		Values: values,
	}
}

func lowerUnionDeclToBackendIR(def *checker.Union) backendir.Decl {
	types := make([]backendir.Type, 0, len(def.Types))
	for _, typ := range def.Types {
		types = append(types, lowerCheckerTypeToBackendIR(typ))
	}
	return &backendir.UnionDecl{
		Name:  def.Name,
		Types: types,
	}
}

func lowerExternTypeDeclToBackendIR(def *checker.ExternType) backendir.Decl {
	args := make([]backendir.Type, 0, len(def.TypeArgs))
	for _, arg := range def.TypeArgs {
		args = append(args, lowerCheckerTypeToBackendIR(arg))
	}
	return &backendir.ExternTypeDecl{
		Name: strings.TrimSpace(def.Name_),
		Args: args,
	}
}

func lowerVariableDeclToBackendIR(def *checker.VariableDef) backendir.Decl {
	return &backendir.VarDecl{
		Name:    def.Name,
		Type:    lowerCheckerTypeToBackendIR(def.Type()),
		Value:   lowerExpressionOrOpaque(def.Value),
		Mutable: def.Mutable,
	}
}

func lowerFunctionDeclToBackendIR(def *checker.FunctionDef) backendir.Decl {
	params := make([]backendir.Param, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		params = append(params, backendir.Param{
			Name:    param.Name,
			Type:    lowerCheckerTypeToBackendIR(param.Type),
			Mutable: param.Mutable,
		})
	}

	returnType := lowerCheckerTypeToBackendIR(effectiveFunctionReturnType(def))
	body := lowerBlockToBackendIR(def.Body)
	finalizeFunctionBodyForReturn(body, returnType)

	return &backendir.FuncDecl{
		Name:      def.Name,
		Params:    params,
		Return:    returnType,
		Body:      body,
		IsExtern:  false,
		IsPrivate: def.Private,
		IsTest:    def.IsTest,
	}
}

func finalizeFunctionBodyForReturn(body *backendir.Block, returnType backendir.Type) {
	if body == nil || isVoidIRType(returnType) || len(body.Stmts) == 0 {
		return
	}

	lastIndex := len(body.Stmts) - 1
	switch last := body.Stmts[lastIndex].(type) {
	case *backendir.ReturnStmt:
		return
	case *backendir.ExprStmt:
		body.Stmts[lastIndex] = &backendir.ReturnStmt{Value: last.Value}
	}
}

func isVoidIRType(t backendir.Type) bool {
	_, ok := t.(*backendir.VoidType)
	return ok
}

func lowerExternFunctionDeclToBackendIR(def *checker.ExternalFunctionDef) backendir.Decl {
	params := make([]backendir.Param, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		params = append(params, backendir.Param{
			Name:    param.Name,
			Type:    lowerCheckerTypeToBackendIR(param.Type),
			Mutable: param.Mutable,
		})
	}

	binding := strings.TrimSpace(def.ExternalBinding)
	if binding == "" {
		binding = "<unresolved>"
	}

	return &backendir.FuncDecl{
		Name:          def.Name,
		Params:        params,
		Return:        lowerCheckerTypeToBackendIR(def.ReturnType),
		Body:          nil,
		ExternBinding: binding,
		IsExtern:      true,
		IsPrivate:     def.Private,
	}
}

func lowerBlockToBackendIR(block *checker.Block) *backendir.Block {
	out := &backendir.Block{Stmts: []backendir.Stmt{}}
	if block == nil {
		return out
	}
	for i, stmt := range block.Stmts {
		out.Stmts = append(out.Stmts, lowerStatementToBackendIR(stmt)...)
		variableDef, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok || variableDef == nil {
			continue
		}
		if variableDef.Name == "_" || strings.TrimSpace(variableDef.Name) == "" {
			continue
		}
		if usesNameInStatements(block.Stmts[i+1:], variableDef.Name) {
			continue
		}
		out.Stmts = append(out.Stmts, &backendir.AssignStmt{
			Target: "_",
			Value:  &backendir.IdentExpr{Name: variableDef.Name},
		})
	}
	return out
}

func lowerStatementToBackendIR(stmt checker.Statement) []backendir.Stmt {
	out := make([]backendir.Stmt, 0, 2)

	if stmt.Break {
		out = append(out, &backendir.BreakStmt{})
	}

	if stmt.Stmt != nil {
		out = append(out, lowerNonProducingToBackendIR(stmt.Stmt)...)
	}

	if stmt.Expr != nil {
		if ifExpr, ok := stmt.Expr.(*checker.If); ok {
			out = append(out, lowerIfChainToBackendIR(ifExpr))
		} else {
			out = append(out, &backendir.ExprStmt{
				Value: lowerExpressionToBackendIR(stmt.Expr),
			})
		}
	}

	return out
}

func lowerNonProducingToBackendIR(node checker.NonProducing) []backendir.Stmt {
	switch n := node.(type) {
	case *checker.VariableDef:
		return []backendir.Stmt{
			&backendir.AssignStmt{
				Target: n.Name,
				Value:  lowerExpressionOrOpaque(n.Value),
			},
		}
	case *checker.Reassignment:
		return []backendir.Stmt{lowerReassignmentToBackendIRStmt(n)}
	case checker.ForIntRange:
		loop := n
		return lowerNonProducingToBackendIR(&loop)
	case *checker.ForIntRange:
		return []backendir.Stmt{
			&backendir.ForIntRangeStmt{
				Cursor: n.Cursor,
				Index:  n.Index,
				Start:  lowerExpressionOrOpaque(n.Start),
				End:    lowerExpressionOrOpaque(n.End),
				Body:   lowerBlockToBackendIR(n.Body),
			},
		}
	case checker.ForInStr:
		loop := n
		return lowerNonProducingToBackendIR(&loop)
	case *checker.ForInStr:
		return []backendir.Stmt{
			&backendir.ForInStrStmt{
				Cursor: n.Cursor,
				Index:  n.Index,
				Value:  lowerExpressionOrOpaque(n.Value),
				Body:   lowerBlockToBackendIR(n.Body),
			},
		}
	case checker.ForInList:
		loop := n
		return lowerNonProducingToBackendIR(&loop)
	case *checker.ForInList:
		return []backendir.Stmt{
			&backendir.ForInListStmt{
				Cursor: n.Cursor,
				Index:  n.Index,
				List:   lowerExpressionOrOpaque(n.List),
				Body:   lowerBlockToBackendIR(n.Body),
			},
		}
	case checker.ForInMap:
		loop := n
		return lowerNonProducingToBackendIR(&loop)
	case *checker.ForInMap:
		return []backendir.Stmt{
			&backendir.ForInMapStmt{
				Key:   n.Key,
				Value: n.Val,
				Map:   lowerExpressionOrOpaque(n.Map),
				Body:  lowerBlockToBackendIR(n.Body),
			},
		}
	case checker.ForLoop:
		loop := n
		return lowerNonProducingToBackendIR(&loop)
	case *checker.ForLoop:
		update := lowerReassignmentToBackendIRStmt(n.Update)
		cond := lowerExpressionOrOpaque(n.Condition)
		if cond == nil {
			cond = literalExpr("bool", "true")
		}
		initName := "i"
		initValue := literalExpr("int", "0")
		if n.Init != nil {
			if strings.TrimSpace(n.Init.Name) != "" {
				initName = n.Init.Name
			}
			initValue = lowerExpressionOrOpaque(n.Init.Value)
		}
		return []backendir.Stmt{
			&backendir.ForLoopStmt{
				InitName:  initName,
				InitValue: initValue,
				Cond:      cond,
				Update:    update,
				Body:      lowerBlockToBackendIR(n.Body),
			},
		}
	case checker.WhileLoop:
		loop := n
		return lowerNonProducingToBackendIR(&loop)
	case *checker.WhileLoop:
		return []backendir.Stmt{
			&backendir.WhileStmt{
				Cond: lowerExpressionOrOpaque(n.Condition),
				Body: lowerBlockToBackendIR(n.Body),
			},
		}
	case checker.StructDef:
		def := n
		return lowerNonProducingToBackendIR(&def)
	case *checker.StructDef:
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("struct_decl_stmt", literalExpr("ident", n.Name)),
			},
		}
	case checker.Enum:
		def := n
		return lowerNonProducingToBackendIR(&def)
	case *checker.Enum:
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("enum_decl_stmt", literalExpr("ident", n.Name)),
			},
		}
	case checker.Union:
		def := n
		return lowerNonProducingToBackendIR(&def)
	case *checker.Union:
		args := make([]backendir.Expr, 0, len(n.Types)+1)
		args = append(args, literalExpr("ident", n.Name))
		for _, typ := range n.Types {
			args = append(args, typeExpr(lowerCheckerTypeToBackendIR(typ)))
		}
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("union_decl_stmt", args...),
			},
		}
	case *checker.ExternType:
		args := make([]backendir.Expr, 0, len(n.TypeArgs)+1)
		args = append(args, literalExpr("ident", n.Name_))
		for _, typeArg := range n.TypeArgs {
			args = append(args, typeExpr(lowerCheckerTypeToBackendIR(typeArg)))
		}
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr("extern_type_decl_stmt", args...),
			},
		}
	default:
		return []backendir.Stmt{
			&backendir.ExprStmt{
				Value: callExpr(
					"nonproducing_stmt",
					literalExpr("type", fmt.Sprintf("%T", node)),
				),
			},
		}
	}
}

func lowerIfChainToBackendIR(node *checker.If) backendir.Stmt {
	if node == nil {
		return &backendir.ExprStmt{Value: literalExpr("if_stmt", "nil")}
	}

	out := &backendir.IfStmt{
		Cond: lowerExpressionOrOpaque(node.Condition),
		Then: lowerBlockToBackendIR(node.Body),
	}

	if node.ElseIf != nil {
		out.Else = &backendir.Block{
			Stmts: []backendir.Stmt{
				lowerIfChainToBackendIR(node.ElseIf),
			},
		}
	} else if node.Else != nil {
		out.Else = lowerBlockToBackendIR(node.Else)
	}

	return out
}

func lowerBlockAsExpr(block *checker.Block) backendir.Expr {
	if block == nil {
		return literalExpr("block", "nil")
	}
	args := make([]backendir.Expr, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		args = append(args, lowerStatementAsExpr(stmt))
	}
	return callExpr("block", args...)
}

func lowerStatementAsExpr(stmt checker.Statement) backendir.Expr {
	lowered := lowerStatementToBackendIR(stmt)
	if len(lowered) == 1 {
		return lowerIRStmtAsExpr(lowered[0])
	}
	args := make([]backendir.Expr, 0, len(lowered))
	for _, item := range lowered {
		args = append(args, lowerIRStmtAsExpr(item))
	}
	return callExpr("stmt_group", args...)
}

func lowerIRStmtAsExpr(stmt backendir.Stmt) backendir.Expr {
	switch s := stmt.(type) {
	case *backendir.ReturnStmt:
		if s.Value == nil {
			return callExpr("return_stmt")
		}
		return callExpr("return_stmt", s.Value)
	case *backendir.ExprStmt:
		return callExpr("expr_stmt", s.Value)
	case *backendir.BreakStmt:
		return callExpr("break_stmt")
	case *backendir.AssignStmt:
		return callExpr("assign_stmt", literalExpr("ident", s.Target), s.Value)
	case *backendir.MemberAssignStmt:
		return callExpr(
			"assign_member_stmt",
			s.Subject,
			literalExpr("ident", s.Field),
			s.Value,
		)
	case *backendir.ForIntRangeStmt:
		return callExpr(
			"for_int_range",
			literalExpr("ident", s.Cursor),
			literalExpr("ident", s.Index),
			s.Start,
			s.End,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForLoopStmt:
		return callExpr(
			"for_loop",
			literalExpr("ident", s.InitName),
			s.InitValue,
			s.Cond,
			lowerIRStmtAsExpr(s.Update),
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForInStrStmt:
		return callExpr(
			"for_in_str",
			literalExpr("ident", s.Cursor),
			literalExpr("ident", s.Index),
			s.Value,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForInListStmt:
		return callExpr(
			"for_in_list",
			literalExpr("ident", s.Cursor),
			literalExpr("ident", s.Index),
			s.List,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.ForInMapStmt:
		return callExpr(
			"for_in_map",
			literalExpr("ident", s.Key),
			literalExpr("ident", s.Value),
			s.Map,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.WhileStmt:
		return callExpr(
			"while_loop",
			s.Cond,
			lowerIRBlockAsExpr(s.Body),
		)
	case *backendir.IfStmt:
		args := []backendir.Expr{
			s.Cond,
			lowerIRBlockAsExpr(s.Then),
		}
		if s.Else != nil {
			args = append(args, lowerIRBlockAsExpr(s.Else))
		}
		return callExpr("if_stmt", args...)
	default:
		return callExpr("stmt_unknown", literalExpr("type", fmt.Sprintf("%T", stmt)))
	}
}

func lowerIRBlockAsExpr(block *backendir.Block) backendir.Expr {
	if block == nil {
		return literalExpr("block", "nil")
	}
	args := make([]backendir.Expr, 0, len(block.Stmts))
	for _, stmt := range block.Stmts {
		args = append(args, lowerIRStmtAsExpr(stmt))
	}
	return callExpr("block_ir", args...)
}

func lowerVariableDefAsExpr(def *checker.VariableDef) backendir.Expr {
	if def == nil {
		return literalExpr("var_def", "nil")
	}
	return callExpr(
		"var_def",
		literalExpr("ident", def.Name),
		typeExpr(lowerCheckerTypeToBackendIR(def.Type())),
		lowerExpressionOrOpaque(def.Value),
	)
}

func lowerReassignmentAsExpr(stmt *checker.Reassignment) backendir.Expr {
	if stmt == nil {
		return literalExpr("reassign", "nil")
	}
	return lowerIRStmtAsExpr(lowerReassignmentToBackendIRStmt(stmt))
}

func lowerReassignmentToBackendIRStmt(stmt *checker.Reassignment) backendir.Stmt {
	if stmt == nil {
		return &backendir.AssignStmt{
			Target: "<target:nil>",
			Value:  literalExpr("reassign", "nil"),
		}
	}

	switch target := stmt.Target.(type) {
	case *checker.InstanceProperty:
		return &backendir.MemberAssignStmt{
			Subject: lowerExpressionOrOpaque(target.Subject),
			Field:   target.Property,
			Value:   lowerExpressionOrOpaque(stmt.Value),
		}
	default:
		return &backendir.AssignStmt{
			Target: lowerAssignmentTargetName(stmt.Target),
			Value:  lowerExpressionOrOpaque(stmt.Value),
		}
	}
}

func lowerExpressionOrOpaque(expr checker.Expression) backendir.Expr {
	if expr == nil {
		return literalExpr("nil_expr", "")
	}
	return lowerExpressionToBackendIR(expr)
}

func lowerExpressionToBackendIR(expr checker.Expression) backendir.Expr {
	switch v := expr.(type) {
	case *checker.Identifier:
		return &backendir.IdentExpr{Name: v.Name}
	case checker.Variable:
		return &backendir.IdentExpr{Name: v.Name()}
	case *checker.Variable:
		return &backendir.IdentExpr{Name: v.Name()}
	case *checker.StrLiteral:
		return &backendir.LiteralExpr{Kind: "str", Value: v.Value}
	case *checker.TemplateStr:
		args := make([]backendir.Expr, 0, len(v.Chunks))
		for _, chunk := range v.Chunks {
			args = append(args, lowerExpressionOrOpaque(chunk))
		}
		return &backendir.CallExpr{
			Callee: &backendir.IdentExpr{Name: "template"},
			Args:   args,
		}
	case *checker.BoolLiteral:
		return &backendir.LiteralExpr{Kind: "bool", Value: strconv.FormatBool(v.Value)}
	case *checker.VoidLiteral:
		return &backendir.LiteralExpr{Kind: "void", Value: "()"}
	case *checker.IntLiteral:
		return &backendir.LiteralExpr{Kind: "int", Value: strconv.Itoa(v.Value)}
	case *checker.FloatLiteral:
		return &backendir.LiteralExpr{Kind: "float", Value: strconv.FormatFloat(v.Value, 'g', 10, 64)}
	case *checker.FunctionCall:
		return lowerFunctionCallToBackendIR(v)
	case *checker.ModuleFunctionCall:
		return lowerModuleFunctionCallToBackendIR(v)
	case *checker.InstanceProperty:
		return &backendir.SelectorExpr{
			Subject: lowerExpressionOrOpaque(v.Subject),
			Name:    v.Property,
		}
	case *checker.InstanceMethod:
		if v.Method == nil {
			return callExpr("instance_method", literalExpr("nil", "method"))
		}
		args := make([]backendir.Expr, 0, len(v.Method.Args))
		for _, arg := range v.Method.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		return &backendir.CallExpr{
			Callee: &backendir.SelectorExpr{
				Subject: lowerExpressionOrOpaque(v.Subject),
				Name:    v.Method.Name,
			},
			Args: args,
		}
	case *checker.ModuleSymbol:
		return &backendir.SelectorExpr{
			Subject: &backendir.IdentExpr{Name: v.Module},
			Name:    v.Symbol.Name,
		}
	case *checker.Block:
		return callExpr("block_expr", lowerBlockAsExpr(v))
	case checker.Enum:
		enum := v
		return lowerExpressionToBackendIR(&enum)
	case *checker.Enum:
		return callExpr("enum_type", literalExpr("ident", v.Name))
	case checker.Union:
		union := v
		return lowerExpressionToBackendIR(&union)
	case *checker.Union:
		args := make([]backendir.Expr, 0, len(v.Types)+1)
		args = append(args, literalExpr("ident", v.Name))
		for _, typ := range v.Types {
			args = append(args, typeExpr(lowerCheckerTypeToBackendIR(typ)))
		}
		return callExpr("union_type", args...)
	case *checker.StrMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.StrSize:
			return callExpr("str_size", args...)
		case checker.StrIsEmpty:
			return callExpr("str_is_empty", args...)
		case checker.StrContains:
			return callExpr("str_contains", args...)
		case checker.StrReplace:
			return callExpr("str_replace", args...)
		case checker.StrReplaceAll:
			return callExpr("str_replace_all", args...)
		case checker.StrSplit:
			return callExpr("str_split", args...)
		case checker.StrStartsWith:
			return callExpr("str_starts_with", args...)
		case checker.StrToStr:
			return callExpr("str_to_str", args...)
		case checker.StrToDyn:
			return callExpr("str_to_dyn", args...)
		case checker.StrTrim:
			return callExpr("str_trim", args...)
		default:
			return callExpr("str_method:"+strMethodKindName(v.Kind), args...)
		}
	case *checker.IntMethod:
		switch v.Kind {
		case checker.IntToStr:
			return callExpr("int_to_str", lowerExpressionOrOpaque(v.Subject))
		case checker.IntToDyn:
			return callExpr("int_to_dyn", lowerExpressionOrOpaque(v.Subject))
		default:
			return callExpr("int_method:"+intMethodKindName(v.Kind), lowerExpressionOrOpaque(v.Subject))
		}
	case *checker.FloatMethod:
		switch v.Kind {
		case checker.FloatToStr:
			return callExpr("float_to_str", lowerExpressionOrOpaque(v.Subject))
		case checker.FloatToInt:
			return callExpr("float_to_int", lowerExpressionOrOpaque(v.Subject))
		case checker.FloatToDyn:
			return callExpr("float_to_dyn", lowerExpressionOrOpaque(v.Subject))
		default:
			return callExpr("float_method:"+floatMethodKindName(v.Kind), lowerExpressionOrOpaque(v.Subject))
		}
	case *checker.BoolMethod:
		switch v.Kind {
		case checker.BoolToStr:
			return callExpr("bool_to_str", lowerExpressionOrOpaque(v.Subject))
		case checker.BoolToDyn:
			return callExpr("bool_to_dyn", lowerExpressionOrOpaque(v.Subject))
		default:
			return callExpr("bool_method:"+boolMethodKindName(v.Kind), lowerExpressionOrOpaque(v.Subject))
		}
	case *checker.ListMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.ListSize:
			return callExpr("list_size", args...)
		case checker.ListAt:
			return callExpr("list_at", args...)
		case checker.ListPush:
			return callExpr("list_push", args...)
		case checker.ListPrepend:
			return callExpr("list_prepend", args...)
		case checker.ListSet:
			return callExpr("list_set", args...)
		case checker.ListSort:
			return callExpr("list_sort", args...)
		case checker.ListSwap:
			return callExpr("list_swap", args...)
		default:
			return callExpr("list_method:"+listMethodKindName(v.Kind), args...)
		}
	case *checker.MapMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.MapSize:
			return callExpr("map_size", args...)
		case checker.MapKeys:
			return callExpr("map_keys", args...)
		case checker.MapHas:
			return callExpr("map_has", args...)
		case checker.MapGet:
			return callExpr("map_get", args...)
		case checker.MapSet:
			return callExpr("map_set", args...)
		case checker.MapDrop:
			return callExpr("map_drop", args...)
		default:
			return callExpr("map_method:"+mapMethodKindName(v.Kind), args...)
		}
	case *checker.MaybeMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.MaybeExpect:
			return callExpr("maybe_expect", args...)
		case checker.MaybeIsNone:
			return callExpr("maybe_is_none", args...)
		case checker.MaybeIsSome:
			return callExpr("maybe_is_some", args...)
		case checker.MaybeOr:
			return callExpr("maybe_or", args...)
		case checker.MaybeMap:
			return callExpr("maybe_map", args...)
		case checker.MaybeAndThen:
			return callExpr("maybe_and_then", args...)
		default:
			return callExpr("maybe_method:"+maybeMethodKindName(v.Kind), args...)
		}
	case *checker.ResultMethod:
		args := make([]backendir.Expr, 0, len(v.Args)+1)
		args = append(args, lowerExpressionOrOpaque(v.Subject))
		for _, arg := range v.Args {
			args = append(args, lowerExpressionOrOpaque(arg))
		}
		switch v.Kind {
		case checker.ResultExpect:
			return callExpr("result_expect", args...)
		case checker.ResultOr:
			return callExpr("result_or", args...)
		case checker.ResultIsOk:
			return callExpr("result_is_ok", args...)
		case checker.ResultIsErr:
			return callExpr("result_is_err", args...)
		case checker.ResultMap:
			return callExpr("result_map", args...)
		case checker.ResultMapErr:
			return callExpr("result_map_err", args...)
		case checker.ResultAndThen:
			return callExpr("result_and_then", args...)
		default:
			return callExpr("result_method:"+resultMethodKindName(v.Kind), args...)
		}
	case *checker.ListLiteral:
		elements := make([]backendir.Expr, 0, len(v.Elements))
		for _, element := range v.Elements {
			elements = append(elements, lowerExpressionOrOpaque(element))
		}
		listType := lowerCheckerTypeToBackendIR(v.ListType)
		if _, ok := listType.(*backendir.ListType); !ok {
			listType = &backendir.ListType{Elem: backendir.Dynamic}
		}
		return &backendir.ListLiteralExpr{
			Type:     listType,
			Elements: elements,
		}
	case *checker.MapLiteral:
		entries := make([]backendir.MapEntry, 0, len(v.Keys))
		for i := range v.Keys {
			if i >= len(v.Values) {
				continue
			}
			entries = append(entries, backendir.MapEntry{
				Key:   lowerExpressionOrOpaque(v.Keys[i]),
				Value: lowerExpressionOrOpaque(v.Values[i]),
			})
		}
		mapType := lowerCheckerTypeToBackendIR(v.Type())
		if _, ok := mapType.(*backendir.MapType); !ok {
			mapType = &backendir.MapType{Key: backendir.Dynamic, Value: backendir.Dynamic}
		}
		return &backendir.MapLiteralExpr{
			Type:    mapType,
			Entries: entries,
		}
	case *checker.StructInstance:
		structType := lowerCheckerTypeToBackendIR(v.StructType)
		if _, ok := structType.(*backendir.NamedType); !ok {
			structType = &backendir.NamedType{Name: v.Name}
		}
		fields := make([]backendir.StructFieldValue, 0, len(v.Fields))
		for _, field := range sortedStringKeys(v.Fields) {
			fields = append(fields, backendir.StructFieldValue{
				Name:  field,
				Value: lowerExpressionOrOpaque(v.Fields[field]),
			})
		}
		return &backendir.StructLiteralExpr{
			Type:   structType,
			Fields: fields,
		}
	case *checker.ModuleStructInstance:
		if v.Property == nil {
			return callExpr("module_struct_literal", literalExpr("nil", "property"))
		}
		structType := lowerCheckerTypeToBackendIR(v.StructType)
		if _, ok := structType.(*backendir.NamedType); !ok {
			structType = &backendir.NamedType{Name: v.Property.Name}
		}
		fields := make([]backendir.StructFieldValue, 0, len(v.Property.Fields))
		for _, field := range sortedStringKeys(v.Property.Fields) {
			fields = append(fields, backendir.StructFieldValue{
				Name:  field,
				Value: lowerExpressionOrOpaque(v.Property.Fields[field]),
			})
		}
		return &backendir.StructLiteralExpr{
			Type:   structType,
			Fields: fields,
		}
	case *checker.EnumVariant:
		enumType := lowerCheckerTypeToBackendIR(v.EnumType)
		if named, ok := enumType.(*backendir.NamedType); !ok || strings.TrimSpace(named.Name) == "" {
			enumName := ""
			if sourceEnum, ok := v.EnumType.(*checker.Enum); ok {
				enumName = sourceEnum.Name
			}
			enumType = &backendir.NamedType{Name: enumName}
		}
		return &backendir.EnumVariantExpr{
			Type:         enumType,
			Discriminant: v.Discriminant,
		}
	case checker.EnumVariant:
		variant := v
		return lowerExpressionToBackendIR(&variant)
	case *checker.Not:
		return &backendir.CallExpr{
			Callee: &backendir.IdentExpr{Name: "not"},
			Args:   []backendir.Expr{lowerExpressionOrOpaque(v.Value)},
		}
	case *checker.Negation:
		return &backendir.CallExpr{
			Callee: &backendir.IdentExpr{Name: "neg"},
			Args:   []backendir.Expr{lowerExpressionOrOpaque(v.Value)},
		}
	case *checker.IntAddition:
		return lowerBinaryExprToBackendIR("int_add", v.Left, v.Right)
	case *checker.IntSubtraction:
		return lowerBinaryExprToBackendIR("int_sub", v.Left, v.Right)
	case *checker.IntMultiplication:
		return lowerBinaryExprToBackendIR("int_mul", v.Left, v.Right)
	case *checker.IntDivision:
		return lowerBinaryExprToBackendIR("int_div", v.Left, v.Right)
	case *checker.IntModulo:
		return lowerBinaryExprToBackendIR("int_mod", v.Left, v.Right)
	case *checker.IntGreater:
		return lowerBinaryExprToBackendIR("int_gt", v.Left, v.Right)
	case *checker.IntGreaterEqual:
		return lowerBinaryExprToBackendIR("int_gte", v.Left, v.Right)
	case *checker.IntLess:
		return lowerBinaryExprToBackendIR("int_lt", v.Left, v.Right)
	case *checker.IntLessEqual:
		return lowerBinaryExprToBackendIR("int_lte", v.Left, v.Right)
	case *checker.FloatAddition:
		return lowerBinaryExprToBackendIR("float_add", v.Left, v.Right)
	case *checker.FloatSubtraction:
		return lowerBinaryExprToBackendIR("float_sub", v.Left, v.Right)
	case *checker.FloatMultiplication:
		return lowerBinaryExprToBackendIR("float_mul", v.Left, v.Right)
	case *checker.FloatDivision:
		return lowerBinaryExprToBackendIR("float_div", v.Left, v.Right)
	case *checker.FloatGreater:
		return lowerBinaryExprToBackendIR("float_gt", v.Left, v.Right)
	case *checker.FloatGreaterEqual:
		return lowerBinaryExprToBackendIR("float_gte", v.Left, v.Right)
	case *checker.FloatLess:
		return lowerBinaryExprToBackendIR("float_lt", v.Left, v.Right)
	case *checker.FloatLessEqual:
		return lowerBinaryExprToBackendIR("float_lte", v.Left, v.Right)
	case *checker.StrAddition:
		return lowerBinaryExprToBackendIR("str_add", v.Left, v.Right)
	case *checker.Equality:
		return lowerBinaryExprToBackendIR("eq", v.Left, v.Right)
	case *checker.And:
		return lowerBinaryExprToBackendIR("and", v.Left, v.Right)
	case *checker.Or:
		return lowerBinaryExprToBackendIR("or", v.Left, v.Right)
	case *checker.If:
		return lowerIfExprToBackendIR(v)
	case *checker.BoolMatch:
		return lowerBoolMatchExprToBackendIR(v)
	case *checker.IntMatch:
		return lowerIntMatchExprToBackendIR(v)
	case *checker.ConditionalMatch:
		return lowerConditionalMatchExprToBackendIR(v)
	case *checker.OptionMatch:
		return lowerOptionMatchExprToBackendIR(v)
	case *checker.ResultMatch:
		return lowerResultMatchExprToBackendIR(v)
	case checker.ResultMatch:
		match := v
		return lowerExpressionToBackendIR(&match)
	case *checker.EnumMatch:
		return lowerEnumMatchExprToBackendIR(v)
	case *checker.UnionMatch:
		return lowerUnionMatchExprToBackendIR(v)
	case checker.TryOp:
		tryOp := v
		return lowerExpressionToBackendIR(&tryOp)
	case *checker.TryOp:
		return lowerTryOpExprToBackendIR(v)
	case *checker.CopyExpression:
		if _, ok := v.Type_.(*checker.List); ok {
			return &backendir.CopyExpr{
				Value: lowerExpressionOrOpaque(v.Expr),
				Type:  lowerCheckerTypeToBackendIR(v.Type_),
			}
		}
		return lowerExpressionOrOpaque(v.Expr)
	case checker.FiberStart:
		start := v
		return lowerExpressionToBackendIR(&start)
	case *checker.FiberStart:
		return callExpr("fiber_start", lowerExpressionOrOpaque(v.GetFn()))
	case checker.FiberEval:
		eval := v
		return lowerExpressionToBackendIR(&eval)
	case *checker.FiberEval:
		return callExpr("fiber_eval", lowerExpressionOrOpaque(v.GetFn()))
	case checker.FiberExecution:
		execution := v
		return lowerExpressionToBackendIR(&execution)
	case *checker.FiberExecution:
		modulePath := ""
		mainName := ""
		if v.GetModule() != nil {
			modulePath = v.GetModule().Path()
		}
		mainName = v.GetMainName()
		return callExpr(
			"fiber_execution",
			literalExpr("str", modulePath),
			literalExpr("str", mainName),
		)
	case *checker.FunctionDef:
		params := make([]backendir.Expr, 0, len(v.Parameters))
		for _, param := range v.Parameters {
			params = append(params, callExpr(
				"param",
				literalExpr("ident", param.Name),
				typeExpr(lowerCheckerTypeToBackendIR(param.Type)),
				literalExpr("bool", strconv.FormatBool(param.Mutable)),
			))
		}
		return callExpr(
			"fn_literal",
			literalExpr("ident", v.Name),
			typeExpr(lowerCheckerTypeToBackendIR(effectiveFunctionReturnType(v))),
			callExpr("params", params...),
			lowerBlockAsExpr(v.Body),
		)
	case *checker.ExternalFunctionDef:
		params := make([]backendir.Expr, 0, len(v.Parameters))
		for _, param := range v.Parameters {
			params = append(params, callExpr(
				"param",
				literalExpr("ident", param.Name),
				typeExpr(lowerCheckerTypeToBackendIR(param.Type)),
				literalExpr("bool", strconv.FormatBool(param.Mutable)),
			))
		}
		return callExpr(
			"extern_fn_literal",
			literalExpr("ident", v.Name),
			literalExpr("binding", strings.TrimSpace(v.ExternalBinding)),
			typeExpr(lowerCheckerTypeToBackendIR(v.ReturnType)),
			callExpr("params", params...),
		)
	case *checker.Panic:
		return &backendir.PanicExpr{
			Message: lowerExpressionOrOpaque(v.Message),
			Type:    lowerCheckerTypeToBackendIR(v.Type()),
		}
	case checker.Panic:
		return &backendir.PanicExpr{
			Message: lowerExpressionOrOpaque(v.Message),
			Type:    lowerCheckerTypeToBackendIR(v.Type()),
		}
	default:
		return callExpr(
			"unknown_expr",
			literalExpr("type", fmt.Sprintf("%T", expr)),
		)
	}
}

func lowerFunctionCallToBackendIR(call *checker.FunctionCall) backendir.Expr {
	if call == nil {
		return callExpr("call", literalExpr("nil", "call"))
	}
	args := make([]backendir.Expr, 0, len(call.Args))
	for _, arg := range call.Args {
		args = append(args, lowerExpressionOrOpaque(arg))
	}
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "anonymous_fn"
	}
	return &backendir.CallExpr{
		Callee: &backendir.IdentExpr{Name: name},
		Args:   args,
	}
}

func lowerModuleFunctionCallToBackendIR(call *checker.ModuleFunctionCall) backendir.Expr {
	if call == nil || call.Call == nil {
		return callExpr("module_call", literalExpr("nil", "call"))
	}
	args := make([]backendir.Expr, 0, len(call.Call.Args))
	for _, arg := range call.Call.Args {
		args = append(args, lowerExpressionOrOpaque(arg))
	}
	moduleName := strings.TrimSpace(call.Module)
	if moduleName == "" {
		moduleName = "module"
	}
	funcName := strings.TrimSpace(call.Call.Name)
	if funcName == "" {
		funcName = "fn"
	}
	return &backendir.CallExpr{
		Callee: &backendir.SelectorExpr{
			Subject: &backendir.IdentExpr{Name: moduleName},
			Name:    funcName,
		},
		Args: args,
	}
}

func lowerBinaryExprToBackendIR(name string, left checker.Expression, right checker.Expression) backendir.Expr {
	return &backendir.CallExpr{
		Callee: &backendir.IdentExpr{Name: name},
		Args: []backendir.Expr{
			lowerExpressionOrOpaque(left),
			lowerExpressionOrOpaque(right),
		},
	}
}

func lowerIfExprToBackendIR(expr *checker.If) backendir.Expr {
	if expr == nil {
		return callExpr("if_expr", literalExpr("nil", "if"))
	}
	resultType := lowerCheckerTypeToBackendIR(expr.Type())
	thenBlock := lowerBlockToBackendIR(expr.Body)
	finalizeFunctionBodyForReturn(thenBlock, resultType)

	var elseBlock *backendir.Block
	if expr.ElseIf != nil {
		nested := lowerIfExprToBackendIR(withElseFallback(expr.ElseIf, expr.Else))
		if isVoidIRType(resultType) {
			elseBlock = &backendir.Block{
				Stmts: []backendir.Stmt{
					&backendir.ExprStmt{Value: nested},
				},
			}
		} else {
			elseBlock = &backendir.Block{
				Stmts: []backendir.Stmt{
					&backendir.ReturnStmt{Value: nested},
				},
			}
		}
	} else if expr.Else != nil {
		elseBlock = lowerBlockToBackendIR(expr.Else)
		finalizeFunctionBodyForReturn(elseBlock, resultType)
	}

	return &backendir.IfExpr{
		Cond: lowerExpressionOrOpaque(expr.Condition),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
}

func lowerBoolMatchExprToBackendIR(match *checker.BoolMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "bool")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	thenBlock := lowerBlockToBackendIR(match.True)
	finalizeFunctionBodyForReturn(thenBlock, resultType)
	elseBlock := lowerBlockToBackendIR(match.False)
	finalizeFunctionBodyForReturn(elseBlock, resultType)
	return &backendir.IfExpr{
		Cond: lowerExpressionOrOpaque(match.Subject),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
}

func lowerIntMatchExprToBackendIR(match *checker.IntMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "int")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())

	subject, setup := matchSubjectExpr(match.Subject, "int")
	body, ok := buildIntMatchIfChain(match, subject, resultType)
	if !ok {
		// The checker should always produce an int match with at least
		// one branch or a non-void result that gets a synthetic panic
		// catch-all. This branch only fires for malformed checker output
		// which would have previously emitted an `int_match` marker
		// fallback. Marker artifacts are now rejected by validation, so
		// instead we emit an explicit invariant-failure PanicExpr.
		return invariantMatchFailureExpr(resultType, "int")
	}
	return wrapWithMatchSubjectSetup(body, setup, resultType)
}

func buildIntMatchIfChain(match *checker.IntMatch, subject backendir.Expr, resultType backendir.Type) (backendir.Expr, bool) {
	branches := make([]intMatchBranch, 0, len(match.IntCases)+len(match.RangeCases))
	for _, key := range sortedIntCaseKeys(match.IntCases) {
		block := match.IntCases[key]
		if block == nil {
			continue
		}
		branches = append(branches, intMatchBranch{
			Cond: callExpr("eq", subject, literalExpr("int", strconv.Itoa(key))),
			Body: block,
		})
	}
	for _, key := range sortedIntRangeCaseKeys(match.RangeCases) {
		block := match.RangeCases[key]
		if block == nil {
			continue
		}
		branches = append(branches, intMatchBranch{
			Cond: callExpr(
				"and",
				callExpr("int_gte", subject, literalExpr("int", strconv.Itoa(key.Start))),
				callExpr("int_lte", subject, literalExpr("int", strconv.Itoa(key.End))),
			),
			Body: block,
		})
	}
	// Lower the deepest else as a semantic backend IR Block (for catch-all)
	// or as a panic-bearing expression (for non-exhaustive non-void matches).
	// Avoid the legacy `block(...)` marker so emission can stay native and
	// preserve single-evaluation semantics for unsafe-subject matches.
	var deepestElseBlock *backendir.Block
	var deepestElseExpr backendir.Expr
	if match.CatchAll != nil {
		deepestElseBlock = lowerBlockToBackendIR(match.CatchAll)
		finalizeFunctionBodyForReturn(deepestElseBlock, resultType)
	} else if !isVoidIRType(resultType) {
		deepestElseExpr = nonExhaustiveMatchExpr(resultType, "non-exhaustive int match")
	}
	if len(branches) == 0 {
		if deepestElseBlock != nil {
			return lowerBlockAsExpr(match.CatchAll), true
		}
		if deepestElseExpr != nil {
			return deepestElseExpr, true
		}
		return nil, false
	}

	var nested *backendir.IfExpr
	for i := len(branches) - 1; i >= 0; i-- {
		thenBlock := lowerBlockToBackendIR(branches[i].Body)
		finalizeFunctionBodyForReturn(thenBlock, resultType)
		var elseBlock *backendir.Block
		switch {
		case nested != nil:
			elseBlock = wrapExprAsIfElseBlock(nested, resultType)
		case deepestElseBlock != nil:
			elseBlock = deepestElseBlock
		case deepestElseExpr != nil:
			elseBlock = wrapExprAsIfElseBlock(deepestElseExpr, resultType)
		}
		nested = &backendir.IfExpr{
			Cond: branches[i].Cond,
			Then: thenBlock,
			Else: elseBlock,
			Type: resultType,
		}
	}
	if nested == nil {
		return nil, false
	}
	return nested, true
}

type intMatchBranch struct {
	Cond backendir.Expr
	Body *checker.Block
}

func canSafelyDuplicateMatchSubject(subject checker.Expression) bool {
	switch subject.(type) {
	case *checker.Identifier, checker.Variable, *checker.Variable, *checker.IntLiteral, *checker.BoolLiteral, *checker.StrLiteral, *checker.FloatLiteral, *checker.ModuleSymbol:
		return true
	default:
		return false
	}
}

// matchSubjectTempPrefix is the leading marker for synthetic match-subject
// hoist temporaries. Its first character is a non-ASCII Unicode letter (Greek
// lowercase alpha, U+03B1) which Go accepts in identifiers but Ard's lexer
// cannot produce — Ard restricts identifier starts to ASCII `[A-Za-z_]` (see
// parse/lexer.go isAlpha). This guarantees that synthetic match temps cannot
// collide with any legal user-defined Ard identifier reaching the Go backend,
// so user locals are never silently shadowed or mutated by hoisting.
const matchSubjectTempPrefix = "\u03b1ardMatchSubject_"

// matchSubjectTempName returns a synthetic identifier used to bind a
// non-trivial match subject so it is evaluated only once before branch
// dispatch. The name is namespaced per match shape (int/option/result/enum)
// for readability of generated Go code, and is guaranteed to be unreachable
// from user-written Ard source (see matchSubjectTempPrefix).
func matchSubjectTempName(kind string) string {
	return matchSubjectTempPrefix + strings.TrimSpace(kind)
}

// matchSubjectExpr returns an expression that should be used inside the
// match's lowered branches to refer to its subject, along with the optional
// setup statement that hoists the subject's evaluation.
//
// For trivially duplicable subjects (identifiers/literals), the subject is
// lowered directly with no setup. For non-trivial subjects (calls, complex
// expressions), the subject is bound once to a synthetic temporary so that
// reuse across multiple branch conditions does not duplicate side effects.
func matchSubjectExpr(subject checker.Expression, kind string) (backendir.Expr, backendir.Stmt) {
	if subject == nil {
		return literalExpr("nil", "subject"), nil
	}
	if canSafelyDuplicateMatchSubject(subject) {
		return lowerExpressionOrOpaque(subject), nil
	}
	temp := matchSubjectTempName(kind)
	return &backendir.IdentExpr{Name: temp}, &backendir.AssignStmt{
		Target: temp,
		Value:  lowerExpressionOrOpaque(subject),
	}
}

// wrapWithMatchSubjectSetup wraps a lowered match body in a BlockExpr when a
// subject hoisting setup is required, preserving single-evaluation semantics
// for non-trivial match subjects.
func wrapWithMatchSubjectSetup(body backendir.Expr, setup backendir.Stmt, resultType backendir.Type) backendir.Expr {
	if body == nil {
		return body
	}
	if setup == nil {
		return body
	}
	return &backendir.BlockExpr{
		Setup: []backendir.Stmt{setup},
		Value: body,
		Type:  resultType,
	}
}

func wrapExprAsIfElseBlock(expr backendir.Expr, resultType backendir.Type) *backendir.Block {
	if expr == nil {
		return nil
	}
	if isVoidIRType(resultType) {
		return &backendir.Block{
			Stmts: []backendir.Stmt{
				&backendir.ExprStmt{Value: expr},
			},
		}
	}
	return &backendir.Block{
		Stmts: []backendir.Stmt{
			&backendir.ReturnStmt{Value: expr},
		},
	}
}

func lowerConditionalMatchExprToBackendIR(match *checker.ConditionalMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "conditional")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())

	branches := make([]conditionalMatchBranch, 0, len(match.Cases))
	for _, matchCase := range match.Cases {
		if matchCase.Condition == nil || matchCase.Body == nil {
			continue
		}
		branches = append(branches, conditionalMatchBranch{
			Cond: lowerExpressionOrOpaque(matchCase.Condition),
			Body: matchCase.Body,
		})
	}
	var nested backendir.Expr
	if match.CatchAll != nil {
		nested = lowerBlockAsExpr(match.CatchAll)
	} else if !isVoidIRType(resultType) {
		nested = nonExhaustiveMatchExpr(resultType, "non-exhaustive conditional match")
	}
	if len(branches) == 0 {
		if nested != nil {
			return nested
		}
		// Branches and catch-all both empty: previously fell back to a
		// `conditional_match` marker which is now rejected by validation.
		// Emit an explicit invariant-failure PanicExpr instead so misuse is
		// surfaced as a clear runtime panic instead of a smuggled marker.
		return invariantMatchFailureExpr(resultType, "conditional")
	}

	for i := len(branches) - 1; i >= 0; i-- {
		thenBlock := lowerBlockToBackendIR(branches[i].Body)
		finalizeFunctionBodyForReturn(thenBlock, resultType)
		nested = &backendir.IfExpr{
			Cond: branches[i].Cond,
			Then: thenBlock,
			Else: wrapExprAsIfElseBlock(nested, resultType),
			Type: resultType,
		}
	}
	if nested == nil {
		return invariantMatchFailureExpr(resultType, "conditional")
	}
	return nested
}

type conditionalMatchBranch struct {
	Cond backendir.Expr
	Body *checker.Block
}

func lowerOptionMatchExprToBackendIR(match *checker.OptionMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "option")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if match.Some == nil || match.None == nil {
		// Structurally invalid checker output (an option match must always
		// produce both Some and None branches). Previously emitted an
		// `option_match` marker which is now rejected by validation; emit
		// an explicit invariant-failure PanicExpr so misuse is observable
		// at runtime rather than smuggled past validation.
		return invariantMatchFailureExpr(resultType, "option")
	}
	subject, setup := matchSubjectExpr(match.Subject, "option")
	thenBlock := lowerBlockToBackendIR(match.Some.Body)
	prependBindingAssign(
		thenBlock,
		match.Some.Body,
		matchPatternName(match.Some.Pattern),
		callExpr(
			"maybe_expect",
			subject,
			literalExpr("str", "expected some"),
		),
	)
	finalizeFunctionBodyForReturn(thenBlock, resultType)
	elseBlock := lowerBlockToBackendIR(match.None)
	finalizeFunctionBodyForReturn(elseBlock, resultType)
	body := &backendir.IfExpr{
		Cond: callExpr("maybe_is_some", subject),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
	return wrapWithMatchSubjectSetup(body, setup, resultType)
}

func lowerResultMatchExprToBackendIR(match *checker.ResultMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "result")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())
	if match.Ok == nil || match.Err == nil {
		// Structurally invalid checker output (a result match must always
		// produce both Ok and Err branches). Previously emitted a
		// `result_match` marker which is now rejected by validation; emit
		// an explicit invariant-failure PanicExpr instead.
		return invariantMatchFailureExpr(resultType, "result")
	}
	subject, setup := matchSubjectExpr(match.Subject, "result")
	thenBlock := lowerBlockToBackendIR(match.Ok.Body)
	prependBindingAssign(
		thenBlock,
		match.Ok.Body,
		matchPatternName(match.Ok.Pattern),
		callExpr(
			"result_expect",
			subject,
			literalExpr("str", "expected ok"),
		),
	)
	finalizeFunctionBodyForReturn(thenBlock, resultType)
	elseBlock := lowerBlockToBackendIR(match.Err.Body)
	prependBindingAssign(
		elseBlock,
		match.Err.Body,
		matchPatternName(match.Err.Pattern),
		&backendir.CallExpr{
			Callee: &backendir.SelectorExpr{
				Subject: subject,
				Name:    "unwrap_err",
			},
			Args: nil,
		},
	)
	finalizeFunctionBodyForReturn(elseBlock, resultType)
	body := &backendir.IfExpr{
		Cond: callExpr("result_is_ok", subject),
		Then: thenBlock,
		Else: elseBlock,
		Type: resultType,
	}
	return wrapWithMatchSubjectSetup(body, setup, resultType)
}

func lowerEnumMatchExprToBackendIR(match *checker.EnumMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "enum")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())

	subject, setup := matchSubjectExpr(match.Subject, "enum")
	subjectTag := &backendir.SelectorExpr{
		Subject: subject,
		Name:    "tag",
	}
	// Lower the deepest else as a semantic backend IR Block (for catch-all)
	// or as a panic-bearing expression (for non-exhaustive non-void matches).
	// Avoid the legacy `block(...)` marker so emission can stay native and
	// preserve single-evaluation semantics for unsafe-subject matches.
	var deepestElseBlock *backendir.Block
	var deepestElseExpr backendir.Expr
	if match.CatchAll != nil {
		deepestElseBlock = lowerBlockToBackendIR(match.CatchAll)
		finalizeFunctionBodyForReturn(deepestElseBlock, resultType)
	} else if !isVoidIRType(resultType) {
		deepestElseExpr = nonExhaustiveMatchExpr(resultType, "non-exhaustive enum match")
	}

	var nested *backendir.IfExpr
	for index := len(match.Cases) - 1; index >= 0; index-- {
		block := match.Cases[index]
		if block == nil {
			continue
		}
		thenBlock := lowerBlockToBackendIR(block)
		finalizeFunctionBodyForReturn(thenBlock, resultType)
		var elseBlock *backendir.Block
		switch {
		case nested != nil:
			elseBlock = wrapExprAsIfElseBlock(nested, resultType)
		case deepestElseBlock != nil:
			elseBlock = deepestElseBlock
		case deepestElseExpr != nil:
			elseBlock = wrapExprAsIfElseBlock(deepestElseExpr, resultType)
		}
		nested = &backendir.IfExpr{
			Cond: callExpr("eq", subjectTag, literalExpr("int", strconv.Itoa(index))),
			Then: thenBlock,
			Else: elseBlock,
			Type: resultType,
		}
	}
	if nested == nil {
		// All cases were nil-bodied or empty, leaving no usable branches.
		// Previously emitted an `enum_match` marker which is rejected by
		// validation; emit an explicit invariant-failure PanicExpr.
		return invariantMatchFailureExpr(resultType, "enum")
	}
	return wrapWithMatchSubjectSetup(nested, setup, resultType)
}

func lowerUnionMatchExprToBackendIR(match *checker.UnionMatch) backendir.Expr {
	if match == nil {
		return invariantMatchFailureExpr(backendir.Void, "union")
	}
	resultType := lowerCheckerTypeToBackendIR(match.Type())

	cases := make([]backendir.UnionMatchCase, 0, len(match.TypeCases))
	for _, caseName := range sortedStringKeys(match.TypeCases) {
		matchCase := match.TypeCases[caseName]
		if matchCase == nil || matchCase.Body == nil {
			continue
		}
		caseType := unionMatchCaseTypeByName(match, caseName)
		if caseType == nil {
			// Missing case type for a named union case is a structural
			// invariant violation. Previously fell back to a `union_match`
			// marker which is rejected by validation; emit an explicit
			// invariant-failure PanicExpr instead.
			return invariantMatchFailureExpr(resultType, "union")
		}
		body := lowerBlockToBackendIR(matchCase.Body)
		finalizeFunctionBodyForReturn(body, resultType)
		cases = append(cases, backendir.UnionMatchCase{
			Type:    lowerCheckerTypeToBackendIR(caseType),
			Pattern: matchPatternName(matchCase.Pattern),
			Body:    body,
		})
	}
	if len(cases) == 0 {
		// No usable cases at all — previously emitted `union_match` marker;
		// surface the invariant violation directly.
		return invariantMatchFailureExpr(resultType, "union")
	}

	var catchAll *backendir.Block
	if match.CatchAll != nil {
		catchAll = lowerBlockToBackendIR(match.CatchAll)
		finalizeFunctionBodyForReturn(catchAll, resultType)
	} else if !isVoidIRType(resultType) {
		catchAll = nonExhaustiveMatchBlock("non-exhaustive union match")
	}
	return &backendir.UnionMatchExpr{
		Subject:  lowerExpressionOrOpaque(match.Subject),
		Cases:    cases,
		CatchAll: catchAll,
		Type:     resultType,
	}
}

func unionMatchCaseTypeByName(match *checker.UnionMatch, caseName string) checker.Type {
	if match == nil {
		return nil
	}
	for caseType := range match.TypeCasesByType {
		if caseType != nil && caseType.String() == caseName {
			return caseType
		}
	}
	return nil
}

// invariantMatchFailureExpr replaces the legacy `*_match` marker fallback
// returns. It produces a typed PanicExpr that fails loudly if reached. These
// fallbacks are only entered for structurally invalid checker output (for
// example, an option/result match missing one of its branches, or a union
// match with no usable cases). Previously the lowering would emit a marker
// CallExpr (e.g. `int_match(...)`) that the legacy emitter could pick up;
// marker artifacts are now rejected by the IR validator (VAL-CROSS-005), so
// no successful compile path can produce them. Surfacing the violation as
// an explicit PanicExpr keeps the lowering output validatable while still
// terminating execution if the unreachable path is somehow reached.
func invariantMatchFailureExpr(resultType backendir.Type, kind string) backendir.Expr {
	if resultType == nil {
		resultType = backendir.Void
	}
	message := fmt.Sprintf("invariant: %s match lowering reached unreachable fallback", strings.TrimSpace(kind))
	return &backendir.PanicExpr{
		Message: literalExpr("str", message),
		Type:    resultType,
	}
}

func nonExhaustiveMatchExpr(resultType backendir.Type, message string) backendir.Expr {
	// Emit the non-exhaustive panic directly as a typed PanicExpr so the
	// surrounding else branch of the lowered match IfExpr-chain can be
	// emitted natively (PanicExpr is natively emittable, whereas an
	// IfExpr-with-no-else of non-void type cannot be). The PanicExpr is
	// expression-positioned and never falls through, so its return type
	// is satisfied by the panic itself.
	return &backendir.PanicExpr{
		Message: literalExpr("str", strings.TrimSpace(message)),
		Type:    resultType,
	}
}

func nonExhaustiveMatchBlock(message string) *backendir.Block {
	return &backendir.Block{
		Stmts: []backendir.Stmt{
			&backendir.ExprStmt{
				Value: &backendir.PanicExpr{
					Message: literalExpr("str", strings.TrimSpace(message)),
					Type:    backendir.Void,
				},
			},
		},
	}
}

func lowerTryOpExprToBackendIR(op *checker.TryOp) backendir.Expr {
	if op == nil {
		return &backendir.PanicExpr{
			Message: literalExpr("str", "invalid try expression"),
			Type:    backendir.Void,
		}
	}
	kind := ""
	switch op.Kind {
	case checker.TryMaybe:
		kind = "maybe"
	default:
		kind = "result"
	}
	resultType := lowerCheckerTypeToBackendIR(op.Type())
	if op.CatchBlock == nil {
		return &backendir.TryExpr{
			Kind:    kind,
			Subject: lowerExpressionOrOpaque(op.Expr()),
			Catch:   nil,
			Type:    resultType,
		}
	}
	catchBlock := lowerBlockToBackendIR(op.CatchBlock)
	// The catch block always early-returns from the enclosing function. Per the
	// checker, the catch block's value type matches the function's return type
	// (whether or not it equals the unwrapped success type). Finalize the catch
	// block so its trailing expression becomes a return statement, mirroring the
	// VM's `OpReturn` after the catch body.
	finalizeFunctionBodyForReturn(catchBlock, lowerCheckerTypeToBackendIR(op.CatchBlock.Type()))
	return &backendir.TryExpr{
		Kind:     kind,
		Subject:  lowerExpressionOrOpaque(op.Expr()),
		CatchVar: strings.TrimSpace(op.CatchVar),
		Catch:    catchBlock,
		Type:     resultType,
	}
}

func prependBindingAssign(block *backendir.Block, source *checker.Block, name string, value backendir.Expr) {
	name = strings.TrimSpace(name)
	if block == nil || value == nil || name == "" || name == "_" {
		return
	}
	prefix := []backendir.Stmt{
		&backendir.AssignStmt{
			Target: name,
			Value:  value,
		},
	}
	if source == nil || !usesNameInStatements(source.Stmts, name) {
		prefix = append(prefix, &backendir.AssignStmt{
			Target: "_",
			Value:  &backendir.IdentExpr{Name: name},
		})
	}
	block.Stmts = append(prefix, block.Stmts...)
}

func matchPatternName(pattern *checker.Identifier) string {
	if pattern == nil {
		return ""
	}
	return strings.TrimSpace(pattern.Name)
}

func lowerAssignmentTargetName(expr checker.Expression) string {
	switch target := expr.(type) {
	case *checker.Identifier:
		return target.Name
	case checker.Variable:
		return target.Name()
	case *checker.Variable:
		return target.Name()
	case *checker.InstanceProperty:
		return fmt.Sprintf("%s.%s", expressionDebugName(target.Subject), target.Property)
	default:
		return fmt.Sprintf("<target:%T>", expr)
	}
}

func expressionDebugName(expr checker.Expression) string {
	switch value := expr.(type) {
	case *checker.Identifier:
		return value.Name
	case checker.Variable:
		return value.Name()
	case *checker.Variable:
		return value.Name()
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func callExpr(name string, args ...backendir.Expr) backendir.Expr {
	return &backendir.CallExpr{
		Callee: &backendir.IdentExpr{Name: name},
		Args:   args,
	}
}

func literalExpr(kind, value string) backendir.Expr {
	return &backendir.LiteralExpr{
		Kind:  kind,
		Value: value,
	}
}

func typeExpr(t backendir.Type) backendir.Expr {
	return literalExpr("type", backendIRTypeName(t))
}

func lowerMatchAsExpr(match *checker.Match) backendir.Expr {
	if match == nil {
		return literalExpr("match", "nil")
	}
	pattern := ""
	if match.Pattern != nil {
		pattern = match.Pattern.Name
	}
	return callExpr(
		"match_case",
		literalExpr("ident", pattern),
		lowerBlockAsExpr(match.Body),
	)
}

func sortedIntCaseKeys(values map[int]*checker.Block) []int {
	if len(values) == 0 {
		return nil
	}
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedIntRangeCaseKeys(values map[checker.IntRange]*checker.Block) []checker.IntRange {
	if len(values) == 0 {
		return nil
	}
	keys := make([]checker.IntRange, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(left checker.IntRange, right checker.IntRange) int {
		if left.Start != right.Start {
			return left.Start - right.Start
		}
		return left.End - right.End
	})
	return keys
}

func strMethodKindName(kind checker.StrMethodKind) string {
	switch kind {
	case checker.StrSize:
		return "size"
	case checker.StrIsEmpty:
		return "is_empty"
	case checker.StrContains:
		return "contains"
	case checker.StrReplace:
		return "replace"
	case checker.StrReplaceAll:
		return "replace_all"
	case checker.StrSplit:
		return "split"
	case checker.StrStartsWith:
		return "starts_with"
	case checker.StrToStr:
		return "to_str"
	case checker.StrToDyn:
		return "to_dyn"
	case checker.StrTrim:
		return "trim"
	default:
		return "unknown"
	}
}

func intMethodKindName(kind checker.IntMethodKind) string {
	switch kind {
	case checker.IntToStr:
		return "to_str"
	case checker.IntToDyn:
		return "to_dyn"
	default:
		return "unknown"
	}
}

func floatMethodKindName(kind checker.FloatMethodKind) string {
	switch kind {
	case checker.FloatToStr:
		return "to_str"
	case checker.FloatToInt:
		return "to_int"
	case checker.FloatToDyn:
		return "to_dyn"
	default:
		return "unknown"
	}
}

func boolMethodKindName(kind checker.BoolMethodKind) string {
	switch kind {
	case checker.BoolToStr:
		return "to_str"
	case checker.BoolToDyn:
		return "to_dyn"
	default:
		return "unknown"
	}
}

func listMethodKindName(kind checker.ListMethodKind) string {
	switch kind {
	case checker.ListAt:
		return "at"
	case checker.ListPrepend:
		return "prepend"
	case checker.ListPush:
		return "push"
	case checker.ListSet:
		return "set"
	case checker.ListSize:
		return "size"
	case checker.ListSort:
		return "sort"
	case checker.ListSwap:
		return "swap"
	default:
		return "unknown"
	}
}

func mapMethodKindName(kind checker.MapMethodKind) string {
	switch kind {
	case checker.MapKeys:
		return "keys"
	case checker.MapSize:
		return "size"
	case checker.MapGet:
		return "get"
	case checker.MapSet:
		return "set"
	case checker.MapDrop:
		return "drop"
	case checker.MapHas:
		return "has"
	default:
		return "unknown"
	}
}

func maybeMethodKindName(kind checker.MaybeMethodKind) string {
	switch kind {
	case checker.MaybeExpect:
		return "expect"
	case checker.MaybeIsNone:
		return "is_none"
	case checker.MaybeIsSome:
		return "is_some"
	case checker.MaybeOr:
		return "or"
	case checker.MaybeMap:
		return "map"
	case checker.MaybeAndThen:
		return "and_then"
	default:
		return "unknown"
	}
}

func resultMethodKindName(kind checker.ResultMethodKind) string {
	switch kind {
	case checker.ResultExpect:
		return "expect"
	case checker.ResultOr:
		return "or"
	case checker.ResultIsOk:
		return "is_ok"
	case checker.ResultIsErr:
		return "is_err"
	case checker.ResultMap:
		return "map"
	case checker.ResultMapErr:
		return "map_err"
	case checker.ResultAndThen:
		return "and_then"
	default:
		return "unknown"
	}
}

func backendIRTypeName(t backendir.Type) string {
	switch typed := t.(type) {
	case nil:
		return "Unknown"
	case *backendir.PrimitiveType:
		return typed.Name
	case *backendir.DynamicType:
		return "Dynamic"
	case *backendir.VoidType:
		return "Void"
	case *backendir.TypeVarType:
		return "$" + typed.Name
	case *backendir.NamedType:
		if len(typed.Args) == 0 {
			return typed.Name
		}
		parts := make([]string, 0, len(typed.Args))
		for _, arg := range typed.Args {
			parts = append(parts, backendIRTypeName(arg))
		}
		return typed.Name + "<" + strings.Join(parts, ", ") + ">"
	case *backendir.ListType:
		return "[" + backendIRTypeName(typed.Elem) + "]"
	case *backendir.MapType:
		return "[" + backendIRTypeName(typed.Key) + ":" + backendIRTypeName(typed.Value) + "]"
	case *backendir.MaybeType:
		return backendIRTypeName(typed.Of) + "?"
	case *backendir.ResultType:
		return backendIRTypeName(typed.Val) + "!" + backendIRTypeName(typed.Err)
	case *backendir.FuncType:
		params := make([]string, 0, len(typed.Params))
		for _, param := range typed.Params {
			params = append(params, backendIRTypeName(param))
		}
		return "fn(" + strings.Join(params, ",") + ") " + backendIRTypeName(typed.Return)
	default:
		return fmt.Sprintf("%T", t)
	}
}

// lowerNestedCheckerTypeToBackendIR lowers checker types when they appear
// nested inside other type constructors (list element, map key/value,
// maybe inner, result val/err). Union types in such nested positions are
// kept as Dynamic to preserve runtime FFI compatibility — the existing
// extern bridge expects `[]any`/`map[string]any` payloads for
// dynamically-typed extern arguments. Direct (non-nested) union types,
// such as a function parameter or return type whose immediate type IS a
// union, still lower to backend IR NamedType so signature emission can
// reference the concrete union interface.
func lowerNestedCheckerTypeToBackendIR(t checker.Type) backendir.Type {
	switch typed := t.(type) {
	case *checker.Union:
		_ = typed
		return backendir.Dynamic
	case checker.Union:
		_ = typed
		return backendir.Dynamic
	}
	return lowerCheckerTypeToBackendIR(t)
}

func lowerCheckerTypeToBackendIR(t checker.Type) backendir.Type {
	if t == nil {
		return backendir.UnknownType
	}

	switch t {
	case checker.Int:
		return backendir.IntType
	case checker.Float:
		return backendir.FloatType
	case checker.Str:
		return backendir.StrType
	case checker.Bool:
		return backendir.BoolType
	case checker.Dynamic:
		return backendir.Dynamic
	case checker.Void:
		return backendir.Void
	}

	switch typed := t.(type) {
	case checker.Trait:
		trait := typed
		return lowerCheckerTypeToBackendIR(&trait)
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return lowerCheckerTypeToBackendIR(actual)
		}
		name := strings.TrimSpace(typed.Name())
		if name == "" {
			name = "T"
		}
		return &backendir.TypeVarType{Name: name}
	case *checker.List:
		return &backendir.ListType{Elem: lowerNestedCheckerTypeToBackendIR(typed.Of())}
	case *checker.Map:
		return &backendir.MapType{
			Key:   lowerNestedCheckerTypeToBackendIR(typed.Key()),
			Value: lowerNestedCheckerTypeToBackendIR(typed.Value()),
		}
	case *checker.Maybe:
		return &backendir.MaybeType{Of: lowerNestedCheckerTypeToBackendIR(typed.Of())}
	case *checker.Result:
		return &backendir.ResultType{
			Val: lowerNestedCheckerTypeToBackendIR(typed.Val()),
			Err: lowerNestedCheckerTypeToBackendIR(typed.Err()),
		}
	case *checker.FunctionDef:
		params := make([]backendir.Type, 0, len(typed.Parameters))
		for _, param := range typed.Parameters {
			params = append(params, lowerCheckerTypeToBackendIR(param.Type))
		}
		return &backendir.FuncType{
			Params: params,
			Return: lowerCheckerTypeToBackendIR(effectiveFunctionReturnType(typed)),
		}
	case *checker.ExternalFunctionDef:
		params := make([]backendir.Type, 0, len(typed.Parameters))
		for _, param := range typed.Parameters {
			params = append(params, lowerCheckerTypeToBackendIR(param.Type))
		}
		return &backendir.FuncType{
			Params: params,
			Return: lowerCheckerTypeToBackendIR(typed.ReturnType),
		}
	case *checker.Trait:
		return backendir.Dynamic
	case *checker.StructDef:
		order := structTypeParamOrder(typed)
		if len(order) == 0 {
			return &backendir.NamedType{Name: typed.Name}
		}
		bindings := inferStructBoundTypeArgs(typed, order, nil)
		args := make([]backendir.Type, 0, len(order))
		for _, name := range order {
			bound := bindings[name]
			if tv, ok := bound.(*checker.TypeVar); ok {
				if actual := tv.Actual(); actual != nil {
					bound = actual
				} else {
					bound = nil
				}
			}
			if bound == nil {
				args = append(args, &backendir.TypeVarType{Name: name})
				continue
			}
			args = append(args, lowerCheckerTypeToBackendIR(bound))
		}
		return &backendir.NamedType{Name: typed.Name, Args: args}
	case *checker.Enum:
		return &backendir.NamedType{Name: typed.Name}
	case checker.Union:
		name := strings.TrimSpace(typed.Name)
		if name == "" {
			return backendir.Dynamic
		}
		return &backendir.NamedType{Name: name}
	case *checker.Union:
		if typed == nil {
			return backendir.Dynamic
		}
		name := strings.TrimSpace(typed.Name)
		if name == "" {
			return backendir.Dynamic
		}
		return &backendir.NamedType{Name: name}
	case *checker.ExternType:
		args := make([]backendir.Type, 0, len(typed.TypeArgs))
		for _, typeArg := range typed.TypeArgs {
			args = append(args, lowerCheckerTypeToBackendIR(typeArg))
		}
		name := strings.TrimSpace(typed.Name_)
		if name == "" {
			name = "Extern"
		}
		return &backendir.NamedType{Name: name, Args: args}
	default:
		return &backendir.NamedType{Name: t.String()}
	}
}
