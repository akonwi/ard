package ir

import (
	"fmt"
	"strings"
)

// markerFallbackArtifactNames lists the legacy try/match marker fallback
// callee names that are no longer permitted in finalized backend IR for
// migrated surfaces. Lowering must produce semantic IR shapes (TryExpr,
// IfExpr, UnionMatchExpr, BlockExpr, PanicExpr, ...) for these surfaces;
// any CallExpr whose callee identifier resolves to one of these names is a
// marker-style fallback artifact and must be rejected by validation so the
// build fails loudly instead of silently relying on emitter-side fallback.
var markerFallbackArtifactNames = map[string]struct{}{
	"try_op":            {},
	"bool_match":        {},
	"int_match":         {},
	"conditional_match": {},
	"option_match":      {},
	"result_match":      {},
	"enum_match":        {},
	"union_match":       {},
}

// markerFallbackArtifactName returns the marker artifact name and true when
// expr is a CallExpr whose callee is an IdentExpr naming a legacy marker
// fallback artifact for migrated try/match surfaces.
func markerFallbackArtifactName(expr Expr) (string, bool) {
	call, ok := expr.(*CallExpr)
	if !ok || call == nil {
		return "", false
	}
	ident, ok := call.Callee.(*IdentExpr)
	if !ok || ident == nil {
		return "", false
	}
	name := strings.TrimSpace(ident.Name)
	if _, isMarker := markerFallbackArtifactNames[name]; !isMarker {
		return "", false
	}
	return name, true
}

func ValidateModule(module *Module) error {
	if module == nil {
		return fmt.Errorf("nil module")
	}
	if strings.TrimSpace(module.PackageName) == "" {
		return fmt.Errorf("module package name is empty")
	}

	seenDeclNames := map[string]struct{}{}
	for i, decl := range module.Decls {
		name := declName(decl)
		if name != "" {
			if _, exists := seenDeclNames[name]; exists {
				return fmt.Errorf("decl[%d]: duplicate declaration name %q", i, name)
			}
			seenDeclNames[name] = struct{}{}
		}
		if err := validateDecl(decl); err != nil {
			return fmt.Errorf("decl[%d]: %w", i, err)
		}
	}
	if module.Entrypoint != nil {
		if err := validateBlock(module.Entrypoint); err != nil {
			return fmt.Errorf("entrypoint: %w", err)
		}
	}

	return nil
}

func declName(decl Decl) string {
	switch d := decl.(type) {
	case *StructDecl:
		return "struct:" + d.Name
	case *EnumDecl:
		return "enum:" + d.Name
	case *UnionDecl:
		return "union:" + d.Name
	case *ExternTypeDecl:
		return "extern_type:" + d.Name
	case *FuncDecl:
		return "func:" + d.Name
	case *VarDecl:
		return "var:" + d.Name
	default:
		return ""
	}
}

func validateDecl(decl Decl) error {
	switch d := decl.(type) {
	case *StructDecl:
		return validateStructDecl(d)
	case *EnumDecl:
		return validateEnumDecl(d)
	case *UnionDecl:
		return validateUnionDecl(d)
	case *ExternTypeDecl:
		return validateExternTypeDecl(d)
	case *FuncDecl:
		return validateFuncDecl(d)
	case *VarDecl:
		return validateVarDecl(d)
	default:
		return fmt.Errorf("unsupported declaration type %T", decl)
	}
}

func validateStructDecl(decl *StructDecl) error {
	if decl == nil {
		return fmt.Errorf("nil struct declaration")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("struct name is empty")
	}
	seenFields := map[string]struct{}{}
	for i, field := range decl.Fields {
		fieldName := strings.TrimSpace(field.Name)
		if fieldName == "" {
			return fmt.Errorf("field[%d] name is empty", i)
		}
		if _, exists := seenFields[fieldName]; exists {
			return fmt.Errorf("field[%d] duplicate name %q", i, fieldName)
		}
		seenFields[fieldName] = struct{}{}
		if err := validateType(field.Type); err != nil {
			return fmt.Errorf("field[%d] type: %w", i, err)
		}
	}
	return nil
}

func validateEnumDecl(decl *EnumDecl) error {
	if decl == nil {
		return fmt.Errorf("nil enum declaration")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("enum name is empty")
	}
	seenValues := map[string]struct{}{}
	for i, value := range decl.Values {
		valueName := strings.TrimSpace(value.Name)
		if valueName == "" {
			return fmt.Errorf("value[%d] name is empty", i)
		}
		if _, exists := seenValues[valueName]; exists {
			return fmt.Errorf("value[%d] duplicate name %q", i, valueName)
		}
		seenValues[valueName] = struct{}{}
	}
	return nil
}

func validateUnionDecl(decl *UnionDecl) error {
	if decl == nil {
		return fmt.Errorf("nil union declaration")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("union name is empty")
	}
	if len(decl.Types) == 0 {
		return fmt.Errorf("union types are empty")
	}
	for i, typ := range decl.Types {
		if err := validateType(typ); err != nil {
			return fmt.Errorf("union type[%d]: %w", i, err)
		}
	}
	return nil
}

func validateExternTypeDecl(decl *ExternTypeDecl) error {
	if decl == nil {
		return fmt.Errorf("nil extern type declaration")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("extern type name is empty")
	}
	for i, arg := range decl.Args {
		if err := validateType(arg); err != nil {
			return fmt.Errorf("extern type arg[%d]: %w", i, err)
		}
	}
	return nil
}

func validateFuncDecl(decl *FuncDecl) error {
	if decl == nil {
		return fmt.Errorf("nil function declaration")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("function name is empty")
	}
	seenParams := map[string]struct{}{}
	for i, param := range decl.Params {
		paramName := strings.TrimSpace(param.Name)
		if paramName == "" {
			return fmt.Errorf("param[%d] name is empty", i)
		}
		if _, exists := seenParams[paramName]; exists {
			return fmt.Errorf("param[%d] duplicate name %q", i, paramName)
		}
		seenParams[paramName] = struct{}{}
		if err := validateType(param.Type); err != nil {
			return fmt.Errorf("param[%d] type: %w", i, err)
		}
	}
	if err := validateType(decl.Return); err != nil {
		return fmt.Errorf("return type: %w", err)
	}

	if decl.IsExtern {
		if decl.Body != nil {
			return fmt.Errorf("extern function must not define body")
		}
		if strings.TrimSpace(decl.ExternBinding) == "" {
			return fmt.Errorf("extern function binding is empty")
		}
		return nil
	}

	if decl.Body == nil {
		return fmt.Errorf("non-extern function body is nil")
	}
	return validateBlock(decl.Body)
}

func validateVarDecl(decl *VarDecl) error {
	if decl == nil {
		return fmt.Errorf("nil variable declaration")
	}
	if strings.TrimSpace(decl.Name) == "" {
		return fmt.Errorf("variable name is empty")
	}
	if err := validateType(decl.Type); err != nil {
		return fmt.Errorf("variable type: %w", err)
	}
	if decl.Value == nil {
		return fmt.Errorf("variable value is nil")
	}
	if err := validateExpr(decl.Value); err != nil {
		return fmt.Errorf("variable value: %w", err)
	}
	return nil
}

func validateBlock(block *Block) error {
	if block == nil {
		return fmt.Errorf("nil block")
	}
	for i, stmt := range block.Stmts {
		if err := validateStmt(stmt); err != nil {
			return fmt.Errorf("stmt[%d]: %w", i, err)
		}
	}
	return nil
}

func validateStmt(stmt Stmt) error {
	switch s := stmt.(type) {
	case *ReturnStmt:
		if s == nil {
			return fmt.Errorf("nil return statement")
		}
		if s.Value == nil {
			return nil
		}
		return validateExpr(s.Value)
	case *ExprStmt:
		if s == nil || s.Value == nil {
			return fmt.Errorf("expr statement value is nil")
		}
		return validateExpr(s.Value)
	case *BreakStmt:
		if s == nil {
			return fmt.Errorf("nil break statement")
		}
		return nil
	case *AssignStmt:
		if s == nil {
			return fmt.Errorf("nil assign statement")
		}
		if strings.TrimSpace(s.Target) == "" {
			return fmt.Errorf("assign target is empty")
		}
		if s.Value == nil {
			return fmt.Errorf("assign value is nil")
		}
		return validateExpr(s.Value)
	case *MemberAssignStmt:
		if s == nil {
			return fmt.Errorf("nil member assign statement")
		}
		if s.Subject == nil {
			return fmt.Errorf("member assign subject is nil")
		}
		if strings.TrimSpace(s.Field) == "" {
			return fmt.Errorf("member assign field is empty")
		}
		if s.Value == nil {
			return fmt.Errorf("member assign value is nil")
		}
		if err := validateExpr(s.Subject); err != nil {
			return fmt.Errorf("member assign subject: %w", err)
		}
		return validateExpr(s.Value)
	case *ForIntRangeStmt:
		if s == nil {
			return fmt.Errorf("nil for-int-range statement")
		}
		if strings.TrimSpace(s.Cursor) == "" {
			return fmt.Errorf("for-int-range cursor is empty")
		}
		if strings.TrimSpace(s.Index) != "" && strings.TrimSpace(s.Index) == strings.TrimSpace(s.Cursor) {
			return fmt.Errorf("for-int-range index must differ from cursor")
		}
		if s.Start == nil {
			return fmt.Errorf("for-int-range start is nil")
		}
		if s.End == nil {
			return fmt.Errorf("for-int-range end is nil")
		}
		if s.Body == nil {
			return fmt.Errorf("for-int-range body is nil")
		}
		if err := validateExpr(s.Start); err != nil {
			return fmt.Errorf("for-int-range start: %w", err)
		}
		if err := validateExpr(s.End); err != nil {
			return fmt.Errorf("for-int-range end: %w", err)
		}
		return validateBlock(s.Body)
	case *ForLoopStmt:
		if s == nil {
			return fmt.Errorf("nil for-loop statement")
		}
		if strings.TrimSpace(s.InitName) == "" {
			return fmt.Errorf("for-loop init name is empty")
		}
		if s.InitValue == nil {
			return fmt.Errorf("for-loop init value is nil")
		}
		if s.Cond == nil {
			return fmt.Errorf("for-loop condition is nil")
		}
		if s.Update == nil {
			return fmt.Errorf("for-loop update is nil")
		}
		switch s.Update.(type) {
		case *AssignStmt, *MemberAssignStmt:
		default:
			return fmt.Errorf("for-loop update must be assign statement")
		}
		if s.Body == nil {
			return fmt.Errorf("for-loop body is nil")
		}
		if err := validateExpr(s.InitValue); err != nil {
			return fmt.Errorf("for-loop init value: %w", err)
		}
		if err := validateExpr(s.Cond); err != nil {
			return fmt.Errorf("for-loop condition: %w", err)
		}
		if err := validateStmt(s.Update); err != nil {
			return fmt.Errorf("for-loop update: %w", err)
		}
		return validateBlock(s.Body)
	case *ForInStrStmt:
		if s == nil {
			return fmt.Errorf("nil for-in-str statement")
		}
		if strings.TrimSpace(s.Cursor) == "" {
			return fmt.Errorf("for-in-str cursor is empty")
		}
		if strings.TrimSpace(s.Index) != "" && strings.TrimSpace(s.Index) == strings.TrimSpace(s.Cursor) {
			return fmt.Errorf("for-in-str index must differ from cursor")
		}
		if s.Value == nil {
			return fmt.Errorf("for-in-str value is nil")
		}
		if s.Body == nil {
			return fmt.Errorf("for-in-str body is nil")
		}
		if err := validateExpr(s.Value); err != nil {
			return fmt.Errorf("for-in-str value: %w", err)
		}
		return validateBlock(s.Body)
	case *ForInListStmt:
		if s == nil {
			return fmt.Errorf("nil for-in-list statement")
		}
		if strings.TrimSpace(s.Cursor) == "" {
			return fmt.Errorf("for-in-list cursor is empty")
		}
		if strings.TrimSpace(s.Index) != "" && strings.TrimSpace(s.Index) == strings.TrimSpace(s.Cursor) {
			return fmt.Errorf("for-in-list index must differ from cursor")
		}
		if s.List == nil {
			return fmt.Errorf("for-in-list list is nil")
		}
		if s.Body == nil {
			return fmt.Errorf("for-in-list body is nil")
		}
		if err := validateExpr(s.List); err != nil {
			return fmt.Errorf("for-in-list list: %w", err)
		}
		return validateBlock(s.Body)
	case *ForInMapStmt:
		if s == nil {
			return fmt.Errorf("nil for-in-map statement")
		}
		if strings.TrimSpace(s.Key) == "" {
			return fmt.Errorf("for-in-map key is empty")
		}
		if strings.TrimSpace(s.Value) == "" {
			return fmt.Errorf("for-in-map value is empty")
		}
		if s.Map == nil {
			return fmt.Errorf("for-in-map map is nil")
		}
		if s.Body == nil {
			return fmt.Errorf("for-in-map body is nil")
		}
		if err := validateExpr(s.Map); err != nil {
			return fmt.Errorf("for-in-map map: %w", err)
		}
		return validateBlock(s.Body)
	case *WhileStmt:
		if s == nil {
			return fmt.Errorf("nil while statement")
		}
		if s.Cond == nil {
			return fmt.Errorf("while condition is nil")
		}
		if s.Body == nil {
			return fmt.Errorf("while body is nil")
		}
		if err := validateExpr(s.Cond); err != nil {
			return fmt.Errorf("while condition: %w", err)
		}
		return validateBlock(s.Body)
	case *IfStmt:
		if s == nil {
			return fmt.Errorf("nil if statement")
		}
		if s.Cond == nil {
			return fmt.Errorf("if condition is nil")
		}
		if s.Then == nil {
			return fmt.Errorf("if then block is nil")
		}
		if err := validateExpr(s.Cond); err != nil {
			return fmt.Errorf("if condition: %w", err)
		}
		if err := validateBlock(s.Then); err != nil {
			return fmt.Errorf("if then: %w", err)
		}
		if s.Else != nil {
			if err := validateBlock(s.Else); err != nil {
				return fmt.Errorf("if else: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported statement type %T", stmt)
	}
}

func validateExpr(expr Expr) error {
	switch e := expr.(type) {
	case *IdentExpr:
		if e == nil || strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("identifier name is empty")
		}
		return nil
	case *LiteralExpr:
		if e == nil {
			return fmt.Errorf("nil literal expression")
		}
		if strings.TrimSpace(e.Kind) == "" {
			return fmt.Errorf("literal kind is empty")
		}
		return nil
	case *SelectorExpr:
		if e == nil {
			return fmt.Errorf("nil selector expression")
		}
		if e.Subject == nil {
			return fmt.Errorf("selector subject is nil")
		}
		if strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("selector name is empty")
		}
		return validateExpr(e.Subject)
	case *CallExpr:
		if e == nil {
			return fmt.Errorf("nil call expression")
		}
		if e.Callee == nil {
			return fmt.Errorf("call callee is nil")
		}
		if name, isMarker := markerFallbackArtifactName(e); isMarker {
			return fmt.Errorf("marker fallback artifact %q is not permitted in finalized backend IR", name)
		}
		if err := validateExpr(e.Callee); err != nil {
			return fmt.Errorf("call callee: %w", err)
		}
		for i, arg := range e.Args {
			if arg == nil {
				return fmt.Errorf("call arg[%d] is nil", i)
			}
			if err := validateExpr(arg); err != nil {
				return fmt.Errorf("call arg[%d]: %w", i, err)
			}
		}
		return nil
	case *ListLiteralExpr:
		if e == nil {
			return fmt.Errorf("nil list literal expression")
		}
		listType, ok := e.Type.(*ListType)
		if !ok || listType == nil {
			return fmt.Errorf("list literal type must be list type")
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("list literal type: %w", err)
		}
		for i, element := range e.Elements {
			if element == nil {
				return fmt.Errorf("list literal elem[%d] is nil", i)
			}
			if err := validateExpr(element); err != nil {
				return fmt.Errorf("list literal elem[%d]: %w", i, err)
			}
		}
		return nil
	case *MapLiteralExpr:
		if e == nil {
			return fmt.Errorf("nil map literal expression")
		}
		mapType, ok := e.Type.(*MapType)
		if !ok || mapType == nil {
			return fmt.Errorf("map literal type must be map type")
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("map literal type: %w", err)
		}
		for i, entry := range e.Entries {
			if entry.Key == nil {
				return fmt.Errorf("map literal entry[%d] key is nil", i)
			}
			if entry.Value == nil {
				return fmt.Errorf("map literal entry[%d] value is nil", i)
			}
			if err := validateExpr(entry.Key); err != nil {
				return fmt.Errorf("map literal entry[%d] key: %w", i, err)
			}
			if err := validateExpr(entry.Value); err != nil {
				return fmt.Errorf("map literal entry[%d] value: %w", i, err)
			}
		}
		return nil
	case *StructLiteralExpr:
		if e == nil {
			return fmt.Errorf("nil struct literal expression")
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("struct literal type: %w", err)
		}
		seen := make(map[string]struct{}, len(e.Fields))
		for i, field := range e.Fields {
			name := strings.TrimSpace(field.Name)
			if name == "" {
				return fmt.Errorf("struct literal field[%d] name is empty", i)
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("struct literal field[%d] duplicate name %q", i, name)
			}
			seen[name] = struct{}{}
			if field.Value == nil {
				return fmt.Errorf("struct literal field[%d] value is nil", i)
			}
			if err := validateExpr(field.Value); err != nil {
				return fmt.Errorf("struct literal field[%d] value: %w", i, err)
			}
		}
		return nil
	case *EnumVariantExpr:
		if e == nil {
			return fmt.Errorf("nil enum variant expression")
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("enum variant type: %w", err)
		}
		return nil
	case *IfExpr:
		if e == nil {
			return fmt.Errorf("nil if expression")
		}
		if e.Cond == nil {
			return fmt.Errorf("if expression condition is nil")
		}
		if e.Then == nil {
			return fmt.Errorf("if expression then block is nil")
		}
		if e.Type == nil {
			return fmt.Errorf("if expression type is nil")
		}
		if err := validateExpr(e.Cond); err != nil {
			return fmt.Errorf("if expression condition: %w", err)
		}
		if err := validateBlock(e.Then); err != nil {
			return fmt.Errorf("if expression then: %w", err)
		}
		if e.Else != nil {
			if err := validateBlock(e.Else); err != nil {
				return fmt.Errorf("if expression else: %w", err)
			}
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("if expression type: %w", err)
		}
		return nil
	case *UnionMatchExpr:
		if e == nil {
			return fmt.Errorf("nil union match expression")
		}
		if e.Subject == nil {
			return fmt.Errorf("union match subject is nil")
		}
		if len(e.Cases) == 0 {
			return fmt.Errorf("union match cases are empty")
		}
		if e.Type == nil {
			return fmt.Errorf("union match type is nil")
		}
		if err := validateExpr(e.Subject); err != nil {
			return fmt.Errorf("union match subject: %w", err)
		}
		for i, matchCase := range e.Cases {
			if err := validateType(matchCase.Type); err != nil {
				return fmt.Errorf("union match case[%d] type: %w", i, err)
			}
			if matchCase.Body == nil {
				return fmt.Errorf("union match case[%d] body is nil", i)
			}
			if err := validateBlock(matchCase.Body); err != nil {
				return fmt.Errorf("union match case[%d] body: %w", i, err)
			}
		}
		if e.CatchAll != nil {
			if err := validateBlock(e.CatchAll); err != nil {
				return fmt.Errorf("union match catch-all: %w", err)
			}
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("union match type: %w", err)
		}
		return nil
	case *TryExpr:
		if e == nil {
			return fmt.Errorf("nil try expression")
		}
		if strings.TrimSpace(e.Kind) != "result" && strings.TrimSpace(e.Kind) != "maybe" {
			return fmt.Errorf("try expression kind must be result or maybe")
		}
		if e.Subject == nil {
			return fmt.Errorf("try expression subject is nil")
		}
		if e.Type == nil {
			return fmt.Errorf("try expression type is nil")
		}
		if err := validateExpr(e.Subject); err != nil {
			return fmt.Errorf("try expression subject: %w", err)
		}
		if e.Catch != nil {
			if err := validateBlock(e.Catch); err != nil {
				return fmt.Errorf("try expression catch: %w", err)
			}
		} else if strings.TrimSpace(e.CatchVar) != "" {
			return fmt.Errorf("try expression catch var requires catch block")
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("try expression type: %w", err)
		}
		return nil
	case *PanicExpr:
		if e == nil {
			return fmt.Errorf("nil panic expression")
		}
		if e.Message == nil {
			return fmt.Errorf("panic expression message is nil")
		}
		if e.Type == nil {
			return fmt.Errorf("panic expression type is nil")
		}
		if err := validateExpr(e.Message); err != nil {
			return fmt.Errorf("panic expression message: %w", err)
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("panic expression type: %w", err)
		}
		return nil
	case *CopyExpr:
		if e == nil {
			return fmt.Errorf("nil copy expression")
		}
		if e.Value == nil {
			return fmt.Errorf("copy expression value is nil")
		}
		listType, ok := e.Type.(*ListType)
		if !ok || listType == nil {
			return fmt.Errorf("copy expression type must be list type")
		}
		if err := validateExpr(e.Value); err != nil {
			return fmt.Errorf("copy expression value: %w", err)
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("copy expression type: %w", err)
		}
		return nil
	case *BlockExpr:
		if e == nil {
			return fmt.Errorf("nil block expression")
		}
		if e.Value == nil {
			return fmt.Errorf("block expression value is nil")
		}
		if e.Type == nil {
			return fmt.Errorf("block expression type is nil")
		}
		for i, stmt := range e.Setup {
			if stmt == nil {
				return fmt.Errorf("block expression setup[%d] is nil", i)
			}
			if err := validateStmt(stmt); err != nil {
				return fmt.Errorf("block expression setup[%d]: %w", i, err)
			}
		}
		if err := validateExpr(e.Value); err != nil {
			return fmt.Errorf("block expression value: %w", err)
		}
		if err := validateType(e.Type); err != nil {
			return fmt.Errorf("block expression type: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported expression type %T", expr)
	}
}

func validateType(t Type) error {
	switch typed := t.(type) {
	case nil:
		return fmt.Errorf("type is nil")
	case *PrimitiveType:
		if typed == nil || strings.TrimSpace(typed.Name) == "" {
			return fmt.Errorf("primitive type name is empty")
		}
		return nil
	case *DynamicType, *VoidType:
		return nil
	case *TypeVarType:
		if typed == nil || strings.TrimSpace(typed.Name) == "" {
			return fmt.Errorf("type var name is empty")
		}
		return nil
	case *NamedType:
		if typed == nil || strings.TrimSpace(typed.Name) == "" {
			return fmt.Errorf("named type name is empty")
		}
		for i, arg := range typed.Args {
			if err := validateType(arg); err != nil {
				return fmt.Errorf("named type arg[%d]: %w", i, err)
			}
		}
		return nil
	case *ListType:
		if typed == nil {
			return fmt.Errorf("nil list type")
		}
		if err := validateType(typed.Elem); err != nil {
			return fmt.Errorf("list elem: %w", err)
		}
		return nil
	case *MapType:
		if typed == nil {
			return fmt.Errorf("nil map type")
		}
		if err := validateType(typed.Key); err != nil {
			return fmt.Errorf("map key: %w", err)
		}
		if err := validateType(typed.Value); err != nil {
			return fmt.Errorf("map value: %w", err)
		}
		return nil
	case *MaybeType:
		if typed == nil {
			return fmt.Errorf("nil maybe type")
		}
		if err := validateType(typed.Of); err != nil {
			return fmt.Errorf("maybe of: %w", err)
		}
		return nil
	case *ResultType:
		if typed == nil {
			return fmt.Errorf("nil result type")
		}
		if err := validateType(typed.Val); err != nil {
			return fmt.Errorf("result val: %w", err)
		}
		if err := validateType(typed.Err); err != nil {
			return fmt.Errorf("result err: %w", err)
		}
		return nil
	case *FuncType:
		if typed == nil {
			return fmt.Errorf("nil function type")
		}
		for i, param := range typed.Params {
			if err := validateType(param); err != nil {
				return fmt.Errorf("func param[%d]: %w", i, err)
			}
		}
		if err := validateType(typed.Return); err != nil {
			return fmt.Errorf("func return: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported type %T", t)
	}
}
