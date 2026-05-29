package lsp

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/std_lib"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type definitionTarget struct {
	filePath string
	loc      parse.Location
}

func computeDefinition(source string, filePath string, position protocol.Position) []protocol.Location {
	target := lspPositionToParsePoint(position)
	prog := parseAndCache(source, filePath)
	if prog == nil {
		return nil
	}

	expr := findInStmts(prog.Statements, target)
	if expr == nil {
		return nil
	}

	def := definitionForExpr(expr, prog, filePath)
	if def == nil {
		return nil
	}
	return []protocol.Location{definitionLocation(def.filePath, def.loc)}
}

func definitionLocation(filePath string, loc parse.Location) protocol.Location {
	return protocol.Location{
		URI:   uri.File(filePath),
		Range: checkerLocationToLSPRange(loc),
	}
}

func definitionForExpr(expr parse.Expression, prog *parse.Program, filePath string) *definitionTarget {
	switch e := expr.(type) {
	case *parse.Identifier:
		return definitionForIdentifier(e.Name, prog, filePath)
	case *parse.Parameter:
		return &definitionTarget{filePath: filePath, loc: e.Location}
	case *parse.VariableDeclaration:
		return &definitionTarget{filePath: filePath, loc: e.Location}
	case *parse.FunctionDeclaration:
		return &definitionTarget{filePath: filePath, loc: e.Location}
	case *parse.FunctionCall:
		return definitionForLocalFunction(e.Name, prog, filePath)
	case *parse.StaticFunction:
		return definitionForStaticFunction(e, prog, filePath)
	case *parse.StaticProperty:
		return definitionForStaticProperty(e, prog, filePath)
	case *parse.InstanceProperty:
		return definitionForInstanceProperty(e, prog, filePath)
	case *parse.InstanceMethod:
		return definitionForInstanceMethod(e, prog, filePath)
	case *parse.StructInstance:
		return definitionForTypeName(e.Name.Name, prog, filePath)
	}
	return nil
}

func definitionForIdentifier(name string, prog *parse.Program, filePath string) *definitionTarget {
	if def := findIdentifierDefinition(name, prog.Statements, filePath); def != nil {
		return def
	}
	if def := definitionForModuleAlias(name, prog, filePath); def != nil {
		return def
	}
	return nil
}

func definitionForModuleAlias(alias string, prog *parse.Program, filePath string) *definitionTarget {
	modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath)
	if !ok || moduleProg == nil {
		return nil
	}
	return &definitionTarget{filePath: modulePath, loc: moduleStartLocation(moduleProg)}
}

func definitionForLocalFunction(name string, prog *parse.Program, filePath string) *definitionTarget {
	return findFunctionDefinition(name, prog.Statements, filePath)
}

func definitionForTypeName(name string, prog *parse.Program, filePath string) *definitionTarget {
	if alias, memberName, ok := importedTypeDisplayParts(name); ok {
		return definitionForImportedType(alias, memberName, prog, filePath)
	}
	return findTypeDefinition(name, prog.Statements, filePath)
}

func definitionForStaticFunction(sf *parse.StaticFunction, prog *parse.Program, filePath string) *definitionTarget {
	target := simpleExprName(sf.Target)
	if target == "" {
		return nil
	}
	alias, memberPrefix := splitStaticTarget(target)
	if modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath); ok {
		lookupName := sf.Function.Name
		if memberPrefix != "" {
			lookupName = memberPrefix + "::" + sf.Function.Name
		}
		if def := findStaticFunctionDefinition(lookupName, moduleProg.Statements, modulePath); def != nil {
			return def
		}
		if def := findFunctionDefinition(sf.Function.Name, moduleProg.Statements, modulePath); def != nil && memberPrefix == "" {
			return def
		}
		return nil
	}
	return findStaticFunctionDefinition(target+"::"+sf.Function.Name, prog.Statements, filePath)
}

func definitionForStaticProperty(sp *parse.StaticProperty, prog *parse.Program, filePath string) *definitionTarget {
	target := simpleExprName(sp.Target)
	property := simpleExprName(sp.Property)
	if target == "" || property == "" {
		return nil
	}

	alias, memberPrefix := splitStaticTarget(target)
	if modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath); ok {
		lookupName := property
		if memberPrefix != "" {
			lookupName = memberPrefix + "::" + property
		}
		if def := findVariableDefinition(lookupName, moduleProg.Statements, modulePath); def != nil {
			return def
		}
		if def := findTypeDefinition(lookupName, moduleProg.Statements, modulePath); def != nil {
			return def
		}
		if memberPrefix != "" {
			if def := findEnumVariantDefinition(memberPrefix, property, moduleProg.Statements, modulePath); def != nil {
				return def
			}
		}
		return nil
	}

	if def := findVariableDefinition(property, prog.Statements, filePath); def != nil {
		return def
	}
	return nil
}

func definitionForInstanceProperty(ip *parse.InstanceProperty, prog *parse.Program, filePath string) *definitionTarget {
	ownerType := normalizeDisplayType(inferExprType(ip.Target))
	ownerType = strings.TrimSuffix(ownerType, "?")
	if ownerType == "" || ownerType == "?" {
		return nil
	}
	if alias, memberName, ok := importedTypeDisplayParts(ownerType); ok {
		modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath)
		if !ok {
			return nil
		}
		return findStructFieldDefinition(memberName, ip.Property.Name, moduleProg.Statements, modulePath)
	}
	return findStructFieldDefinition(ownerType, ip.Property.Name, prog.Statements, filePath)
}

func definitionForInstanceMethod(im *parse.InstanceMethod, prog *parse.Program, filePath string) *definitionTarget {
	ownerType := normalizeDisplayType(inferExprType(im.Target))
	ownerType = strings.TrimSuffix(ownerType, "?")
	if ownerType == "" || ownerType == "?" {
		return nil
	}
	if alias, memberName, ok := importedTypeDisplayParts(ownerType); ok {
		modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath)
		if !ok {
			return nil
		}
		return findMethodDefinition(memberName, im.Method.Name, moduleProg.Statements, modulePath)
	}
	return findMethodDefinition(ownerType, im.Method.Name, prog.Statements, filePath)
}

func moduleSourceForAlias(alias string, prog *parse.Program, filePath string) (string, *parse.Program, bool) {
	if prog == nil {
		return "", nil, false
	}
	for _, imp := range prog.Imports {
		if imp.Name != alias {
			continue
		}
		return moduleSourceForImport(imp, filePath)
	}
	if path := preludeModulePath(alias); path != "" {
		moduleFile := stdLibSourcePath(path)
		content, err := std_lib.Find(path)
		if err != nil {
			return "", nil, false
		}
		result := parse.Parse(content, moduleFile)
		if result.Program == nil {
			return "", nil, false
		}
		return moduleFile, result.Program, true
	}
	return "", nil, false
}

func moduleSourceForImport(imp parse.Import, filePath string) (string, *parse.Program, bool) {
	if strings.HasPrefix(imp.Path, "ard/") {
		moduleFile := stdLibSourcePath(imp.Path)
		content, err := std_lib.Find(imp.Path)
		if err != nil {
			return "", nil, false
		}
		result := parse.Parse(content, moduleFile)
		if result.Program == nil {
			return "", nil, false
		}
		return moduleFile, result.Program, true
	}

	if filePath == "" {
		return "", nil, false
	}
	moduleResolver, err := checker.NewModuleResolver(filepath.Dir(filePath))
	if err != nil {
		return "", nil, false
	}
	moduleFile, err := moduleResolver.ResolveImportPath(imp.Path)
	if err != nil {
		return "", nil, false
	}
	moduleProg, err := moduleResolver.LoadModule(imp.Path)
	if err != nil || moduleProg == nil {
		return "", nil, false
	}
	return moduleFile, moduleProg, true
}

func stdLibSourcePath(importPath string) string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return importPath
	}
	moduleName := strings.TrimPrefix(importPath, "ard/") + ".ard"
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "std_lib", moduleName))
}

func moduleStartLocation(prog *parse.Program) parse.Location {
	loc := parse.Location{Start: parse.Point{Row: 1, Col: 1}, End: parse.Point{Row: 1, Col: 1}}
	if prog == nil {
		return loc
	}
	if len(prog.Statements) > 0 {
		return prog.Statements[0].GetLocation()
	}
	if len(prog.Imports) > 0 {
		return prog.Imports[0].Location
	}
	return loc
}

func definitionForImportedType(alias string, memberName string, prog *parse.Program, filePath string) *definitionTarget {
	modulePath, moduleProg, ok := moduleSourceForAlias(alias, prog, filePath)
	if !ok {
		return nil
	}
	return findTypeDefinition(memberName, moduleProg.Statements, modulePath)
}

func findIdentifierDefinition(name string, stmts []parse.Statement, filePath string) *definitionTarget {
	if def := findVariableDefinition(name, stmts, filePath); def != nil {
		return def
	}
	if def := findFunctionDefinition(name, stmts, filePath); def != nil {
		return def
	}
	if def := findTypeDefinition(name, stmts, filePath); def != nil {
		return def
	}
	return findParameterOrLoopDefinition(name, stmts, filePath)
}

func findVariableDefinition(name string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.VariableDeclaration:
			if s.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Location}
			}
		case *parse.FunctionDeclaration:
			if def := findVariableDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.ImplBlock:
			for i := range s.Methods {
				if def := findVariableDefinition(name, s.Methods[i].Body, filePath); def != nil {
					return def
				}
			}
		case *parse.TraitImplementation:
			for i := range s.Methods {
				if def := findVariableDefinition(name, s.Methods[i].Body, filePath); def != nil {
					return def
				}
			}
		case *parse.IfStatement:
			if def := findVariableDefinition(name, s.Body, filePath); def != nil {
				return def
			}
			if s.Else != nil {
				if def := findVariableDefinition(name, []parse.Statement{s.Else}, filePath); def != nil {
					return def
				}
			}
		case *parse.WhileLoop:
			if def := findVariableDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.ForInLoop:
			if def := findVariableDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.RangeLoop:
			if def := findVariableDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.ForLoop:
			if s.Init != nil && s.Init.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Init.Location}
			}
			if def := findVariableDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.BlockExpression:
			if def := findVariableDefinition(name, s.Statements, filePath); def != nil {
				return def
			}
		}
	}
	return nil
}

func findFunctionDefinition(name string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.FunctionDeclaration:
			if s.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Location}
			}
		case *parse.ExternalFunction:
			if s.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Location}
			}
		case *parse.StaticFunctionDeclaration:
			if s.Name == name || simpleExprName(&s.Path) == name {
				return &definitionTarget{filePath: filePath, loc: s.Location}
			}
		case *parse.ImplBlock:
			for i := range s.Methods {
				if s.Methods[i].Name == name {
					return &definitionTarget{filePath: filePath, loc: s.Methods[i].Location}
				}
			}
		}
	}
	return nil
}

func findStaticFunctionDefinition(path string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		if s, ok := stmt.(*parse.StaticFunctionDeclaration); ok && simpleExprName(&s.Path) == path {
			return &definitionTarget{filePath: filePath, loc: s.Location}
		}
	}
	return nil
}

func findTypeDefinition(name string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.StructDefinition:
			if s.Name.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Name.Location}
			}
		case *parse.EnumDefinition:
			if s.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Location}
			}
		case *parse.TraitDefinition:
			if s.Name.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Name.Location}
			}
		case *parse.TypeDeclaration:
			if s.Name.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Name.Location}
			}
		case *parse.ExternTypeDeclaration:
			if s.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Location}
			}
		}
	}
	return nil
}

func findParameterOrLoopDefinition(name string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		if stmt == nil {
			continue
		}
		switch s := stmt.(type) {
		case *parse.FunctionDeclaration:
			for _, p := range s.Parameters {
				if p.Name == name {
					return &definitionTarget{filePath: filePath, loc: p.Location}
				}
			}
			if def := findParameterOrLoopDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.ImplBlock:
			if s.Receiver.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Receiver.Location}
			}
			for i := range s.Methods {
				if def := findParameterOrLoopDefinition(name, []parse.Statement{&s.Methods[i]}, filePath); def != nil {
					return def
				}
			}
		case *parse.TraitImplementation:
			if s.Receiver.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Receiver.Location}
			}
			for i := range s.Methods {
				if def := findParameterOrLoopDefinition(name, []parse.Statement{&s.Methods[i]}, filePath); def != nil {
					return def
				}
			}
		case *parse.ForInLoop:
			if s.Cursor.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Cursor.Location}
			}
			if s.Cursor2.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Cursor2.Location}
			}
			if def := findParameterOrLoopDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.RangeLoop:
			if s.Cursor.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Cursor.Location}
			}
			if s.Cursor2.Name == name {
				return &definitionTarget{filePath: filePath, loc: s.Cursor2.Location}
			}
			if def := findParameterOrLoopDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.IfStatement:
			if def := findParameterOrLoopDefinition(name, s.Body, filePath); def != nil {
				return def
			}
			if s.Else != nil {
				if def := findParameterOrLoopDefinition(name, []parse.Statement{s.Else}, filePath); def != nil {
					return def
				}
			}
		case *parse.WhileLoop:
			if def := findParameterOrLoopDefinition(name, s.Body, filePath); def != nil {
				return def
			}
		case *parse.BlockExpression:
			if def := findParameterOrLoopDefinition(name, s.Statements, filePath); def != nil {
				return def
			}
		}
	}
	return nil
}

func findStructFieldDefinition(structName string, fieldName string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		s, ok := stmt.(*parse.StructDefinition)
		if !ok || s.Name.Name != structName {
			continue
		}
		for _, field := range s.Fields {
			if field.Name.Name == fieldName {
				return &definitionTarget{filePath: filePath, loc: field.Name.Location}
			}
		}
	}
	return nil
}

func findMethodDefinition(ownerName string, methodName string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *parse.ImplBlock:
			if s.Target.Name != ownerName {
				continue
			}
			for i := range s.Methods {
				if s.Methods[i].Name == methodName {
					return &definitionTarget{filePath: filePath, loc: s.Methods[i].Location}
				}
			}
		case *parse.TraitImplementation:
			if s.ForType.Name != ownerName {
				continue
			}
			for i := range s.Methods {
				if s.Methods[i].Name == methodName {
					return &definitionTarget{filePath: filePath, loc: s.Methods[i].Location}
				}
			}
		case *parse.TraitDefinition:
			if s.Name.Name != ownerName {
				continue
			}
			for i := range s.Methods {
				if s.Methods[i].Name == methodName {
					return &definitionTarget{filePath: filePath, loc: s.Methods[i].Location}
				}
			}
		}
	}
	return nil
}

func findEnumVariantDefinition(enumName string, variantName string, stmts []parse.Statement, filePath string) *definitionTarget {
	for _, stmt := range stmts {
		e, ok := stmt.(*parse.EnumDefinition)
		if !ok || e.Name != enumName {
			continue
		}
		for _, variant := range e.Variants {
			if variant.Name == variantName {
				return &definitionTarget{filePath: filePath, loc: e.Location}
			}
		}
	}
	return nil
}
