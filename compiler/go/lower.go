package gotarget

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
)

type loweredExpr struct {
	stmts []ast.Stmt
	expr  ast.Expr
}

type lowerer struct {
	program                 *air.Program
	packageName             string
	tempCounter             int
	currentImports          map[string]string
	currentModule           air.ModuleID
	importErr               error
	reservedGoIdentifiers   map[string]bool
	topLevelReserved        map[string]bool
	localNameCache          map[air.FunctionID]map[air.LocalID]string
	declaredLocals          map[air.LocalID]bool
	runtimeHelpers          map[string]bool
	projectInfo             *checker.ProjectInfo
	inlineClosures          map[air.FunctionID]bool
	goMethodCollisions      map[string]bool
	emittedGoMethods        map[string]bool
	functionModules         map[air.FunctionID]air.ModuleID
	mutableTraitRefs        map[air.TraitID]bool
	emittedMutableTraitRefs map[air.TraitID]bool
	suppressMain            bool
	includeTests            bool
	useModulePackages       bool

	// When the entry root lives in a module named `main` (main.ard) that no
	// other module imports, that module is emitted as the root `package main`
	// with the root lowered to `func main()`, instead of an importable package
	// plus a separate synthetic main (ADR 0031).
	entryAsMainPackage  bool
	entryMainModuleID   air.ModuleID
	entryMainFunctionID air.FunctionID
}

func lowerProgram(program *air.Program, options Options) (map[string]*ast.File, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	l := &lowerer{program: program, packageName: defaultPackageName(options.PackageName), runtimeHelpers: map[string]bool{}, projectInfo: options.ProjectInfo, suppressMain: options.SuppressMain, includeTests: options.IncludeTests, useModulePackages: true}
	l.reservedGoIdentifiers = l.buildReservedGoIdentifiers()
	l.inlineClosures = l.collectInlineClosureFunctions()
	l.goMethodCollisions = l.collectGoMethodCollisions()
	l.emittedGoMethods = map[string]bool{}
	l.functionModules = l.collectFunctionEmitModules()
	files := map[string]*ast.File{}
	rootID, hasRoot := findRootFunction(program)
	mainModuleID := l.mainModuleID(rootID, hasRoot)
	// A `main.ard` entry that nothing imports becomes the root `package main`
	// directly, rather than an importable package plus a synthetic main.
	l.entryMainFunctionID = air.NoFunction
	if !l.suppressMain && hasRoot {
		rootModuleID := program.Functions[rootID].Module
		if strings.TrimSuffix(filepath.Base(program.Modules[rootModuleID].Path), filepath.Ext(program.Modules[rootModuleID].Path)) == "main" &&
			l.isVoidType(program.Functions[rootID].Signature.Return) &&
			!moduleIsImported(program, rootModuleID) {
			l.entryAsMainPackage = true
			l.entryMainModuleID = rootModuleID
			l.entryMainFunctionID = rootID
		}
	}
	modules := make([]air.Module, 0, len(program.Modules))
	if hasRoot {
		rootModuleID := program.Functions[rootID].Module
		for _, module := range program.Modules {
			if module.ID != rootModuleID {
				modules = append(modules, module)
			}
		}
		for _, module := range program.Modules {
			if module.ID == rootModuleID {
				modules = append(modules, module)
				break
			}
		}
	} else {
		modules = append(modules, program.Modules...)
	}
	for _, module := range modules {
		file, err := l.lowerModule(module)
		if err != nil {
			return nil, err
		}
		files[l.moduleOutputFileName(module, mainModuleID)] = file
	}
	if !l.suppressMain && !l.entryAsMainPackage {
		if hasRoot {
			mainFile, err := l.synthesizeEntryMain(rootID, mainModuleID)
			if err != nil {
				return nil, err
			}
			files["main.go"] = mainFile
		} else {
			// A program with no entry or script root still emits an empty main so
			// the workspace builds and runs as a no-op.
			files["main.go"] = &ast.File{Name: ast.NewIdent("main"), Decls: []ast.Decl{
				&ast.FuncDecl{Name: ast.NewIdent("main"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{}},
			}}
		}
	}
	return files, nil
}

// synthesizeEntryMain builds the synthetic `package main` that imports the entry
// module's package and calls its entry function. The entry module itself lowers
// as an ordinary package; `main` is never a transpiled Ard module (ADR 0031).
func (l *lowerer) synthesizeEntryMain(rootID air.FunctionID, entryModuleID air.ModuleID) (*ast.File, error) {
	fn := l.program.Functions[rootID]
	if len(fn.Signature.Params) != 0 {
		return nil, fmt.Errorf("entry function parameters are not supported yet")
	}
	alias := modulePackageName(l.program, entryModuleID)
	importPath := moduleImportPath(l.program, entryModuleID)
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(alias), Sel: ast.NewIdent(functionName(l.program, fn))}}
	var stmt ast.Stmt
	if l.isVoidType(fn.Signature.Return) {
		stmt = &ast.ExprStmt{X: call}
	} else {
		stmt = &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}}
	}
	importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{&ast.ImportSpec{
		Name: ast.NewIdent(alias),
		Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importPath)},
	}}}
	mainDecl := &ast.FuncDecl{Name: ast.NewIdent("main"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}}
	return &ast.File{Name: ast.NewIdent("main"), Decls: []ast.Decl{importDecl, mainDecl}}, nil
}

func (l *lowerer) lowerModule(module air.Module) (*ast.File, error) {
	previousModule := l.currentModule
	l.currentModule = module.ID
	defer func() { l.currentModule = previousModule }()
	l.currentImports = map[string]string{}
	l.importErr = nil
	l.runtimeHelpers = map[string]bool{}
	l.mutableTraitRefs = map[air.TraitID]bool{}
	l.emittedMutableTraitRefs = map[air.TraitID]bool{}
	decls := []ast.Decl{}
	rootID, hasRoot := findRootFunction(l.program)
	mainModuleID := l.mainModuleID(rootID, hasRoot)
	for _, typ := range l.typesForModule(module.ID, mainModuleID) {
		typeDecls, err := l.lowerTypeDecls(typ)
		if err != nil {
			return nil, fmt.Errorf("module %s type %s: %w", module.Path, typ.Name, err)
		}
		decls = append(decls, typeDecls...)
	}
	globalIDs := append([]air.GlobalID(nil), module.Globals...)
	sort.Slice(globalIDs, func(i, j int) bool { return globalIDs[i] < globalIDs[j] })
	for _, globalID := range globalIDs {
		global := l.program.Globals[globalID]
		decl, err := l.lowerGlobal(global)
		if err != nil {
			return nil, fmt.Errorf("module %s global %s: %w", module.Path, global.Name, err)
		}
		decls = append(decls, decl)
	}
	functionIDs := l.functionsForModule(module.ID)
	sort.Slice(functionIDs, func(i, j int) bool { return functionIDs[i] < functionIDs[j] })
	for _, functionID := range functionIDs {
		fn := l.program.Functions[functionID]
		if l.inlineClosures[functionID] {
			continue
		}
		if fn.IsTest && !l.includeTests {
			continue
		}
		decl, err := l.lowerFunction(fn)
		if err != nil {
			return nil, fmt.Errorf("module %s function %s: %w", module.Path, fn.Name, err)
		}
		decls = append(decls, decl)
		methodDecl, ok, err := l.lowerGoMethodWrapper(fn)
		if err != nil {
			return nil, fmt.Errorf("module %s function %s Go method wrapper: %w", module.Path, fn.Name, err)
		}
		if ok {
			decls = append(decls, methodDecl)
		}
	}
	mutableDecls, err := l.markedMutableTraitRefDecls()
	if err != nil {
		return nil, err
	}
	decls = append(mutableDecls, decls...)
	decls = append(l.runtimePreludeDecls(), decls...)
	if l.importErr != nil {
		return nil, l.importErr
	}
	if len(l.currentImports) > 0 {
		usedImports := l.usedImports(decls)
		if len(usedImports) > 0 {
			importDecl := &ast.GenDecl{Tok: token.IMPORT}
			aliases := make([]string, 0, len(usedImports))
			for alias := range usedImports {
				aliases = append(aliases, alias)
			}
			sort.Strings(aliases)
			for _, alias := range aliases {
				importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
					Name: ast.NewIdent(alias),
					Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", usedImports[alias])},
				})
			}
			decls = append([]ast.Decl{importDecl}, decls...)
		}
	}
	return &ast.File{Name: ast.NewIdent(l.modulePackageName(module.ID, mainModuleID)), Decls: decls}, nil
}

func (l *lowerer) collectFunctionEmitModules() map[air.FunctionID]air.ModuleID {
	modules := map[air.FunctionID]air.ModuleID{}
	for _, fn := range l.program.Functions {
		modules[fn.ID] = fn.Module
	}
	for _, fn := range l.program.Functions {
		owners := map[air.ModuleID]bool{}
		for _, param := range fn.Signature.Params {
			l.collectExternalTypeOwnerModules(param.Type, fn.Module, owners)
		}
		l.collectExternalTypeOwnerModules(fn.Signature.Return, fn.Module, owners)
		for _, capture := range fn.Captures {
			l.collectExternalTypeOwnerModules(capture.Type, fn.Module, owners)
		}
		for _, local := range fn.Locals {
			l.collectExternalTypeOwnerModules(local.Type, fn.Module, owners)
		}
		candidateOwners := make([]air.ModuleID, 0, len(owners))
		for owner := range owners {
			if l.moduleImports(owner, fn.Module, map[air.ModuleID]bool{}) {
				candidateOwners = append(candidateOwners, owner)
			}
		}
		if len(candidateOwners) == 1 {
			modules[fn.ID] = candidateOwners[0]
		}
	}
	changed := true
	for changed {
		changed = false
		for _, fn := range l.program.Functions {
			emitModule := modules[fn.ID]
			if emitModule == fn.Module {
				continue
			}
			for _, ref := range functionRefsInBlock(fn.Body) {
				if !validFunctionID(l.program, ref) {
					continue
				}
				target := l.program.Functions[ref]
				if target.Module != fn.Module || modules[target.ID] == emitModule {
					continue
				}
				modules[target.ID] = emitModule
				changed = true
			}
		}
	}
	return modules
}

func (l *lowerer) moduleImports(moduleID air.ModuleID, target air.ModuleID, seen map[air.ModuleID]bool) bool {
	if moduleID == target {
		return true
	}
	if seen[moduleID] || moduleID < 0 || int(moduleID) >= len(l.program.Modules) {
		return false
	}
	seen[moduleID] = true
	for _, imported := range l.program.Modules[moduleID].Imports {
		if imported == target || l.moduleImports(imported, target, seen) {
			return true
		}
	}
	return false
}

func (l *lowerer) collectExternalTypeOwnerModules(typeID air.TypeID, self air.ModuleID, out map[air.ModuleID]bool) {
	if !validTypeID(l.program, typeID) {
		return
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeList, air.TypeMaybe:
		l.collectExternalTypeOwnerModules(info.Elem, self, out)
	case air.TypeMap:
		l.collectExternalTypeOwnerModules(info.Key, self, out)
		l.collectExternalTypeOwnerModules(info.Value, self, out)
	case air.TypeResult:
		l.collectExternalTypeOwnerModules(info.Value, self, out)
		l.collectExternalTypeOwnerModules(info.Error, self, out)
	case air.TypeFunction:
		for _, param := range info.Params {
			l.collectExternalTypeOwnerModules(param, self, out)
		}
		l.collectExternalTypeOwnerModules(info.Return, self, out)
	case air.TypeStruct, air.TypeEnum, air.TypeUnion:
		if owner, ok := l.ownerModuleForType(typeID); ok && owner != self {
			out[owner] = true
		}
	case air.TypeTraitObject:
		if owner, ok := l.ownerModuleForTrait(info.Trait); ok && owner != self {
			out[owner] = true
		}
	}
}

func functionRefsInBlock(block air.Block) []air.FunctionID {
	refs := []air.FunctionID{}
	walkBlockExprs(block, func(expr air.Expr) {
		switch expr.Kind {
		case air.ExprCall, air.ExprFunctionRef, air.ExprMakeClosure:
			refs = append(refs, expr.Function)
		}
	})
	return refs
}

func (l *lowerer) functionsForModule(moduleID air.ModuleID) []air.FunctionID {
	out := []air.FunctionID{}
	for _, fn := range l.program.Functions {
		if l.functionModule(fn) == moduleID {
			out = append(out, fn.ID)
		}
	}
	return out
}

func (l *lowerer) functionModule(fn air.Function) air.ModuleID {
	if l.functionModules != nil {
		if module, ok := l.functionModules[fn.ID]; ok {
			return module
		}
	}
	return fn.Module
}

func (l *lowerer) mainModuleID(rootID air.FunctionID, hasRoot bool) air.ModuleID {
	if hasRoot {
		return l.program.Functions[rootID].Module
	}
	if len(l.program.Modules) > 0 {
		return l.program.Modules[len(l.program.Modules)-1].ID
	}
	return air.ModuleID(-1)
}

func (l *lowerer) modulePackageName(moduleID air.ModuleID, mainModuleID air.ModuleID) string {
	// A `main.ard` entry that nothing imports is emitted as the root `package
	// main` directly; otherwise `package main` is synthetic and never a
	// transpiled module (ADR 0031).
	if l.entryAsMainPackage && moduleID == l.entryMainModuleID {
		return "main"
	}
	return modulePackageName(l.program, moduleID)
}

func (l *lowerer) moduleOutputFileName(module air.Module, mainModuleID air.ModuleID) string {
	if l.entryAsMainPackage && module.ID == l.entryMainModuleID {
		return "main.go"
	}
	return filepath.Join(modulePackageDir(l.program, module.ID), modulePackageFileName(l.program, module.ID))
}

// moduleIsImported reports whether any other module imports the target module.
// A `package main` cannot be imported, so the entry module only collapses into
// the root main package when nothing imports it.
func moduleIsImported(program *air.Program, target air.ModuleID) bool {
	for _, m := range program.Modules {
		if m.ID == target {
			continue
		}
		for _, imp := range m.Imports {
			if imp == target {
				return true
			}
		}
	}
	return false
}

// goFunctionName is the emitted Go identifier for a function. The entry root of
// a collapsed `main.ard` package is `main`; everything else uses the normal
// naming rules.
func (l *lowerer) goFunctionName(fn air.Function) string {
	if l.entryAsMainPackage && fn.ID == l.entryMainFunctionID {
		return "main"
	}
	return functionName(l.program, fn)
}

func modulePackageFileName(program *air.Program, module air.ModuleID) string {
	name := modulePackageName(program, module)
	if strings.HasSuffix(name, "_test") {
		name += "_ard"
	}
	return name + ".go"
}

func (l *lowerer) typesForModule(moduleID air.ModuleID, mainModuleID air.ModuleID) []air.TypeInfo {
	declaredInAnyModule := map[air.TypeID]bool{}
	for _, module := range l.program.Modules {
		for _, typeID := range module.Types {
			declaredInAnyModule[typeID] = true
		}
	}
	out := []air.TypeInfo{}
	if int(moduleID) >= 0 && int(moduleID) < len(l.program.Modules) {
		for _, typeID := range l.program.Modules[moduleID].Types {
			if validTypeID(l.program, typeID) {
				out = append(out, l.program.Types[typeID-1])
			}
		}
	}
	for _, typ := range l.program.Types {
		if declaredInAnyModule[typ.ID] {
			continue
		}
		if typ.Kind == air.TypeTraitObject {
			if owner, ok := l.ownerModuleForTrait(typ.Trait); ok {
				if owner == moduleID {
					out = append(out, typ)
				}
				continue
			}
		}
		if owner, ok := l.ownerModuleForType(typ.ID); ok {
			if owner == moduleID {
				out = append(out, typ)
			}
			continue
		}
		if moduleID == mainModuleID {
			out = append(out, typ)
		}
	}
	return out
}

func (l *lowerer) ownerModuleForTrait(traitID air.TraitID) (air.ModuleID, bool) {
	if !validTraitID(l.program, traitID) {
		return 0, false
	}
	return l.moduleForPath(l.program.Traits[traitID].ModulePath)
}

func (l *lowerer) ownerModuleForType(typeID air.TypeID) (air.ModuleID, bool) {
	if !validTypeID(l.program, typeID) {
		return 0, false
	}
	return l.moduleForPath(l.modulePathForType(typeID))
}

func (l *lowerer) moduleForPath(modulePath string) (air.ModuleID, bool) {
	if modulePath == "" {
		return 0, false
	}
	for _, module := range l.program.Modules {
		if module.Path == modulePath {
			return module.ID, true
		}
	}
	return 0, false
}

func (l *lowerer) usedImports(decls []ast.Decl) map[string]string {
	used := map[string]string{}
	for _, decl := range decls {
		ast.Inspect(decl, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			path, ok := l.currentImports[ident.Name]
			if !ok {
				return true
			}
			used[ident.Name] = path
			return true
		})
	}
	return used
}

func (l *lowerer) markRuntimeHelper(name string) {
	l.runtimeHelpers[name] = true
}

func (l *lowerer) registerImportsForGoType(expr ast.Expr, imports map[string]string) {
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if path, ok := imports[ident.Name]; ok && path != "" {
			ident.Name = l.registerImport(ident.Name, path)
		}
		return true
	})
}

func isPredeclaredGoTypeName(name string) bool {
	switch name {
	case "any", "bool", "byte", "comparable", "complex64", "complex128", "error", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}

func (l *lowerer) runtimePreludeDecls() []ast.Decl {
	parts := []string{"package main\n"}
	if l.runtimeHelpers["sorted_int_keys"] {
		slicesAlias := l.registerImport("slices", "slices")
		parts = append(parts, fmt.Sprintf(`
	func ardSortedIntKeys[V any](m map[int]V) []int {
		keys := make([]int, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		%s.Sort(keys)
		return keys
	}
`, slicesAlias))
	}
	if l.runtimeHelpers["sorted_string_keys"] {
		slicesAlias := l.registerImport("slices", "slices")
		parts = append(parts, fmt.Sprintf(`
	func ardSortedStringKeys[V any](m map[string]V) []string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		%s.Sort(keys)
		return keys
	}
`, slicesAlias))
	}
	if l.runtimeHelpers["sorted_any_keys"] {
		fmtAlias := l.registerImport("fmt", "fmt")
		slicesAlias := l.registerImport("slices", "slices")
		parts = append(parts, fmt.Sprintf(`
	func ardSortedAnyKeys[V any](m map[any]V) []any {
		keys := make([]any, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		%s.SortFunc(keys, func(a any, b any) int {
			as := %s.Sprint(a)
			bs := %s.Sprint(b)
			if as < bs {
				return -1
			}
			if as > bs {
				return 1
			}
			return 0
		})
		return keys
	}
`, slicesAlias, fmtAlias, fmtAlias))
	}
	if l.runtimeHelpers["list_to_any_slice"] {
		parts = append(parts, `
	func ardListToAnySlice[T any](values []T) []any {
		out := make([]any, len(values))
		for i, value := range values {
			out[i] = value
		}
		return out
	}
`)
	}
	src := strings.Join(parts, "\n")
	file, err := parser.ParseFile(token.NewFileSet(), "prelude.go", src, 0)
	if err != nil {
		panic(err)
	}
	return file.Decls
}

func (l *lowerer) lowerTypeDecls(typ air.TypeInfo) ([]ast.Decl, error) {
	// A concrete instantiation of a generic type is emitted at use sites as
	// `Def[args...]`; only the generic definition gets a type declaration.
	// TypeParam references never produce a declaration of their own.
	if typ.Generic != air.NoType || typ.Kind == air.TypeParam {
		return nil, nil
	}
	switch typ.Kind {
	case air.TypeStruct:
		fields := make([]*ast.Field, 0, len(typ.Fields))
		for _, field := range typ.Fields {
			var fieldType ast.Expr
			var err error
			if field.Mutable && l.isTraitObjectType(field.Type) {
				fieldType, err = l.mutableTraitRefType(field.Type)
			} else {
				fieldType, err = l.goType(field.Type)
			}
			if err != nil {
				return nil, err
			}
			if field.Mutable {
				fieldType = &ast.StarExpr{X: fieldType}
			}
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(l.goFieldName(typ, field.Name))},
				Type:  fieldType,
				Tag:   &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("`json:%q`", field.Name)},
			})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), TypeParams: l.goTypeParamList(typ), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeUnion:
		fields := []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(unionTagFieldName(typ))}, Type: ast.NewIdent("uint32")}}
		for _, member := range typ.Members {
			memberType, err := l.goType(member.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(unionMemberFieldName(typ, member))}, Type: memberType})
		}
		unionDecl := &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}
		return []ast.Decl{unionDecl, l.unionMarshalJSONDecl(typ)}, nil
	case air.TypeTraitObject:
		return l.lowerTraitObjectDecls(typ)
	case air.TypeEnum:
		typeSpec := &ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: ast.NewIdent("int")}
		specs := []ast.Spec{typeSpec}
		for _, variant := range typ.Variants {
			value := ast.Expr(&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", variant.Discriminant)})
			specs = append(specs, &ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(enumVariantName(l.program, typ, variant))}, Type: ast.NewIdent(typeName(l.program, typ)), Values: []ast.Expr{value}})
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

func (l *lowerer) markedMutableTraitRefDecls() ([]ast.Decl, error) {
	traitIDs := make([]int, 0, len(l.mutableTraitRefs))
	for traitID := range l.mutableTraitRefs {
		if l.emittedMutableTraitRefs[traitID] {
			continue
		}
		traitIDs = append(traitIDs, int(traitID))
	}
	sort.Ints(traitIDs)
	decls := make([]ast.Decl, 0, len(traitIDs))
	for _, raw := range traitIDs {
		traitID := air.TraitID(raw)
		if !validTraitID(l.program, traitID) {
			continue
		}
		decl, err := l.lowerMutableTraitRefDecl(l.program.Traits[traitID])
		if err != nil {
			return nil, err
		}
		decls = append(decls, decl)
		l.emittedMutableTraitRefs[traitID] = true
	}
	return decls, nil
}

// unionMarshalJSONDecl generates a MarshalJSON method that encodes a union as
// its active member's value, unwrapped (ADR 0031).
func (l *lowerer) unionMarshalJSONDecl(typ air.TypeInfo) *ast.FuncDecl {
	recv := "u"
	cases := make([]ast.Stmt, 0, len(typ.Members))
	for _, member := range typ.Members {
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
				Fun:  l.qualified("json", "encoding/json/v2", "Marshal"),
				Args: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(recv), Sel: ast.NewIdent(unionMemberFieldName(typ, member))}},
			}}}},
		})
	}
	body := &ast.BlockStmt{List: []ast.Stmt{
		&ast.SwitchStmt{Tag: &ast.SelectorExpr{X: ast.NewIdent(recv), Sel: ast.NewIdent(unionTagFieldName(typ))}, Body: &ast.BlockStmt{List: cases}},
		&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("nil"), &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Errorf"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"invalid union tag"`}}}}},
	}}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(recv)}, Type: ast.NewIdent(typeName(l.program, typ))}}},
		Name: ast.NewIdent("MarshalJSON"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Elt: ast.NewIdent("byte")}}, {Type: ast.NewIdent("error")}}}},
		Body: body,
	}
}

func (l *lowerer) lowerTraitObjectDecls(typ air.TypeInfo) ([]ast.Decl, error) {
	if !validTraitID(l.program, typ.Trait) {
		return nil, fmt.Errorf("invalid trait id %d", typ.Trait)
	}
	trait := l.program.Traits[typ.Trait]
	decls := []ast.Decl{}
	interfaceDecl, ok, err := l.lowerTraitInterfaceDecl(trait)
	if err != nil {
		return nil, err
	}
	if ok {
		decls = append(decls, interfaceDecl)
	}
	mutableDecl, err := l.lowerMutableTraitRefDecl(trait)
	if err != nil {
		return nil, err
	}
	decls = append(decls, mutableDecl)
	if l.emittedMutableTraitRefs != nil {
		l.emittedMutableTraitRefs[trait.ID] = true
	}
	return decls, nil
}

func (l *lowerer) lowerTraitInterfaceDecl(trait air.Trait) (ast.Decl, bool, error) {
	if !l.traitInterfaceAvailable(trait.ID) {
		return nil, false, nil
	}
	methods := make([]*ast.Field, 0, len(trait.Methods))
	for _, method := range trait.Methods {
		methodName, _ := goMethodName(method.Name)
		methodType, err := l.traitInterfaceMethodType(method)
		if err != nil {
			return nil, false, err
		}
		methods = append(methods, &ast.Field{Names: []*ast.Ident{ast.NewIdent(methodName)}, Type: methodType})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(l.traitInterfaceTypeName(trait)), Type: &ast.InterfaceType{Methods: &ast.FieldList{List: methods}}}}}, true, nil
}

func (l *lowerer) traitInterfaceAvailable(traitID air.TraitID) bool {
	if !validTraitID(l.program, traitID) {
		return false
	}
	seen := map[string]bool{}
	for _, method := range l.program.Traits[traitID].Methods {
		methodName, ok := goMethodName(method.Name)
		if !ok || seen[methodName] {
			return false
		}
		seen[methodName] = true
	}
	return true
}

func (l *lowerer) usesNativeTraitInterface(typeID air.TypeID) bool {
	if !l.isTraitObjectType(typeID) {
		return false
	}
	traitID := l.program.Types[typeID-1].Trait
	if !l.traitInterfaceAvailable(traitID) || l.traitHasMutableTraitUse(traitID) {
		return false
	}
	for _, impl := range l.program.Impls {
		if impl.Trait != traitID {
			continue
		}
		for _, methodID := range impl.Methods {
			if !validFunctionID(l.program, methodID) {
				return false
			}
			methodFn := l.program.Functions[methodID]
			if len(methodFn.Signature.Params) == 0 {
				return false
			}
			key, _, ok := l.goMethodKey(methodFn)
			if !ok || l.goMethodCollisions[key] {
				return false
			}
		}
	}
	return true
}

func (l *lowerer) traitHasMutableTraitUse(traitID air.TraitID) bool {
	for _, fn := range l.program.Functions {
		for _, param := range fn.Signature.Params {
			if param.Mutable && l.paramIsTrait(param, traitID) {
				return true
			}
		}
	}
	for _, typ := range l.program.Types {
		for _, field := range typ.Fields {
			if field.Mutable && l.typeIDIsTrait(field.Type, traitID) {
				return true
			}
		}
		for i, paramTypeID := range typ.Params {
			if i < len(typ.ParamMutable) && typ.ParamMutable[i] && l.typeIDIsTrait(paramTypeID, traitID) {
				return true
			}
		}
	}
	return false
}

func (l *lowerer) paramIsTrait(param air.Param, traitID air.TraitID) bool {
	return l.typeIDIsTrait(param.Type, traitID)
}

func (l *lowerer) typeIDIsTrait(typeID air.TypeID, traitID air.TraitID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeTraitObject && l.program.Types[typeID-1].Trait == traitID
}

func (l *lowerer) traitInterfaceMethodType(method air.TraitMethod) (*ast.FuncType, error) {
	params := make([]*ast.Field, 0, len(method.Signature.Params))
	for _, param := range method.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{Type: paramType})
	}
	fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !l.isVoidType(method.Signature.Return) {
		returnType, err := l.goType(method.Signature.Return)
		if err != nil {
			return nil, err
		}
		fnType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	return fnType, nil
}

func (l *lowerer) lowerMutableTraitRefDecl(trait air.Trait) (ast.Decl, error) {
	fields := []*ast.Field{
		{Names: []*ast.Ident{ast.NewIdent(mutableTraitLoadFieldName(trait))}, Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}}}},
		{Names: []*ast.Ident{ast.NewIdent(mutableTraitAssignFieldName(trait))}, Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("value")}, Type: ast.NewIdent("any")}}}}},
	}
	for i, method := range trait.Methods {
		fieldType, err := l.mutableTraitMethodFuncType(method)
		if err != nil {
			return nil, err
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(mutableTraitMethodFieldName(trait.ID, i))}, Type: fieldType})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(mutableTraitRefTypeName(trait)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}, nil
}

func (l *lowerer) mutableTraitMethodFuncType(method air.TraitMethod) (ast.Expr, error) {
	params := make([]*ast.Field, 0, len(method.Signature.Params))
	for i, param := range method.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(fmt.Sprintf("arg%d", i))}, Type: paramType})
	}
	fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !l.isVoidType(method.Signature.Return) {
		returnType, err := l.goType(method.Signature.Return)
		if err != nil {
			return nil, err
		}
		fnType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	return fnType, nil
}

func (l *lowerer) traitInterfaceTypeName(trait air.Trait) string {
	if name, ok := l.naturalTraitInterfaceTypeName(trait); ok {
		return name
	}
	return legacyTraitInterfaceTypeName(trait)
}

func (l *lowerer) naturalTraitInterfaceTypeName(trait air.Trait) (string, bool) {
	if trait.Name == "" {
		return "", false
	}
	name := naturalGoIdentifier(trait.Name, !trait.Private)
	if name == "" || name == "_" || isReservedTopLevelName(name) || topLevelNaturalNameCollides(l.program, topLevelNameTrait, int(trait.ID), name) {
		return "", false
	}
	return name, true
}

func (l *lowerer) traitInterfaceTypeExpr(trait air.Trait) ast.Expr {
	name := l.traitInterfaceTypeName(trait)
	if !l.useModulePackages {
		return ast.NewIdent(name)
	}
	owner, ok := l.ownerModuleForTrait(trait.ID)
	if !ok || owner == l.currentModule {
		return ast.NewIdent(name)
	}
	return l.moduleQualified(owner, name)
}

// goTypeParamList renders the Go type-parameter list `[T any, ...]` for a
// generic definition, or nil for a non-generic type.
func (l *lowerer) goTypeParamList(typ air.TypeInfo) *ast.FieldList {
	if len(typ.TypeParams) == 0 {
		return nil
	}
	fields := make([]air.Param, len(typ.Fields))
	for i, f := range typ.Fields {
		fields[i] = air.Param{Type: f.Type}
	}
	comparable := l.comparableTypeParams(air.Signature{Params: fields}, nil)
	return l.typeParamFieldList(typ.TypeParams, comparable)
}

func (l *lowerer) namedTypeExpr(info air.TypeInfo) ast.Expr {
	// A generic type parameter lowers to its Go identifier inside the generic
	// definition's scope (ADR 0031).
	if info.Kind == air.TypeParam {
		return ast.NewIdent(info.Name)
	}
	// A generic instantiation lowers to `Def[args...]`.
	if info.Generic != air.NoType && validTypeID(l.program, info.Generic) {
		defInfo := l.program.Types[info.Generic-1]
		base := l.namedTypeExpr(defInfo)
		args := make([]ast.Expr, len(info.GenericArgs))
		for i, argID := range info.GenericArgs {
			args[i] = mustTypeExpr(l, argID)
		}
		if len(args) == 1 {
			return &ast.IndexExpr{X: base, Index: args[0]}
		}
		return &ast.IndexListExpr{X: base, Indices: args}
	}
	name := typeName(l.program, info)
	if !l.useModulePackages {
		return ast.NewIdent(name)
	}
	owner, ok := l.ownerModuleForType(info.ID)
	if !ok || owner == l.currentModule {
		return ast.NewIdent(name)
	}
	return l.moduleQualified(owner, name)
}

func (l *lowerer) compositeTypeExpr(info air.TypeInfo) ast.Expr {
	return l.namedTypeExpr(info)
}

func (l *lowerer) enumVariantExpr(typ air.TypeInfo, variant air.VariantInfo) ast.Expr {
	name := enumVariantName(l.program, typ, variant)
	if !l.useModulePackages {
		return ast.NewIdent(name)
	}
	owner, ok := l.ownerModuleForType(typ.ID)
	if !ok || owner == l.currentModule {
		return ast.NewIdent(name)
	}
	return l.moduleQualified(owner, name)
}

func (l *lowerer) functionExpr(fn air.Function) ast.Expr {
	name := l.goFunctionName(fn)
	module := l.functionModule(fn)
	if !l.useModulePackages || module == l.currentModule {
		return ast.NewIdent(name)
	}
	return l.moduleQualified(module, name)
}

func (l *lowerer) globalExpr(global air.Global) ast.Expr {
	name := globalName(l.program, global)
	if !l.useModulePackages || global.Module == l.currentModule {
		return ast.NewIdent(name)
	}
	return l.moduleQualified(global.Module, name)
}

func (l *lowerer) moduleQualified(module air.ModuleID, name string) ast.Expr {
	return l.qualified(l.moduleImportAlias(module), moduleImportPath(l.program, module), name)
}

func (l *lowerer) moduleImportAlias(module air.ModuleID) string {
	base := modulePackageName(l.program, module)
	importPath := moduleImportPath(l.program, module)
	if l.currentImports == nil {
		return base
	}
	if existing, ok := l.currentImports[base]; !ok || existing == importPath {
		return base
	}
	for i := 2; ; i++ {
		alias := fmt.Sprintf("%s%d", base, i)
		if existing, ok := l.currentImports[alias]; !ok || existing == importPath {
			return alias
		}
	}
}

func legacyTraitInterfaceTypeName(trait air.Trait) string {
	return fmt.Sprintf("ardTrait_%s_%d", sanitizeName(trait.Name), trait.ID)
}

func mutableTraitRefTypeName(trait air.Trait) string {
	return fmt.Sprintf("ardMutTrait_%s_%d", sanitizeName(trait.Name), trait.ID)
}

func mutableTraitLoadFieldName(trait air.Trait) string {
	return fmt.Sprintf("ardMutTraitLoad_%d", trait.ID)
}

func mutableTraitAssignFieldName(trait air.Trait) string {
	return fmt.Sprintf("ardMutTraitAssign_%d", trait.ID)
}

func mutableTraitMethodFieldName(trait air.TraitID, methodIndex int) string {
	return fmt.Sprintf("ardMutTraitMethod_%d_%d", trait, methodIndex)
}

func (l *lowerer) lowerGlobal(global air.Global) (ast.Decl, error) {
	globalType, err := l.goType(global.Type)
	if err != nil {
		return nil, err
	}
	value, err := l.lowerExprWithExpectedType(air.Function{Module: global.Module, Name: "<global>"}, global.Value, global.Type)
	if err != nil {
		return nil, err
	}
	valueExpr := value.expr
	if l.isVoidType(global.Type) || isVoidExpr(valueExpr) {
		if len(value.stmts) != 0 || !isVoidExpr(valueExpr) {
			body := append([]ast.Stmt{}, value.stmts...)
			body = l.appendVoidValueEval(body, valueExpr)
			body = append(body, &ast.ReturnStmt{Results: []ast.Expr{l.voidValueExpr()}})
			valueExpr = &ast.CallExpr{Fun: &ast.FuncLit{
				Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: globalType}}}},
				Body: &ast.BlockStmt{List: body},
			}}
		} else {
			valueExpr = l.voidValueExpr()
		}
	} else if len(value.stmts) != 0 {
		return nil, fmt.Errorf("global initializers with setup statements are not supported")
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names:  []*ast.Ident{ast.NewIdent(globalName(l.program, global))},
		Type:   globalType,
		Values: []ast.Expr{valueExpr},
	}}}, nil
}

func (l *lowerer) lowerFunction(fn air.Function) (ast.Decl, error) {
	l.declaredLocals = map[air.LocalID]bool{}
	params := []*ast.Field{}
	for _, capture := range fn.Captures {
		captureParam := air.Param{Name: capture.Name, Type: capture.Type}
		if int(capture.Local) >= 0 && int(capture.Local) < len(fn.Locals) {
			captureParam.Mutable = fn.Locals[capture.Local].Mutable
		}
		captureType, err := l.goParamType(captureParam)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(l.localName(fn, capture.Local))},
			Type:  captureType,
		})
		l.declaredLocals[capture.Local] = true
	}
	for i, param := range fn.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(l.localName(fn, air.LocalID(i)))},
			Type:  paramType,
		})
	}
	for _, local := range fn.Locals {
		if int(local.ID) < len(fn.Signature.Params) {
			l.declaredLocals[local.ID] = true
		}
	}
	returnTypeID := fn.Signature.Return
	if len(fn.Captures) > 0 && l.isVoidType(returnTypeID) && fn.Body.Result != nil && !l.isVoidType(fn.Body.Result.Type) {
		returnTypeID = fn.Body.Result.Type
	}
	body, err := l.lowerBlock(fn, fn.Body, returnTypeID)
	if err != nil {
		return nil, err
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}, TypeParams: l.goFuncTypeParamList(fn)}
	if !l.isVoidType(returnTypeID) {
		returnType, err := l.goType(returnTypeID)
		if err != nil {
			return nil, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(l.goFunctionName(fn)),
		Type: funcType,
		Body: body,
	}, nil
}

// goFuncTypeParamList renders `[T any, ...]` for a generic function definition,
// or nil for a non-generic function.
// indexWithTypeArgs renders `fun[arg]` or `fun[arg, ...]` for a generic call.
// indexWithTypeParamNames renders `fun[T, ...]` using the in-scope type
// parameter identifiers, used when instantiating a lifted closure inside its
// enclosing generic definition.
func (l *lowerer) indexWithTypeParamNames(fun ast.Expr, names []string) ast.Expr {
	if len(names) == 1 {
		return &ast.IndexExpr{X: fun, Index: ast.NewIdent(names[0])}
	}
	indices := make([]ast.Expr, len(names))
	for i, n := range names {
		indices[i] = ast.NewIdent(n)
	}
	return &ast.IndexListExpr{X: fun, Indices: indices}
}

func (l *lowerer) indexWithTypeArgs(fun ast.Expr, typeArgs []air.TypeID) ast.Expr {
	if len(typeArgs) == 1 {
		return &ast.IndexExpr{X: fun, Index: mustTypeExpr(l, typeArgs[0])}
	}
	indices := make([]ast.Expr, len(typeArgs))
	for i, ta := range typeArgs {
		indices[i] = mustTypeExpr(l, ta)
	}
	return &ast.IndexListExpr{X: fun, Indices: indices}
}

func (l *lowerer) goFuncTypeParamList(fn air.Function) *ast.FieldList {
	if len(fn.TypeParams) == 0 {
		return nil
	}
	comparable := l.comparableTypeParams(fn.Signature, fn.Locals)
	return l.typeParamFieldList(fn.TypeParams, comparable)
}

// typeParamFieldList renders `[T any, K comparable, ...]`, constraining a
// parameter to `comparable` when it is used as a Go map key (Go requires map
// keys to be comparable).
func (l *lowerer) typeParamFieldList(typeParams []string, comparable map[string]bool) *ast.FieldList {
	fields := make([]*ast.Field, len(typeParams))
	for i, p := range typeParams {
		constraint := "any"
		if comparable[p] {
			constraint = "comparable"
		}
		fields[i] = &ast.Field{Names: []*ast.Ident{ast.NewIdent(p)}, Type: ast.NewIdent(constraint)}
	}
	return &ast.FieldList{List: fields}
}

// comparableTypeParams returns the set of type parameter names that appear as a
// map key within the given signature and locals, and therefore require the
// `comparable` constraint.
func (l *lowerer) comparableTypeParams(signature air.Signature, locals []air.Local) map[string]bool {
	result := map[string]bool{}
	seen := map[air.TypeID]bool{}
	var walk func(id air.TypeID)
	walk = func(id air.TypeID) {
		if id == air.NoType || seen[id] {
			return
		}
		seen[id] = true
		info, ok := l.typeInfo(id)
		if !ok {
			return
		}
		if info.Kind == air.TypeMap {
			if key, ok := l.typeInfo(info.Key); ok && key.Kind == air.TypeParam {
				result[key.Name] = true
			}
		}
		walk(info.Elem)
		walk(info.Key)
		walk(info.Value)
		walk(info.Return)
		walk(info.Error)
		for _, p := range info.Params {
			walk(p)
		}
		for _, f := range info.Fields {
			walk(f.Type)
		}
		for _, m := range info.Members {
			walk(m.Type)
		}
		for _, ga := range info.GenericArgs {
			walk(ga)
		}
	}
	for _, p := range signature.Params {
		walk(p.Type)
	}
	walk(signature.Return)
	for _, loc := range locals {
		walk(loc.Type)
	}
	return result
}

func (l *lowerer) lowerGoMethodWrapper(fn air.Function) (*ast.FuncDecl, bool, error) {
	key, methodName, ok := l.goMethodKey(fn)
	if !ok || l.goMethodCollisions[key] || l.emittedGoMethods[key] {
		return nil, false, nil
	}
	if len(fn.Signature.Params) == 0 {
		return nil, false, nil
	}
	receiverTypeID := fn.Receiver
	if receiverTypeID == air.NoType {
		receiverTypeID = fn.Signature.Params[0].Type
	}
	// A method on a generic struct is a real Go generic-receiver method
	// (`func (self Foo[T]) M(...)`), where the receiver binds the type
	// parameters. A method on a *concrete* instantiation cannot be expressed in
	// Go (the receiver `Foo[int]` would bind a fresh type parameter named
	// `int`); skip its wrapper and rely on the standalone function instead.
	if validTypeID(l.program, receiverTypeID) && l.program.Types[receiverTypeID-1].Generic != air.NoType && len(fn.TypeParams) == 0 {
		return nil, false, nil
	}
	receiverType, err := l.goType(receiverTypeID)
	if err != nil {
		return nil, false, err
	}
	if fn.Signature.Params[0].Mutable {
		receiverType = &ast.StarExpr{X: receiverType}
	}

	params := make([]*ast.Field, 0, len(fn.Signature.Params)-1)
	callArgs := []ast.Expr{ast.NewIdent(l.localName(fn, 0))}
	for i, param := range fn.Signature.Params[1:] {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, false, err
		}
		localID := air.LocalID(i + 1)
		name := l.localName(fn, localID)
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: paramType})
		callArgs = append(callArgs, ast.NewIdent(name))
	}

	callFun := l.functionExpr(fn)
	if len(fn.TypeParams) > 0 {
		// Instantiate the standalone generic method function with the receiver's
		// type parameters, which the receiver `Foo[T]` brings into scope.
		callFun = l.indexWithTypeParamNames(callFun, fn.TypeParams)
	}
	call := &ast.CallExpr{Fun: callFun, Args: callArgs}
	body := []ast.Stmt{}
	if l.isVoidType(fn.Signature.Return) {
		body = append(body, &ast.ExprStmt{X: call})
	} else {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !l.isVoidType(fn.Signature.Return) {
		returnType, err := l.goType(fn.Signature.Return)
		if err != nil {
			return nil, false, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}

	l.emittedGoMethods[key] = true
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(l.localName(fn, 0))}, Type: receiverType}}},
		Name: ast.NewIdent(methodName),
		Type: funcType,
		Body: &ast.BlockStmt{List: body},
	}, true, nil
}

func (l *lowerer) collectGoMethodCollisions() map[string]bool {
	counts := map[string]int{}
	for _, fn := range l.program.Functions {
		key, _, ok := l.goMethodKey(fn)
		if ok {
			counts[key]++
		}
	}
	collisions := map[string]bool{}
	for key, count := range counts {
		if count > 1 {
			collisions[key] = true
		}
	}
	return collisions
}

func (l *lowerer) goMethodKey(fn air.Function) (string, string, bool) {
	if strings.TrimSpace(fn.MethodName) == "" || len(fn.Signature.Params) == 0 {
		return "", "", false
	}
	receiverTypeID := fn.Receiver
	if receiverTypeID == air.NoType {
		receiverTypeID = fn.Signature.Params[0].Type
	}
	if !l.canEmitGoMethodOnType(receiverTypeID) {
		return "", "", false
	}
	methodName, ok := goMethodName(fn.MethodName)
	if !ok || l.goMethodNameUnavailableOnType(receiverTypeID, methodName) {
		return "", "", false
	}
	return fmt.Sprintf("%d:%s", receiverTypeID, methodName), methodName, true
}

func (l *lowerer) canEmitGoMethodOnType(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeStruct, air.TypeEnum, air.TypeUnion:
		return true
	default:
		return false
	}
}

func (l *lowerer) goMethodNameUnavailableOnType(typeID air.TypeID, methodName string) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeStruct:
		if generatedStructReceiverMethodName(methodName) {
			return true
		}
		for _, field := range info.Fields {
			if l.goFieldName(info, field.Name) == methodName {
				return true
			}
		}
	case air.TypeUnion:
		if methodName == unionTagFieldName(info) {
			return true
		}
		for _, member := range info.Members {
			if unionMemberFieldName(info, member) == methodName {
				return true
			}
		}
	}
	return false
}

func generatedStructReceiverMethodName(name string) bool {
	switch name {
	case "MarshalJSONTo", "UnmarshalJSONFrom":
		return true
	default:
		return false
	}
}

func goMethodName(raw string) (string, bool) {
	if len(goIdentifierParts(raw)) == 0 {
		return "", false
	}
	name := naturalGoIdentifier(raw, true)
	if name == "" || name == "_" || !token.IsIdentifier(name) {
		return "", false
	}
	return name, true
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
		result, err := l.lowerExprWithExpectedType(fn, *block.Result, returnType)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, result.stmts...)
		if returnType == air.NoType || l.isVoidType(returnType) {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				stmts = l.appendVoidValueEval(stmts, result.expr)
			} else {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
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
		localType := l.resolvedLocalType(fn, stmt.Local)
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, localType)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, value.stmts...)
		name := l.localName(fn, stmt.Local)
		tok := token.DEFINE
		if l.declaredLocals[stmt.Local] {
			tok = token.ASSIGN
		} else {
			l.declaredLocals[stmt.Local] = true
		}
		if l.isVoidType(localType) || isVoidExpr(value.expr) {
			out = l.appendVoidValueEval(out, value.expr)
			out = append(out, &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(name)},
				Tok: tok,
				Rhs: []ast.Expr{l.voidValueExpr()},
			})
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(name)}})
			return out, nil
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
		localType := l.resolvedLocalType(fn, stmt.Local)
		var value loweredExpr
		var err error
		if l.localIsPointerParam(fn, stmt.Local) && l.isTraitObjectType(localType) {
			value, err = l.lowerExpr(fn, *stmt.Value)
		} else {
			value, err = l.lowerExprWithExpectedType(fn, *stmt.Value, localType)
		}
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, value.stmts...)
		if l.isVoidType(localType) || isVoidExpr(value.expr) {
			out = l.appendVoidValueEval(out, value.expr)
			name := l.localName(fn, stmt.Local)
			tok := token.ASSIGN
			if !l.declaredLocals[stmt.Local] {
				tok = token.DEFINE
				l.declaredLocals[stmt.Local] = true
			}
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(name)}, Tok: tok, Rhs: []ast.Expr{l.voidValueExpr()}})
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(name)}})
			return out, nil
		}
		if l.localIsPointerParam(fn, stmt.Local) && l.isTraitObjectType(localType) {
			assignValue := l.mutableTraitAssignValueExpr(fn, *stmt.Value, value.expr, localType)
			out = append(out, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(l.localName(fn, stmt.Local)), Sel: ast.NewIdent(l.mutableTraitAssignFieldNameForType(localType))}, Args: []ast.Expr{assignValue}}})
			return out, nil
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{l.localAssignExpr(fn, stmt.Local)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value.expr},
		})
		return out, nil
	case air.StmtSetField:
		if stmt.Target == nil {
			return nil, fmt.Errorf("field set statement missing target")
		}
		if stmt.Value == nil {
			return nil, fmt.Errorf("field set statement missing value")
		}
		target, err := l.lowerExpr(fn, *stmt.Target)
		if err != nil {
			return nil, err
		}
		if !validTypeID(l.program, stmt.Target.Type) {
			return nil, fmt.Errorf("invalid field set target type %d", stmt.Target.Type)
		}
		targetType := l.program.Types[stmt.Target.Type-1]
		if targetType.Kind != air.TypeStruct {
			return nil, fmt.Errorf("field set target must be struct, got kind %d", targetType.Kind)
		}
		if stmt.Field < 0 || stmt.Field >= len(targetType.Fields) {
			return nil, fmt.Errorf("invalid field set index %d", stmt.Field)
		}
		field := targetType.Fields[stmt.Field]
		var value loweredExpr
		if field.Mutable && l.isTraitObjectType(field.Type) {
			value, err = l.lowerExpr(fn, *stmt.Value)
		} else {
			value, err = l.lowerExprWithExpectedType(fn, *stmt.Value, field.Type)
		}
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, target.stmts...)
		out = append(out, value.stmts...)
		fieldTarget := ast.Expr(&ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(l.goFieldName(targetType, field.Name))})
		if field.Mutable && l.isTraitObjectType(field.Type) {
			assignValue := l.mutableTraitAssignValueExpr(fn, *stmt.Value, value.expr, field.Type)
			out = append(out, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: fieldTarget, Sel: ast.NewIdent(l.mutableTraitAssignFieldNameForType(field.Type))}, Args: []ast.Expr{assignValue}}})
			return out, nil
		}
		if field.Mutable {
			fieldTarget = &ast.StarExpr{X: fieldTarget}
		}
		valueExpr := value.expr
		if l.isVoidType(field.Type) || isVoidExpr(valueExpr) {
			out = l.appendVoidValueEval(out, valueExpr)
			valueExpr = l.voidValueExpr()
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{fieldTarget},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{valueExpr},
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
			out = l.appendVoidValueEval(out, expr.expr)
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
		return loweredExpr{expr: l.voidValueExpr()}, nil
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
	case air.ExprPanic:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("panic missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append([]ast.Stmt{}, target.stmts...)
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{target.expr}}})
		zero, err := l.zeroValueExpr(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: stmts, expr: zero}, nil
	case air.ExprLoadLocal:
		return loweredExpr{expr: l.localValueExpr(fn, expr.Local)}, nil
	case air.ExprLoadGlobal:
		if expr.Global < 0 || int(expr.Global) >= len(l.program.Globals) {
			return loweredExpr{}, fmt.Errorf("unknown global %d", expr.Global)
		}
		return loweredExpr{expr: l.globalExpr(l.program.Globals[expr.Global])}, nil
	case air.ExprFunctionRef:
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("unknown function %d", expr.Function)
		}
		return loweredExpr{expr: l.functionExpr(l.program.Functions[expr.Function])}, nil
	case air.ExprUnionWrap:
		return l.lowerUnionWrap(fn, expr)
	case air.ExprMatchUnion:
		return l.lowerMatchUnion(fn, expr)
	case air.ExprTraitUpcast:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("trait upcast missing target")
		}
		// Convert the concrete value to the trait-object representation so
		// subsequent assignments and dispatches use the correct type.
		if l.usesNativeTraitInterface(expr.Type) {
			traitType, err := l.goType(expr.Type)
			if err != nil {
				return loweredExpr{}, err
			}
			if l.implRequiresPointerReceiver(expr.Impl) {
				place, setup, ok, err := l.mutableTraitUpcastPlace(fn, *expr.Target)
				if err != nil {
					return loweredExpr{}, err
				}
				if ok {
					return loweredExpr{stmts: setup, expr: &ast.CallExpr{Fun: traitType, Args: []ast.Expr{addressOfPlace(place)}}}, nil
				}
				target, err := l.lowerExpr(fn, *expr.Target)
				if err != nil {
					return loweredExpr{}, err
				}
				temp := l.nextTemp()
				tempType, err := l.goType(expr.Target.Type)
				if err != nil {
					return loweredExpr{}, err
				}
				stmts := append([]ast.Stmt{}, target.stmts...)
				stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(temp)}, Type: tempType}}}})
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
				return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: traitType, Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(temp)}}}}, nil
			}
			target, err := l.lowerExpr(fn, *expr.Target)
			if err != nil {
				return loweredExpr{}, err
			}
			return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: traitType, Args: []ast.Expr{target.expr}}}, nil
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{target.expr}}}, nil
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
		return loweredExpr{stmts: target.stmts, expr: l.toStringExpr(expr.Target.Type, target.expr)}, nil
	case air.ExprToInt:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to_int missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("int"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprMakeClosure:
		return l.lowerMakeClosure(fn, expr)
	case air.ExprCallClosure:
		return l.lowerCallClosure(fn, expr)
	case air.ExprMakeMaybeSome:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("maybe some missing target")
		}
		expected := air.NoType
		if validTypeID(l.program, expr.Type) {
			if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeMaybe {
				expected = info.Elem
			}
		}
		var target loweredExpr
		var err error
		if expected != air.NoType {
			target, err = l.lowerExprWithExpectedType(fn, *expr.Target, expected)
		} else {
			target, err = l.lowerExpr(fn, *expr.Target)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		valueExpr := target.expr
		if l.isVoidType(expr.Target.Type) || isVoidExpr(valueExpr) {
			target = l.materializeVoidValue(target)
			valueExpr = target.expr
		}
		someExpr, err := l.maybeSomeExpr(expr.Type, valueExpr)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: someExpr}, nil
	case air.ExprMakeMaybeNone:
		noneExpr, err := l.maybeNoneExpr(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{expr: noneExpr}, nil
	case air.ExprMakeResultOk:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("result ok missing target")
		}
		expected := air.NoType
		if validTypeID(l.program, expr.Type) {
			if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeResult {
				expected = info.Value
			}
		}
		var target loweredExpr
		var err error
		if expected != air.NoType {
			target, err = l.lowerExprWithExpectedType(fn, *expr.Target, expected)
		} else {
			target, err = l.lowerExpr(fn, *expr.Target)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		typ, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		valueExpr := target.expr
		if l.isVoidType(expr.Target.Type) || isVoidExpr(valueExpr) {
			target = l.materializeVoidValue(target)
			valueExpr = target.expr
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: typ, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
			&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
		}}}, nil
	case air.ExprMakeResultErr:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("result err missing target")
		}
		expected := air.NoType
		if validTypeID(l.program, expr.Type) {
			if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeResult {
				expected = info.Error
			}
		}
		var target loweredExpr
		var err error
		if expected != air.NoType {
			target, err = l.lowerExprWithExpectedType(fn, *expr.Target, expected)
		} else {
			target, err = l.lowerExpr(fn, *expr.Target)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		typ, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		errExpr := target.expr
		if l.resultErrorIsVoid(expr.Type) || isVoidExpr(errExpr) {
			target = l.materializeVoidValue(target)
			errExpr = target.expr
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: typ, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		}}}, nil
	case air.ExprMatchMaybe:
		return l.lowerMatchMaybe(fn, expr)
	case air.ExprTryMaybe:
		return l.lowerTryMaybe(fn, expr)
	case air.ExprMaybeExpect:
		return l.lowerMaybeExpect(fn, expr)
	case air.ExprMaybeIsNone:
		return l.lowerMaybeIsNone(fn, expr)
	case air.ExprMaybeIsSome:
		return l.lowerMaybeIsSome(fn, expr)
	case air.ExprMaybeOr:
		return l.lowerMaybeOr(fn, expr)
	case air.ExprMaybeMap:
		return l.lowerMaybeMap(fn, expr)
	case air.ExprMaybeAndThen:
		return l.lowerMaybeAndThen(fn, expr)
	case air.ExprResultExpect:
		return l.lowerResultExpect(fn, expr)
	case air.ExprResultOr:
		return l.lowerResultOr(fn, expr)
	case air.ExprResultMap:
		return l.lowerResultMap(fn, expr)
	case air.ExprResultMapErr:
		return l.lowerResultMapErr(fn, expr)
	case air.ExprResultAndThen:
		return l.lowerResultAndThen(fn, expr)
	case air.ExprResultIsOk:
		return l.lowerResultIsOk(fn, expr)
	case air.ExprResultIsErr:
		return l.lowerResultIsErr(fn, expr)
	case air.ExprMatchResult:
		return l.lowerMatchResult(fn, expr)
	case air.ExprTryResult:
		return l.lowerTryResult(fn, expr)
	case air.ExprMatchEnum:
		return l.lowerMatchEnum(fn, expr)
	case air.ExprMatchInt:
		return l.lowerMatchInt(fn, expr)
	case air.ExprMatchStr:
		return l.lowerMatchStr(fn, expr)
	case air.ExprMakeList:
		return l.lowerMakeList(fn, expr)
	case air.ExprMakeChannel:
		return l.lowerMakeChannel(fn, expr)
	case air.ExprChannelSend:
		return l.lowerChannelSend(fn, expr)
	case air.ExprChannelRecv:
		return l.lowerChannelRecv(fn, expr)
	case air.ExprChannelClose:
		return l.lowerChannelClose(fn, expr)
	case air.ExprChannelNarrow:
		return l.lowerChannelNarrow(fn, expr)
	case air.ExprSelect:
		return l.lowerSelect(fn, expr)
	case air.ExprJSONParse:
		return l.lowerJSONParse(fn, expr)
	case air.ExprStrContains:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str contains expects target and substring")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		substr, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, substr.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Contains"), Args: []ast.Expr{target.expr, substr.expr}}}, nil
	case air.ExprStrReplace:
		if expr.Target == nil || len(expr.Args) != 2 {
			return loweredExpr{}, fmt.Errorf("str replace expects target, from, to")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		from, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		to, err := l.lowerExpr(fn, expr.Args[1])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, from.stmts...)
		stmts = append(stmts, to.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Replace"), Args: []ast.Expr{target.expr, from.expr, to.expr, &ast.BasicLit{Kind: token.INT, Value: "1"}}}}, nil
	case air.ExprStrReplaceAll:
		if expr.Target == nil || len(expr.Args) != 2 {
			return loweredExpr{}, fmt.Errorf("str replace_all expects target, from, to")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		from, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		to, err := l.lowerExpr(fn, expr.Args[1])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, from.stmts...)
		stmts = append(stmts, to.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "ReplaceAll"), Args: []ast.Expr{target.expr, from.expr, to.expr}}}, nil
	case air.ExprStrSplit:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str split expects target and delimiter")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		delimiter, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, delimiter.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Split"), Args: []ast.Expr{target.expr, delimiter.expr}}}, nil
	case air.ExprStrStartsWith:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str starts_with expects target and prefix")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		prefix, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, prefix.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "HasPrefix"), Args: []ast.Expr{target.expr, prefix.expr}}}, nil
	case air.ExprStrEndsWith:
		if expr.Target == nil || len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str ends_with expects target and suffix")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		suffix, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, suffix.stmts...)
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "HasSuffix"), Args: []ast.Expr{target.expr, suffix.expr}}}, nil
	case air.ExprToAny:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to any missing target")
		}
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprStrTrim:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str trim missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.qualified("strings", "strings", "Trim"), Args: []ast.Expr{target.expr, &ast.BasicLit{Kind: token.STRING, Value: `" "`}}}}, nil
	case air.ExprStrIsEmpty:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str is_empty missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.BinaryExpr{X: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}}, nil
	case air.ExprStrBytes:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str bytes missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Args: []ast.Expr{target.expr}}}, nil
	case air.ExprStrRunes:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str runes missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("rune")}, Args: []ast.Expr{target.expr}}}, nil
	case air.ExprStrSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprStrAt:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str at missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("str at expects one arg")
		}
		index, err := l.lowerExpr(fn, expr.Args[0])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append(target.stmts, index.stmts...)
		if validTypeID(l.program, expr.Type) && l.program.Types[expr.Type-1].Kind == air.TypeMaybe {
			resultTemp := l.nextTemp()
			decls, err := l.declareTemp(expr.Type, resultTemp)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, decls...)
			runesTemp := l.nextTemp()
			indexTemp := l.nextTemp()
			stmts = append(stmts,
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(runesTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("rune")}, Args: []ast.Expr{target.expr}}}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(indexTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{index.expr}},
			)
			cond := &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: ast.NewIdent(indexTemp), Op: token.LSS, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
				Op: token.LOR,
				Y:  &ast.BinaryExpr{X: ast.NewIdent(indexTemp), Op: token.GEQ, Y: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{ast.NewIdent(runesTemp)}}},
			}
			elemTypeID := l.program.Types[expr.Type-1].Elem
			elemType := mustTypeExpr(l, elemTypeID)
			noneCall := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "None"), Index: elemType}}
			someValue := ast.Expr(&ast.IndexExpr{X: ast.NewIdent(runesTemp), Index: ast.NewIdent(indexTemp)})
			if validTypeID(l.program, elemTypeID) && l.program.Types[elemTypeID-1].Kind == air.TypeStr {
				someValue = &ast.CallExpr{Fun: ast.NewIdent("string"), Args: []ast.Expr{someValue}}
			}
			someCall := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "Some"), Index: elemType}, Args: []ast.Expr{someValue}}
			stmts = append(stmts, &ast.IfStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneCall}}}},
				Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someCall}}}},
			})
			return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
		}
		byteExpr := &ast.IndexExpr{X: target.expr, Index: index.expr}
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("string"), Args: []ast.Expr{byteExpr}}}, nil
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
	case air.ExprListPrepend:
		return l.lowerListPrepend(fn, expr)
	case air.ExprListSet:
		return l.lowerListSet(fn, expr)
	case air.ExprListSwap:
		return l.lowerListSwap(fn, expr)
	case air.ExprListSort:
		return l.lowerListSort(fn, expr)
	case air.ExprMakeMap:
		return l.lowerMakeMap(fn, expr)
	case air.ExprMapSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("map size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprMapHas:
		return l.lowerMapHas(fn, expr)
	case air.ExprMapGet:
		return l.lowerMapGet(fn, expr)
	case air.ExprMapSet:
		return l.lowerMapSet(fn, expr)
	case air.ExprMapDrop:
		return l.lowerMapDrop(fn, expr)
	case air.ExprMapKeys:
		return l.lowerMapKeys(fn, expr)
	case air.ExprMapKeyAt:
		return l.lowerMapKeyAt(fn, expr)
	case air.ExprMapValueAt:
		return l.lowerMapValueAt(fn, expr)
	case air.ExprEnumVariant:
		if !validTypeID(l.program, expr.Type) {
			return loweredExpr{}, fmt.Errorf("invalid enum type id %d", expr.Type)
		}
		typ := l.program.Types[expr.Type-1]
		if typ.Kind != air.TypeEnum || expr.Variant < 0 || expr.Variant >= len(typ.Variants) {
			return loweredExpr{}, fmt.Errorf("invalid enum variant %d for type %s", expr.Variant, typ.Name)
		}
		return loweredExpr{expr: l.enumVariantExpr(typ, typ.Variants[expr.Variant])}, nil
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
			fieldInfo, hasFieldInfo := structFieldByName(typ, field.Name)
			var value loweredExpr
			var err error
			if hasFieldInfo && !fieldInfo.Mutable {
				value, err = l.lowerExprWithExpectedType(fn, field.Value, fieldInfo.Type)
			} else {
				value, err = l.lowerExpr(fn, field.Value)
			}
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, value.stmts...)
			fieldValue := value.expr
			if hasFieldInfo && l.isVoidType(fieldInfo.Type) {
				stmts = l.appendVoidValueEval(stmts, fieldValue)
				fieldValue = l.voidValueExpr()
			}
			if hasFieldInfo {
				if fieldInfo.Mutable {
					if l.isTraitObjectType(fieldInfo.Type) {
						adapted, setup, _, ok, err := l.mutableTraitObjectArg(fn, field.Value, fieldValue, air.Param{Type: fieldInfo.Type, Mutable: true})
						if err != nil {
							return loweredExpr{}, err
						}
						if ok {
							stmts = append(stmts, setup...)
							fieldValue = adapted
						} else {
							fieldValue = l.mutableReferenceArg(fn, field.Value, fieldValue)
						}
					} else {
						fieldValue = l.mutableReferenceArg(fn, field.Value, fieldValue)
					}
				}
			}
			elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(l.goFieldName(typ, field.Name)), Value: fieldValue})
		}
		return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: l.compositeTypeExpr(typ), Elts: elts}}, nil
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
		if targetType.Kind == air.TypeMaybe {
			if !validTypeID(l.program, targetType.Elem) {
				return loweredExpr{}, fmt.Errorf("invalid maybe elem type id %d", targetType.Elem)
			}
			elemType := l.program.Types[targetType.Elem-1]
			if elemType.Kind != air.TypeStruct || expr.Field < 0 || expr.Field >= len(elemType.Fields) {
				return loweredExpr{}, fmt.Errorf("invalid field index %d", expr.Field)
			}
			field := elemType.Fields[expr.Field]
			targetTemp := l.nextTemp()
			targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
			if err != nil {
				return loweredExpr{}, err
			}
			resultTemp := l.nextTemp()
			resultDecls, err := l.declareTemp(expr.Type, resultTemp)
			if err != nil {
				return loweredExpr{}, err
			}
			targetExpr := ast.NewIdent(targetTemp)
			resultExpr := ast.NewIdent(resultTemp)
			stmts := append([]ast.Stmt{}, target.stmts...)
			stmts = append(stmts, targetDecls...)
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
			stmts = append(stmts, resultDecls...)
			fieldExpr := &ast.SelectorExpr{X: l.maybeValueExpr(targetExpr), Sel: ast.NewIdent(l.goFieldName(elemType, field.Name))}
			assignValue := ast.Expr(fieldExpr)
			if expr.Type != field.Type {
				resultInfo := l.program.Types[expr.Type-1]
				if resultInfo.Kind == air.TypeMaybe && resultInfo.Elem == field.Type {
					assignValue, err = l.maybeSomeExpr(expr.Type, fieldExpr)
					if err != nil {
						return loweredExpr{}, err
					}
				} else {
					return loweredExpr{}, fmt.Errorf("unsupported maybe field projection from %s.%s to type %d", elemType.Name, field.Name, expr.Type)
				}
			}
			stmts = append(stmts, &ast.IfStmt{
				Cond: l.maybeIsSomeExpr(targetExpr),
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignValue}}}},
			})
			return loweredExpr{stmts: stmts, expr: resultExpr}, nil
		}
		if targetType.Kind != air.TypeStruct || expr.Field < 0 || expr.Field >= len(targetType.Fields) {
			return loweredExpr{}, fmt.Errorf("invalid field index %d", expr.Field)
		}
		field := targetType.Fields[expr.Field]
		fieldExpr := &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(l.goFieldName(targetType, field.Name))}
		if field.Mutable {
			return loweredExpr{stmts: target.stmts, expr: &ast.StarExpr{X: fieldExpr}}, nil
		}
		return loweredExpr{stmts: target.stmts, expr: fieldExpr}, nil
	case air.ExprBlock:
		return l.lowerBlockExpr(fn, expr)
	case air.ExprUnsafeBlock:
		return l.lowerUnsafeBlockExpr(fn, expr)
	case air.ExprIf:
		return l.lowerIfExpr(fn, expr)
	case air.ExprForeignCall:
		return l.lowerForeignCall(fn, expr)
	case air.ExprCall:
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid function id %d", expr.Function)
		}
		target := l.program.Functions[expr.Function]
		args, stmts, writeback, err := l.lowerCallArgs(fn, expr.Args, target.Signature.Params)
		if err != nil {
			return loweredExpr{}, err
		}
		fun := l.functionExpr(target)
		if len(expr.TypeArgs) > 0 {
			fun = l.indexWithTypeArgs(fun, expr.TypeArgs)
		}
		call := &ast.CallExpr{Fun: fun, Args: args}
		return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
	case air.ExprEq, air.ExprNotEq:
		leftTypeID := l.resolvedExprType(fn, *expr.Left)
		rightTypeID := l.resolvedExprType(fn, *expr.Right)
		var left loweredExpr
		var err error
		if l.isMaybeType(leftTypeID) && l.isWeakContextType(leftTypeID) && l.isMaybeType(rightTypeID) && !l.isWeakContextType(rightTypeID) {
			left, err = l.lowerExprWithExpectedType(fn, *expr.Left, rightTypeID)
		} else {
			left, err = l.lowerExpr(fn, *expr.Left)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		var right loweredExpr
		if l.isMaybeType(rightTypeID) && l.isWeakContextType(rightTypeID) && l.isMaybeType(leftTypeID) && !l.isWeakContextType(leftTypeID) {
			right, err = l.lowerExprWithExpectedType(fn, *expr.Right, leftTypeID)
		} else {
			right, err = l.lowerExpr(fn, *expr.Right)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		l.castEnumIntComparisonOperands(&left, leftTypeID, &right, rightTypeID)
		var equality ast.Expr = &ast.BinaryExpr{X: left.expr, Op: l.binaryToken(expr.Kind), Y: right.expr}
		if l.isMaybeType(leftTypeID) || l.isMaybeType(rightTypeID) {
			equality = &ast.CallExpr{Fun: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "MaybeEqual"), Args: []ast.Expr{left.expr, right.expr}}
			if expr.Kind == air.ExprNotEq {
				equality = &ast.UnaryExpr{Op: token.NOT, X: equality}
			}
		}
		return loweredExpr{stmts: append(left.stmts, right.stmts...), expr: equality}, nil
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte,
		air.ExprAnd, air.ExprOr, air.ExprStrConcat:
		leftTypeID := l.resolvedExprType(fn, *expr.Left)
		rightTypeID := l.resolvedExprType(fn, *expr.Right)
		left, err := l.lowerExpr(fn, *expr.Left)
		if err != nil {
			return loweredExpr{}, err
		}
		right, err := l.lowerExpr(fn, *expr.Right)
		if err != nil {
			return loweredExpr{}, err
		}
		if isComparisonKind(expr.Kind) {
			l.castEnumIntComparisonOperands(&left, leftTypeID, &right, rightTypeID)
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

func (l *lowerer) lowerUnsafeBlockExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	resultInfo, ok := l.typeInfo(expr.Type)
	if !ok || resultInfo.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("unsafe block lowered with non-Result type %d", expr.Type)
	}
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}

	resultName := l.nextTemp()
	recoveredName := l.nextTemp()
	recoverAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(recoveredName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("recover")}},
	}
	recoverCond := &ast.BinaryExpr{X: ast.NewIdent(recoveredName), Op: token.NEQ, Y: ast.NewIdent("nil")}
	recoverResult := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(resultName)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{ast.NewIdent(recoveredName)}}},
		}}},
	}
	deferRecover := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.IfStmt{Init: recoverAssign, Cond: recoverCond, Body: &ast.BlockStmt{List: []ast.Stmt{recoverResult}}}}},
	}}}

	helperFn := fn
	helperFn.Signature.Return = expr.Type
	body := []ast.Stmt{deferRecover}
	var valueExpr ast.Expr
	if l.isVoidType(resultInfo.Value) {
		loweredBody, err := l.lowerValueBlock(helperFn, expr.Body, resultInfo.Value, nil)
		if err != nil {
			return loweredExpr{}, err
		}
		body = append(body, loweredBody...)
		valueExpr = l.voidValueExpr()
	} else {
		valueName := l.nextTemp()
		decls, err := l.declareTemp(resultInfo.Value, valueName)
		if err != nil {
			return loweredExpr{}, err
		}
		body = append(body, decls...)
		loweredBody, err := l.lowerValueBlock(helperFn, expr.Body, resultInfo.Value, ast.NewIdent(valueName))
		if err != nil {
			return loweredExpr{}, err
		}
		body = append(body, loweredBody...)
		valueExpr = ast.NewIdent(valueName)
	}
	body = append(body, &ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
	}}}})

	return loweredExpr{expr: &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(resultName)}, Type: resultType}}}},
		Body: &ast.BlockStmt{List: body},
	}}}, nil
}

func (l *lowerer) lowerIfExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Condition == nil {
		return loweredExpr{}, fmt.Errorf("if expression missing condition")
	}
	condition, err := l.lowerExpr(fn, *expr.Condition)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, condition.stmts...)
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
		result, err := l.lowerExprWithExpectedType(fn, *block.Result, resultType)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, result.stmts...)
		if l.isVoidType(resultType) {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				stmts = l.appendVoidValueEval(stmts, result.expr)
			} else {
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
			}
		} else {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				return stmts, nil
			}
			if target == nil {
				return nil, fmt.Errorf("non-void block result missing target")
			}
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
		}
	}
	return stmts, nil
}

func (l *lowerer) lowerExprWithExpectedType(fn air.Function, expr air.Expr, expectedType air.TypeID) (loweredExpr, error) {
	if l.shouldLoadMutableTraitValue(fn, expr, expectedType) {
		return l.lowerMutableTraitValue(fn, expr, expectedType)
	}
	if expectedType != air.NoType && expectedType != expr.Type && l.canOverrideExprType(expr, expectedType) {
		cloned := expr
		cloned.Type = expectedType
		return l.lowerExpr(fn, cloned)
	}
	return l.lowerExpr(fn, expr)
}

func (l *lowerer) shouldLoadMutableTraitValue(fn air.Function, expr air.Expr, expectedType air.TypeID) bool {
	return l.isTraitObjectType(expectedType) && l.isTraitObjectType(expr.Type) && l.exprIsMutableReference(fn, expr)
}

func (l *lowerer) lowerMutableTraitValue(fn air.Function, expr air.Expr, expectedType air.TypeID) (loweredExpr, error) {
	value, err := l.lowerExpr(fn, expr)
	if err != nil {
		return loweredExpr{}, err
	}
	loadName := l.mutableTraitLoadFieldNameForType(expectedType)
	if loadName == "" {
		return loweredExpr{}, fmt.Errorf("invalid mutable trait value type %d", expectedType)
	}
	loaded := ast.Expr(&ast.CallExpr{Fun: &ast.SelectorExpr{X: value.expr, Sel: ast.NewIdent(loadName)}})
	if l.usesNativeTraitInterface(expectedType) {
		traitType, err := l.goType(expectedType)
		if err != nil {
			return loweredExpr{}, err
		}
		loaded = &ast.TypeAssertExpr{X: loaded, Type: traitType}
	}
	return loweredExpr{stmts: value.stmts, expr: loaded}, nil
}

func (l *lowerer) canOverrideExprType(expr air.Expr, expectedType air.TypeID) bool {
	if expr.Kind == air.ExprPanic {
		return expectedType != air.NoType
	}
	if !validTypeID(l.program, expr.Type) || !validTypeID(l.program, expectedType) {
		return false
	}
	if inferred := l.inferTypeFromExprShape(&expr); inferred == expectedType {
		return true
	}
	from := l.program.Types[expr.Type-1]
	to := l.program.Types[expectedType-1]
	if from.Kind != to.Kind {
		return false
	}
	switch expr.Kind {
	case air.ExprMakeList:
		return len(expr.Args) == 0 && from.Kind == air.TypeList && to.Kind == air.TypeList
	case air.ExprMakeResultOk, air.ExprMakeResultErr,
		air.ExprMakeMaybeSome, air.ExprMakeMaybeNone,
		air.ExprBlock, air.ExprIf,
		air.ExprMatchEnum, air.ExprMatchInt, air.ExprMatchStr, air.ExprMatchMaybe, air.ExprMatchResult,
		air.ExprSelect, air.ExprTryResult, air.ExprTryMaybe:
		return from.Kind == air.TypeResult || from.Kind == air.TypeMaybe
	default:
		return false
	}
}

func (l *lowerer) shouldPropagateMaybeNone(expr air.Expr) bool {
	if expr.Target == nil || expr.Type == expr.Target.Type {
		return false
	}
	if len(expr.None.Stmts) != 0 || expr.None.Result == nil {
		return false
	}
	return sameAIRExpr(*expr.None.Result, *expr.Target)
}

func sameAIRExpr(a air.Expr, b air.Expr) bool {
	if a.Kind != b.Kind || a.Type != b.Type || a.Field != b.Field || a.Local != b.Local || a.Function != b.Function {
		return false
	}
	if a.Int != b.Int || a.Float != b.Float || a.Bool != b.Bool || a.Str != b.Str {
		return false
	}
	if (a.Target == nil) != (b.Target == nil) || len(a.Args) != len(b.Args) {
		return false
	}
	if a.Target != nil && !sameAIRExpr(*a.Target, *b.Target) {
		return false
	}
	for i := range a.Args {
		if !sameAIRExpr(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

func (l *lowerer) declareTemp(typeID air.TypeID, name string) ([]ast.Stmt, error) {
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

func (l *lowerer) lowerForeignCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.ForeignTarget != "go" {
		return loweredExpr{}, fmt.Errorf("unsupported foreign call target %q", expr.ForeignTarget)
	}
	if expr.ForeignNamespace == "" || expr.ForeignSymbol == "" {
		return loweredExpr{}, fmt.Errorf("invalid go foreign call %q::%q", expr.ForeignNamespace, expr.ForeignSymbol)
	}

	args := make([]ast.Expr, 0, len(expr.Args))
	var stmts []ast.Stmt
	for i := range expr.Args {
		arg, err := l.lowerExpr(fn, expr.Args[i])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, arg.stmts...)
		args = append(args, arg.expr)
	}
	importPath := expr.ForeignNamespace
	functionName := expr.ForeignSymbol
	pkgName := expr.ForeignQualifier
	if pkgName == "" {
		pkgName = importPath
		if slash := strings.LastIndex(pkgName, "/"); slash >= 0 {
			pkgName = pkgName[slash+1:]
		}
	}
	call := &ast.CallExpr{Fun: l.qualified(pkgName, importPath, functionName), Args: args}
	if validTypeID(l.program, expr.Type) {
		if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeResult {
			return l.lowerGoValueErrorResultCall(expr, stmts, call, info)
		}
	}
	return loweredExpr{stmts: stmts, expr: call}, nil
}

func (l *lowerer) lowerGoValueErrorResultCall(expr air.Expr, stmts []ast.Stmt, call *ast.CallExpr, result air.TypeInfo) (loweredExpr, error) {
	resultTemp := l.nextTemp()
	valueTemp := l.nextTemp()
	errTemp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	valueType, err := l.goType(result.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: valueType}}}})
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("error")}}}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	okLit := &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: ast.NewIdent(valueTemp)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
	}}
	errLit := &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(errTemp), Sel: ast.NewIdent("Error")}}},
	}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.NEQ, Y: ast.NewIdent("nil")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{errLit}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{okLit}}}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
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

func isComparisonKind(kind air.ExprKind) bool {
	switch kind {
	case air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte:
		return true
	default:
		return false
	}
}

func (l *lowerer) castEnumIntComparisonOperands(left *loweredExpr, leftTypeID air.TypeID, right *loweredExpr, rightTypeID air.TypeID) {
	leftInfo, leftOK := l.typeInfo(leftTypeID)
	rightInfo, rightOK := l.typeInfo(rightTypeID)
	if !leftOK || !rightOK {
		return
	}

	if leftInfo.Kind == air.TypeEnum && rightInfo.Kind == air.TypeInt {
		right.expr = castGoExprToType(right.expr, l.namedTypeExpr(leftInfo))
	}
	if leftInfo.Kind == air.TypeInt && rightInfo.Kind == air.TypeEnum {
		left.expr = castGoExprToType(left.expr, l.namedTypeExpr(rightInfo))
	}
}

func castGoExprToType(expr ast.Expr, typ ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: typ, Args: []ast.Expr{expr}}
}

func (l *lowerer) typeInfo(id air.TypeID) (air.TypeInfo, bool) {
	if id <= 0 || int(id) > len(l.program.Types) {
		return air.TypeInfo{}, false
	}
	return l.program.Types[id-1], true
}

func (l *lowerer) voidTypeExpr() ast.Expr {
	return &ast.StructType{Fields: &ast.FieldList{}}
}

func (l *lowerer) voidValueExpr() ast.Expr {
	return &ast.CompositeLit{Type: l.voidTypeExpr()}
}

func (l *lowerer) appendVoidValueEval(stmts []ast.Stmt, expr ast.Expr) []ast.Stmt {
	if isVoidExpr(expr) {
		return stmts
	}
	if _, ok := expr.(*ast.CallExpr); ok {
		return append(stmts, &ast.ExprStmt{X: expr})
	}
	return append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr}})
}

func (l *lowerer) materializeVoidValue(value loweredExpr) loweredExpr {
	value.stmts = l.appendVoidValueEval(value.stmts, value.expr)
	value.expr = l.voidValueExpr()
	return value
}

func (l *lowerer) zeroValueExpr(typeID air.TypeID) (ast.Expr, error) {
	if l.isVoidType(typeID) {
		return l.voidValueExpr(), nil
	}
	if !validTypeID(l.program, typeID) {
		return ast.NewIdent("nil"), nil
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeInt, air.TypeByte, air.TypeRune, air.TypeEnum:
		return &ast.BasicLit{Kind: token.INT, Value: "0"}, nil
	case air.TypeFloat:
		return &ast.BasicLit{Kind: token.FLOAT, Value: "0"}, nil
	case air.TypeBool:
		return ast.NewIdent("false"), nil
	case air.TypeStr:
		return &ast.BasicLit{Kind: token.STRING, Value: "\"\""}, nil
	case air.TypeAny, air.TypeFunction, air.TypeTraitObject:
		return ast.NewIdent("nil"), nil
	default:
		typ, err := l.goType(typeID)
		if err != nil {
			return nil, err
		}
		return &ast.CompositeLit{Type: typ}, nil
	}
}

func (l *lowerer) isMaybeType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeMaybe
}

func (l *lowerer) mapKeyValueTypes(mapTypeID air.TypeID) (air.TypeID, air.TypeID) {
	if !validTypeID(l.program, mapTypeID) {
		return air.NoType, air.NoType
	}
	info := l.program.Types[mapTypeID-1]
	if info.Kind != air.TypeMap {
		return air.NoType, air.NoType
	}
	return info.Key, info.Value
}

func (l *lowerer) lowerMapKeyArg(fn air.Function, mapTypeID air.TypeID, expr air.Expr) (loweredExpr, error) {
	keyType, _ := l.mapKeyValueTypes(mapTypeID)
	var key loweredExpr
	var err error
	if keyType != air.NoType {
		key, err = l.lowerExprWithExpectedType(fn, expr, keyType)
	} else {
		key, err = l.lowerExpr(fn, expr)
	}
	if err != nil {
		return loweredExpr{}, err
	}
	if l.isVoidType(keyType) || isVoidExpr(key.expr) {
		key = l.materializeVoidValue(key)
	}
	return key, nil
}

func (l *lowerer) isTraitObjectType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeTraitObject
}

func (l *lowerer) mutableTraitRefType(typeID air.TypeID) (ast.Expr, error) {
	if !l.isTraitObjectType(typeID) {
		return nil, fmt.Errorf("type %d is not a trait object", typeID)
	}
	traitID := l.program.Types[typeID-1].Trait
	if !validTraitID(l.program, traitID) {
		return nil, fmt.Errorf("invalid trait id %d", traitID)
	}
	if l.mutableTraitRefs != nil {
		l.mutableTraitRefs[traitID] = true
	}
	return ast.NewIdent(mutableTraitRefTypeName(l.program.Traits[traitID])), nil
}

func (l *lowerer) goParamType(param air.Param) (ast.Expr, error) {
	typ, err := l.goType(param.Type)
	if err != nil {
		return nil, err
	}
	if param.Mutable && validTypeID(l.program, param.Type) && !l.isVoidType(param.Type) {
		if l.isTraitObjectType(param.Type) {
			typ, err = l.mutableTraitRefType(param.Type)
			if err != nil {
				return nil, err
			}
		}
		return &ast.StarExpr{X: typ}, nil
	}
	return typ, nil
}

func (l *lowerer) modulePathForType(typeID air.TypeID) string {
	if validTypeID(l.program, typeID) && l.program.Types[typeID-1].ModulePath != "" {
		return l.program.Types[typeID-1].ModulePath
	}
	for _, module := range l.program.Modules {
		for _, moduleTypeID := range module.Types {
			if moduleTypeID == typeID {
				return module.Path
			}
		}
	}
	return ""
}

func (l *lowerer) goType(typeID air.TypeID) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeVoid:
		return l.voidTypeExpr(), nil
	case air.TypeInt:
		return ast.NewIdent("int"), nil
	case air.TypeByte:
		return ast.NewIdent("byte"), nil
	case air.TypeRune:
		return ast.NewIdent("rune"), nil
	case air.TypeFloat:
		return ast.NewIdent("float64"), nil
	case air.TypeBool:
		return ast.NewIdent("bool"), nil
	case air.TypeStr:
		return ast.NewIdent("string"), nil
	case air.TypeMaybe:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "Maybe"), Index: elem}, nil
	case air.TypeFunction:
		params := make([]*ast.Field, 0, len(info.Params))
		for i, paramTypeID := range info.Params {
			paramType, err := l.goType(paramTypeID)
			if err != nil {
				return nil, err
			}
			if i < len(info.ParamMutable) && info.ParamMutable[i] {
				if l.isTraitObjectType(paramTypeID) {
					paramType, err = l.mutableTraitRefType(paramTypeID)
					if err != nil {
						return nil, err
					}
				}
				paramType = &ast.StarExpr{X: paramType}
			}
			params = append(params, &ast.Field{Type: paramType})
		}
		fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
		if !l.isVoidType(info.Return) {
			returnType, err := l.goType(info.Return)
			if err != nil {
				return nil, err
			}
			fnType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
		}
		return fnType, nil
	case air.TypeResult:
		l.markRuntimeHelper("result")
		value, err := l.goType(info.Value)
		if err != nil {
			return nil, err
		}
		errType, err := l.goType(info.Error)
		if err != nil {
			return nil, err
		}
		return &ast.IndexListExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "Result"), Indices: []ast.Expr{value, errType}}, nil
	case air.TypeList:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Elt: elem}, nil
	case air.TypeChannel:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: elem}, nil
	case air.TypeReceiver:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ChanType{Dir: ast.RECV, Value: elem}, nil
	case air.TypeSender:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ChanType{Dir: ast.SEND, Value: elem}, nil
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
	case air.TypeParam:
		return ast.NewIdent(info.Name), nil
	case air.TypeStruct, air.TypeEnum:
		return l.namedTypeExpr(info), nil
	case air.TypeUnion:
		return l.namedTypeExpr(info), nil
	case air.TypeAny:
		return ast.NewIdent("any"), nil
	case air.TypeTraitObject:
		if l.usesNativeTraitInterface(typeID) {
			return l.traitInterfaceTypeExpr(l.program.Traits[info.Trait]), nil
		}
		return ast.NewIdent("any"), nil
	default:
		return nil, fmt.Errorf("unsupported Go type kind %d", info.Kind)
	}
}

func (l *lowerer) isVoidType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeVoid
}

func (l *lowerer) typeKind(typeID air.TypeID) air.TypeKind {
	if !validTypeID(l.program, typeID) {
		return air.TypeVoid
	}
	return l.program.Types[typeID-1].Kind
}

func (l *lowerer) maybeElemIsVoid(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return info.Kind == air.TypeMaybe && l.isVoidType(info.Elem)
}

func (l *lowerer) resultValueIsVoid(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return info.Kind == air.TypeResult && l.isVoidType(info.Value)
}

func (l *lowerer) resultErrorIsVoid(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return info.Kind == air.TypeResult && l.isVoidType(info.Error)
}

func (l *lowerer) resolvedLocalType(fn air.Function, local air.LocalID) air.TypeID {
	if int(local) < 0 || int(local) >= len(fn.Locals) {
		return air.NoType
	}
	typeID := fn.Locals[local].Type
	if !l.isWeakContextType(typeID) && !l.isVoidType(typeID) {
		return typeID
	}
	if inferred := l.inferLocalTypeFromBlock(fn, local, fn.Body); inferred != air.NoType {
		return inferred
	}
	if initExpr := l.findLocalInitializerExpr(fn.Body, local); initExpr != nil {
		if validTypeID(l.program, initExpr.Type) && !l.isVoidType(initExpr.Type) && !l.isWeakContextType(initExpr.Type) {
			return initExpr.Type
		}
		if inferred := l.resolveExpectedTypeFromExpr(typeID, initExpr); inferred != air.NoType {
			return inferred
		}
	}
	if fn.Body.Result != nil && fn.Body.Result.Kind == air.ExprLoadLocal && fn.Body.Result.Local == local && !l.isWeakContextType(fn.Signature.Return) {
		return fn.Signature.Return
	}
	return typeID
}

func (l *lowerer) findLocalInitializerExpr(block air.Block, local air.LocalID) *air.Expr {
	for _, stmt := range block.Stmts {
		switch stmt.Kind {
		case air.StmtLet:
			if stmt.Local == local {
				return stmt.Value
			}
		case air.StmtWhile:
			if expr := l.findLocalInitializerExpr(stmt.Body, local); expr != nil {
				return expr
			}
		}
		for _, expr := range []*air.Expr{stmt.Value, stmt.Expr, stmt.Target, stmt.Condition} {
			if nested := l.findLocalInitializerExprInExpr(expr, local); nested != nil {
				return nested
			}
		}
	}
	if nested := l.findLocalInitializerExprInExpr(block.Result, local); nested != nil {
		return nested
	}
	return nil
}

func (l *lowerer) findLocalInitializerExprInExpr(expr *air.Expr, local air.LocalID) *air.Expr {
	if expr == nil {
		return nil
	}
	for _, block := range []air.Block{expr.Body, expr.Then, expr.Else, expr.CatchAll, expr.Some, expr.None, expr.Ok, expr.Err, expr.Catch} {
		if nested := l.findLocalInitializerExpr(block, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.EnumCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.IntCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.RangeCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, c := range expr.UnionCases {
		if nested := l.findLocalInitializerExpr(c.Body, local); nested != nil {
			return nested
		}
	}
	for _, child := range []*air.Expr{expr.Target, expr.Left, expr.Right} {
		if nested := l.findLocalInitializerExprInExpr(child, local); nested != nil {
			return nested
		}
	}
	for i := range expr.Args {
		if nested := l.findLocalInitializerExprInExpr(&expr.Args[i], local); nested != nil {
			return nested
		}
	}
	return nil
}

func (l *lowerer) inferLocalTypeFromBlock(fn air.Function, local air.LocalID, block air.Block) air.TypeID {
	for _, stmt := range block.Stmts {
		switch stmt.Kind {
		case air.StmtLet, air.StmtAssign:
			if stmt.Local == local && stmt.Value != nil && !l.isWeakContextType(stmt.Value.Type) {
				return stmt.Value.Type
			}
		case air.StmtWhile:
			if inferred := l.inferLocalTypeFromBlock(fn, local, stmt.Body); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Value != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Value); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Expr != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Expr); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Target != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Target); inferred != air.NoType {
				return inferred
			}
		}
		if stmt.Condition != nil {
			if inferred := l.inferLocalTypeFromExpr(fn, local, *stmt.Condition); inferred != air.NoType {
				return inferred
			}
		}
	}
	if block.Result != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *block.Result); inferred != air.NoType {
			return inferred
		}
	}
	return air.NoType
}

func (l *lowerer) resolveExpectedTypeFromExpr(fallback air.TypeID, expr *air.Expr) air.TypeID {
	if expr == nil || !validTypeID(l.program, fallback) {
		return air.NoType
	}
	fallbackInfo := l.program.Types[fallback-1]
	if inferred := l.inferTypeFromExprShape(expr); validTypeID(l.program, inferred) {
		inferredInfo := l.program.Types[inferred-1]
		if inferredInfo.Kind == fallbackInfo.Kind || l.isVoidType(fallback) {
			return inferred
		}
	}
	if validTypeID(l.program, expr.Type) {
		exprInfo := l.program.Types[expr.Type-1]
		if exprInfo.Kind == fallbackInfo.Kind && !l.isWeakContextType(expr.Type) {
			return expr.Type
		}
	}
	for _, block := range []air.Block{expr.Body, expr.Then, expr.Else, expr.CatchAll, expr.Some, expr.None, expr.Ok, expr.Err, expr.Catch} {
		if block.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, block.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.EnumCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.IntCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.RangeCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	for _, c := range expr.UnionCases {
		if c.Body.Result != nil {
			if inferred := l.resolveExpectedTypeFromExpr(fallback, c.Body.Result); inferred != air.NoType {
				return inferred
			}
		}
	}
	if expr.Target != nil {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, expr.Target); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Left != nil {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, expr.Left); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Right != nil {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, expr.Right); inferred != air.NoType {
			return inferred
		}
	}
	for i := range expr.Args {
		if inferred := l.resolveExpectedTypeFromExpr(fallback, &expr.Args[i]); inferred != air.NoType {
			return inferred
		}
	}
	return air.NoType
}

func (l *lowerer) inferTypeFromExprShape(expr *air.Expr) air.TypeID {
	if expr == nil {
		return air.NoType
	}
	switch expr.Kind {
	case air.ExprMakeMaybeSome:
		if expr.Target != nil {
			return l.findMaybeTypeByElem(expr.Target.Type)
		}
	case air.ExprTryMaybe:
		if expr.Target != nil && validTypeID(l.program, expr.Target.Type) {
			targetType := l.program.Types[expr.Target.Type-1]
			if targetType.Kind == air.TypeMaybe {
				return targetType.Elem
			}
		}
	case air.ExprTryResult:
		if expr.Target != nil && validTypeID(l.program, expr.Target.Type) {
			targetType := l.program.Types[expr.Target.Type-1]
			if targetType.Kind == air.TypeResult {
				return targetType.Value
			}
		}
	}
	return air.NoType
}

func (l *lowerer) findMaybeTypeByElem(elem air.TypeID) air.TypeID {
	for _, info := range l.program.Types {
		if info.Kind == air.TypeMaybe && info.Elem == elem {
			return info.ID
		}
	}
	if !validTypeID(l.program, elem) {
		return air.NoType
	}
	id := air.TypeID(len(l.program.Types) + 1)
	l.program.Types = append(l.program.Types, air.TypeInfo{ID: id, Kind: air.TypeMaybe, Name: fmt.Sprintf("Maybe<%d>", elem), Elem: elem})
	return id
}

func (l *lowerer) inferLocalTypeFromExpr(fn air.Function, local air.LocalID, expr air.Expr) air.TypeID {
	if expr.Target != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *expr.Target); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Left != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *expr.Left); inferred != air.NoType {
			return inferred
		}
	}
	if expr.Right != nil {
		if inferred := l.inferLocalTypeFromExpr(fn, local, *expr.Right); inferred != air.NoType {
			return inferred
		}
	}
	for _, arg := range expr.Args {
		if inferred := l.inferLocalTypeFromExpr(fn, local, arg); inferred != air.NoType {
			return inferred
		}
	}
	for _, block := range []air.Block{expr.Body, expr.Then, expr.Else, expr.CatchAll, expr.Some, expr.None, expr.Ok, expr.Err, expr.Catch} {
		if inferred := l.inferLocalTypeFromBlock(fn, local, block); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.EnumCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.IntCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.RangeCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	for _, c := range expr.UnionCases {
		if inferred := l.inferLocalTypeFromBlock(fn, local, c.Body); inferred != air.NoType {
			return inferred
		}
	}
	return air.NoType
}

func (l *lowerer) isWeakContextType(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeList:
		return !validTypeID(l.program, info.Elem) || l.program.Types[info.Elem-1].Kind == air.TypeVoid
	case air.TypeMaybe:
		return !validTypeID(l.program, info.Elem) || l.program.Types[info.Elem-1].Kind == air.TypeVoid
	case air.TypeResult:
		return !validTypeID(l.program, info.Value) || !validTypeID(l.program, info.Error) || l.program.Types[info.Value-1].Kind == air.TypeVoid || l.program.Types[info.Error-1].Kind == air.TypeVoid
	default:
		return false
	}
}

func (l *lowerer) resolvedExprType(fn air.Function, expr air.Expr) air.TypeID {
	if expr.Kind == air.ExprLoadLocal {
		if resolved := l.resolvedLocalType(fn, expr.Local); resolved != air.NoType {
			return resolved
		}
	}
	if inferred := l.inferTypeFromExprShape(&expr); inferred != air.NoType {
		return inferred
	}
	return expr.Type
}

func (l *lowerer) lowerCallArgs(fn air.Function, rawArgs []air.Expr, params []air.Param) ([]ast.Expr, []ast.Stmt, []ast.Stmt, error) {
	args := make([]ast.Expr, 0, len(rawArgs))
	stmts := []ast.Stmt{}
	writeback := []ast.Stmt{}
	for i, arg := range rawArgs {
		var loweredArg loweredExpr
		var err error
		if i < len(params) && !params[i].Mutable {
			loweredArg, err = l.lowerExprWithExpectedType(fn, arg, params[i].Type)
		} else {
			loweredArg, err = l.lowerExpr(fn, arg)
		}
		if err != nil {
			return nil, nil, nil, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		argExpr := loweredArg.expr
		if i < len(params) && l.isVoidType(params[i].Type) {
			stmts = l.appendVoidValueEval(stmts, argExpr)
			argExpr = l.voidValueExpr()
		}
		if i < len(params) {
			var setup []ast.Stmt
			var post []ast.Stmt
			argExpr, setup, post, err = l.adaptCallArgWithStmts(fn, arg, argExpr, params[i])
			if err != nil {
				return nil, nil, nil, err
			}
			stmts = append(stmts, setup...)
			writeback = append(writeback, post...)
		}
		args = append(args, argExpr)
	}
	return args, stmts, writeback, nil
}

func (l *lowerer) finishCallWithWriteback(typeID air.TypeID, stmts []ast.Stmt, call ast.Expr, writeback []ast.Stmt) (loweredExpr, error) {
	if len(writeback) == 0 {
		return loweredExpr{stmts: stmts, expr: call}, nil
	}
	if l.isVoidType(typeID) {
		stmts = append(stmts, &ast.ExprStmt{X: call})
		stmts = append(stmts, writeback...)
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	}
	resultTemp := l.nextTemp()
	resultType, err := l.goType(typeID)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts,
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: resultType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	)
	stmts = append(stmts, writeback...)
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
}

func (l *lowerer) adaptCallArg(fn air.Function, arg air.Expr, argExpr ast.Expr, param air.Param) ast.Expr {
	if !param.Mutable || !validTypeID(l.program, param.Type) || l.isVoidType(param.Type) {
		return argExpr
	}
	return l.mutableReferenceArg(fn, arg, argExpr)
}

func (l *lowerer) adaptCallArgWithStmts(fn air.Function, arg air.Expr, argExpr ast.Expr, param air.Param) (ast.Expr, []ast.Stmt, []ast.Stmt, error) {
	if !param.Mutable || !validTypeID(l.program, param.Type) || l.isVoidType(param.Type) {
		return argExpr, nil, nil, nil
	}
	if adapted, setup, writeback, ok, err := l.mutableTraitObjectArg(fn, arg, argExpr, param); ok || err != nil {
		return adapted, setup, writeback, err
	}
	return l.mutableReferenceArg(fn, arg, argExpr), nil, nil, nil
}

func (l *lowerer) mutableTraitObjectArg(fn air.Function, arg air.Expr, argExpr ast.Expr, param air.Param) (ast.Expr, []ast.Stmt, []ast.Stmt, bool, error) {
	if !l.isTraitObjectType(param.Type) {
		return nil, nil, nil, false, nil
	}
	if ref, setup, ok, err := l.mutableTraitRefPointerExpr(fn, arg); ok || err != nil {
		return ref, setup, nil, ok, err
	}
	if arg.Kind == air.ExprTraitUpcast && arg.Target != nil {
		place, setup, ok, err := l.mutableTraitUpcastPlace(fn, *arg.Target)
		if err != nil {
			return nil, nil, nil, true, err
		}
		if !ok {
			return nil, nil, nil, true, fmt.Errorf("mutable trait object argument is not an assignable place")
		}
		ref, err := l.mutableTraitForwarderExpr(arg, place, param.Type)
		if err != nil {
			return nil, nil, nil, true, err
		}
		return &ast.UnaryExpr{Op: token.AND, X: ref}, setup, nil, true, nil
	}
	if l.isTraitObjectType(arg.Type) {
		place, setup, ok, err := l.mutableTraitUpcastPlace(fn, arg)
		if err != nil {
			return nil, nil, nil, true, err
		}
		if !ok {
			return nil, nil, nil, true, fmt.Errorf("mutable trait object argument is not an assignable place")
		}
		ref, err := l.mutableTraitAnyForwarderExpr(place, param.Type)
		if err != nil {
			return nil, nil, nil, true, err
		}
		return &ast.UnaryExpr{Op: token.AND, X: ref}, setup, nil, true, nil
	}
	return nil, nil, nil, false, nil
}

func (l *lowerer) implRequiresPointerReceiver(implID air.ImplID) bool {
	if implID < 0 || int(implID) >= len(l.program.Impls) {
		return false
	}
	for _, methodID := range l.program.Impls[implID].Methods {
		if !validFunctionID(l.program, methodID) {
			continue
		}
		methodFn := l.program.Functions[methodID]
		if len(methodFn.Signature.Params) > 0 && methodFn.Signature.Params[0].Mutable {
			return true
		}
	}
	return false
}

func (l *lowerer) mutableTraitRefPointerExpr(fn air.Function, arg air.Expr) (ast.Expr, []ast.Stmt, bool, error) {
	if !l.isTraitObjectType(arg.Type) {
		return nil, nil, false, nil
	}
	switch arg.Kind {
	case air.ExprLoadLocal:
		if l.localIsPointerParam(fn, arg.Local) {
			return ast.NewIdent(l.localName(fn, arg.Local)), nil, true, nil
		}
	case air.ExprGetField:
		if arg.Target == nil || !validTypeID(l.program, arg.Target.Type) {
			return nil, nil, false, nil
		}
		targetType := l.program.Types[arg.Target.Type-1]
		if targetType.Kind != air.TypeStruct || arg.Field < 0 || arg.Field >= len(targetType.Fields) {
			return nil, nil, false, nil
		}
		field := targetType.Fields[arg.Field]
		if !field.Mutable || !l.isTraitObjectType(field.Type) {
			return nil, nil, false, nil
		}
		target, err := l.lowerExpr(fn, *arg.Target)
		if err != nil {
			return nil, nil, false, err
		}
		return &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(l.goFieldName(targetType, field.Name))}, target.stmts, true, nil
	}
	return nil, nil, false, nil
}

func (l *lowerer) mutableTraitAssignValueExpr(fn air.Function, arg air.Expr, argExpr ast.Expr, traitTypeID air.TypeID) ast.Expr {
	if l.isTraitObjectType(arg.Type) && l.exprIsMutableReference(fn, arg) {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: argExpr, Sel: ast.NewIdent(l.mutableTraitLoadFieldNameForType(traitTypeID))}}
	}
	return argExpr
}

func (l *lowerer) mutableTraitLoadFieldNameForType(typeID air.TypeID) string {
	if !l.isTraitObjectType(typeID) {
		return ""
	}
	traitID := l.program.Types[typeID-1].Trait
	if !validTraitID(l.program, traitID) {
		return ""
	}
	return mutableTraitLoadFieldName(l.program.Traits[traitID])
}

func (l *lowerer) mutableTraitAssignFieldNameForType(typeID air.TypeID) string {
	if !l.isTraitObjectType(typeID) {
		return ""
	}
	traitID := l.program.Types[typeID-1].Trait
	if !validTraitID(l.program, traitID) {
		return ""
	}
	return mutableTraitAssignFieldName(l.program.Traits[traitID])
}

func (l *lowerer) mutableTraitForwarderExpr(upcast air.Expr, place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
	if !l.isTraitObjectType(traitTypeID) {
		return nil, fmt.Errorf("type %d is not a trait object", traitTypeID)
	}
	traitID := l.program.Types[traitTypeID-1].Trait
	if !validTraitID(l.program, traitID) {
		return nil, fmt.Errorf("invalid trait id %d", traitID)
	}
	if !validImplID(l.program, upcast.Impl) {
		return nil, fmt.Errorf("invalid impl id %d", upcast.Impl)
	}
	if upcast.Target == nil || !validTypeID(l.program, upcast.Target.Type) {
		return nil, fmt.Errorf("mutable trait forwarder missing concrete target")
	}
	trait := l.program.Traits[traitID]
	impl := l.program.Impls[upcast.Impl]
	if impl.Trait != traitID {
		return nil, fmt.Errorf("trait upcast impl %d has trait %d, want %d", upcast.Impl, impl.Trait, traitID)
	}
	concreteType, err := l.goType(upcast.Target.Type)
	if err != nil {
		return nil, err
	}
	elts := []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitLoadFieldName(trait)), Value: mutableTraitLoadFuncLit(place)},
		&ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitAssignFieldName(trait)), Value: mutableTraitAssignFuncLit(place, concreteType, trait)},
	}
	for i, traitMethod := range trait.Methods {
		if i >= len(impl.Methods) || !validFunctionID(l.program, impl.Methods[i]) {
			return nil, fmt.Errorf("impl %d missing method %d for trait %s", upcast.Impl, i, trait.Name)
		}
		fieldValue, err := l.mutableTraitForwarderMethodExpr(traitMethod, l.program.Functions[impl.Methods[i]], place)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitMethodFieldName(trait.ID, i)), Value: fieldValue})
	}
	refType, err := l.mutableTraitRefType(traitTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CompositeLit{Type: refType, Elts: elts}, nil
}

func (l *lowerer) mutableTraitAnyForwarderExpr(place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
	if !l.isTraitObjectType(traitTypeID) {
		return nil, fmt.Errorf("type %d is not a trait object", traitTypeID)
	}
	traitID := l.program.Types[traitTypeID-1].Trait
	if !validTraitID(l.program, traitID) {
		return nil, fmt.Errorf("invalid trait id %d", traitID)
	}
	trait := l.program.Traits[traitID]
	assignFunc, err := l.mutableTraitAnyAssignFuncLit(place, traitTypeID)
	if err != nil {
		return nil, err
	}
	elts := []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitLoadFieldName(trait)), Value: mutableTraitLoadFuncLit(place)},
		&ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitAssignFieldName(trait)), Value: assignFunc},
	}
	for i, method := range trait.Methods {
		fieldValue, err := l.mutableTraitAnyForwarderMethodExpr(trait, i, method, place, traitTypeID)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitMethodFieldName(trait.ID, i)), Value: fieldValue})
	}
	refType, err := l.mutableTraitRefType(traitTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CompositeLit{Type: refType, Elts: elts}, nil
}

func (l *lowerer) mutableTraitAnyForwarderMethodExpr(trait air.Trait, methodIndex int, traitMethod air.TraitMethod, place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
	fnTypeExpr, err := l.mutableTraitMethodFuncType(traitMethod)
	if err != nil {
		return nil, err
	}
	fnType := fnTypeExpr.(*ast.FuncType)
	switchVar := l.nextTemp()
	switchVarExpr := ast.NewIdent(switchVar)
	cases := []ast.Stmt{
		l.mutableTraitForwardingCase(traitMethod, mutableTraitMethodFieldName(trait.ID, methodIndex), switchVarExpr, ast.NewIdent(mutableTraitRefTypeName(trait))),
		l.mutableTraitForwardingCase(traitMethod, mutableTraitMethodFieldName(trait.ID, methodIndex), switchVarExpr, &ast.StarExpr{X: ast.NewIdent(mutableTraitRefTypeName(trait))}),
	}
	for _, impl := range l.program.Impls {
		if impl.Trait != trait.ID || methodIndex >= len(impl.Methods) || !validTypeID(l.program, impl.ForType) || !validFunctionID(l.program, impl.Methods[methodIndex]) {
			continue
		}
		methodFn := l.program.Functions[impl.Methods[methodIndex]]
		cases = append(cases, l.mutableTraitImplForwardingCase(traitMethod, methodFn, impl.ForType, switchVarExpr, place))
	}
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: "\"unsupported trait object dispatch\""}}}}}})
	switchTarget := place
	if l.usesNativeTraitInterface(traitTypeID) {
		switchTarget = &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{place}}
	}
	body := []ast.Stmt{&ast.TypeSwitchStmt{Assign: &ast.AssignStmt{Lhs: []ast.Expr{switchVarExpr}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: switchTarget}}}, Body: &ast.BlockStmt{List: cases}}}
	return &ast.FuncLit{Type: fnType, Body: &ast.BlockStmt{List: body}}, nil
}

func (l *lowerer) mutableTraitForwardingCase(traitMethod air.TraitMethod, methodField string, receiver ast.Expr, caseType ast.Expr) *ast.CaseClause {
	args := make([]ast.Expr, 0, len(traitMethod.Signature.Params))
	for i := range traitMethod.Signature.Params {
		args = append(args, ast.NewIdent(fmt.Sprintf("arg%d", i)))
	}
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: receiver, Sel: ast.NewIdent(methodField)}, Args: args}
	body := []ast.Stmt{}
	if l.isVoidType(traitMethod.Signature.Return) {
		body = append(body, &ast.ExprStmt{X: call})
	} else {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	return &ast.CaseClause{List: []ast.Expr{caseType}, Body: body}
}

func (l *lowerer) mutableTraitImplForwardingCase(traitMethod air.TraitMethod, methodFn air.Function, implType air.TypeID, receiver ast.Expr, place ast.Expr) *ast.CaseClause {
	callReceiver := receiver
	writeback := false
	if len(methodFn.Signature.Params) > 0 {
		receiverParam := methodFn.Signature.Params[0]
		if receiverParam.Mutable && validTypeID(l.program, receiverParam.Type) && l.program.Types[receiverParam.Type-1].Kind == air.TypeStruct {
			callReceiver = &ast.UnaryExpr{Op: token.AND, X: receiver}
			writeback = true
		}
	}
	args := []ast.Expr{callReceiver}
	for i := range traitMethod.Signature.Params {
		args = append(args, ast.NewIdent(fmt.Sprintf("arg%d", i)))
	}
	call := &ast.CallExpr{Fun: l.functionExpr(methodFn), Args: args}
	body := []ast.Stmt{}
	if l.isVoidType(traitMethod.Signature.Return) {
		body = append(body, &ast.ExprStmt{X: call})
		if writeback {
			body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{place}, Tok: token.ASSIGN, Rhs: []ast.Expr{receiver}})
		}
	} else if writeback {
		resultTemp := l.nextTemp()
		body = append(body,
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			&ast.AssignStmt{Lhs: []ast.Expr{place}, Tok: token.ASSIGN, Rhs: []ast.Expr{receiver}},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(resultTemp)}},
		)
	} else {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	return &ast.CaseClause{List: []ast.Expr{mustTypeExpr(l, implType)}, Body: body}
}

func mutableTraitLoadFuncLit(place ast.Expr) ast.Expr {
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{place}}}},
	}
}

func mutableTraitAssignFuncLit(place ast.Expr, targetType ast.Expr, trait air.Trait) ast.Expr {
	forwarded := ast.NewIdent("forwarded")
	loadAssignedValue := func(receiver ast.Expr) ast.Expr {
		return &ast.TypeAssertExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: receiver, Sel: ast.NewIdent(mutableTraitLoadFieldName(trait))}}, Type: targetType}
	}
	assignLoaded := func(receiver ast.Expr) []ast.Stmt {
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{place}, Tok: token.ASSIGN, Rhs: []ast.Expr{loadAssignedValue(receiver)}}}
	}
	assignValue := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{place}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("value"), Type: targetType}}}}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("value")}, Type: ast.NewIdent("any")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.TypeSwitchStmt{
			Assign: &ast.AssignStmt{Lhs: []ast.Expr{forwarded}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent("value")}}},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.CaseClause{List: []ast.Expr{ast.NewIdent(mutableTraitRefTypeName(trait))}, Body: assignLoaded(forwarded)},
				&ast.CaseClause{List: []ast.Expr{&ast.StarExpr{X: ast.NewIdent(mutableTraitRefTypeName(trait))}}, Body: assignLoaded(forwarded)},
				&ast.CaseClause{Body: assignValue},
			}},
		}}},
	}
}

func (l *lowerer) mutableTraitAnyAssignFuncLit(place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
	value := ast.Expr(ast.NewIdent("value"))
	if l.usesNativeTraitInterface(traitTypeID) {
		traitType, err := l.goType(traitTypeID)
		if err != nil {
			return nil, err
		}
		value = &ast.TypeAssertExpr{X: value, Type: traitType}
	}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("value")}, Type: ast.NewIdent("any")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{place}, Tok: token.ASSIGN, Rhs: []ast.Expr{value}}}},
	}, nil
}

func (l *lowerer) mutableTraitForwarderMethodExpr(traitMethod air.TraitMethod, methodFn air.Function, place ast.Expr) (ast.Expr, error) {
	fnTypeExpr, err := l.mutableTraitMethodFuncType(traitMethod)
	if err != nil {
		return nil, err
	}
	fnType := fnTypeExpr.(*ast.FuncType)
	args := []ast.Expr{}
	if len(methodFn.Signature.Params) > 0 {
		receiver := place
		receiverParam := methodFn.Signature.Params[0]
		if receiverParam.Mutable && validTypeID(l.program, receiverParam.Type) && l.program.Types[receiverParam.Type-1].Kind == air.TypeStruct {
			receiver = addressOfPlace(place)
		}
		args = append(args, receiver)
	}
	for i := range traitMethod.Signature.Params {
		args = append(args, ast.NewIdent(fmt.Sprintf("arg%d", i)))
	}
	call := &ast.CallExpr{Fun: l.functionExpr(methodFn), Args: args}
	body := []ast.Stmt{}
	if l.isVoidType(traitMethod.Signature.Return) {
		body = append(body, &ast.ExprStmt{X: call})
	} else {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	return &ast.FuncLit{Type: fnType, Body: &ast.BlockStmt{List: body}}, nil
}

func addressOfPlace(place ast.Expr) ast.Expr {
	if star, ok := place.(*ast.StarExpr); ok {
		return star.X
	}
	return &ast.UnaryExpr{Op: token.AND, X: place}
}

func (l *lowerer) mutableTraitUpcastPlace(fn air.Function, arg air.Expr) (ast.Expr, []ast.Stmt, bool, error) {
	switch arg.Kind {
	case air.ExprLoadLocal:
		return l.localAssignExpr(fn, arg.Local), nil, true, nil
	case air.ExprGetField:
		if arg.Target == nil || !validTypeID(l.program, arg.Target.Type) {
			return nil, nil, false, nil
		}
		targetPlace, setup, ok, err := l.mutableTraitUpcastPlace(fn, *arg.Target)
		if err != nil || !ok {
			return nil, nil, ok, err
		}
		targetType := l.program.Types[arg.Target.Type-1]
		if targetType.Kind != air.TypeStruct || arg.Field < 0 || arg.Field >= len(targetType.Fields) {
			return nil, nil, false, nil
		}
		field := targetType.Fields[arg.Field]
		fieldTarget := ast.Expr(&ast.SelectorExpr{X: targetPlace, Sel: ast.NewIdent(l.goFieldName(targetType, field.Name))})
		if field.Mutable {
			fieldTarget = &ast.StarExpr{X: fieldTarget}
		}
		return fieldTarget, setup, true, nil
	default:
		return nil, nil, false, nil
	}
}

func (l *lowerer) mutableReferenceArg(fn air.Function, arg air.Expr, argExpr ast.Expr) ast.Expr {
	if arg.Kind == air.ExprLoadLocal && l.localIsPointerParam(fn, arg.Local) {
		return ast.NewIdent(l.localName(fn, arg.Local))
	}
	if arg.Kind == air.ExprGetField {
		if fieldExpr, ok := l.mutableFieldReferenceExpr(fn, arg); ok {
			return fieldExpr
		}
	}
	return &ast.UnaryExpr{Op: token.AND, X: argExpr}
}

func (l *lowerer) mutableFieldReferenceExpr(fn air.Function, arg air.Expr) (ast.Expr, bool) {
	if arg.Target == nil || !validTypeID(l.program, arg.Target.Type) {
		return nil, false
	}
	targetType := l.program.Types[arg.Target.Type-1]
	if targetType.Kind != air.TypeStruct || arg.Field < 0 || arg.Field >= len(targetType.Fields) {
		return nil, false
	}
	field := targetType.Fields[arg.Field]
	if !field.Mutable {
		return nil, false
	}
	target, err := l.lowerExpr(fn, *arg.Target)
	if err != nil {
		return nil, false
	}
	return &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(l.goFieldName(targetType, field.Name))}, true
}

func (l *lowerer) localValueExpr(fn air.Function, local air.LocalID) ast.Expr {
	name := ast.Expr(ast.NewIdent(l.localName(fn, local)))
	if l.localIsPointerParam(fn, local) {
		return &ast.StarExpr{X: name}
	}
	return name
}

func (l *lowerer) localAssignExpr(fn air.Function, local air.LocalID) ast.Expr {
	return l.localValueExpr(fn, local)
}

func (l *lowerer) localIsPointerParam(fn air.Function, local air.LocalID) bool {
	idx := int(local)
	if idx >= 0 && idx < len(fn.Signature.Params) {
		param := fn.Signature.Params[idx]
		return param.Mutable && validTypeID(l.program, param.Type) && !l.isVoidType(param.Type)
	}
	for _, capture := range fn.Captures {
		if capture.Local != local || idx < 0 || idx >= len(fn.Locals) {
			continue
		}
		captured := fn.Locals[idx]
		return captured.Mutable && validTypeID(l.program, captured.Type) && !l.isVoidType(captured.Type)
	}
	return false
}

func (l *lowerer) qualified(alias string, importPath string, name string) ast.Expr {
	alias = l.registerImport(alias, importPath)
	return &ast.SelectorExpr{X: ast.NewIdent(alias), Sel: ast.NewIdent(name)}
}

func (l *lowerer) registerImport(alias string, importPath string) string {
	if alias == "" || importPath == "" {
		return alias
	}
	if l.currentImports == nil {
		l.currentImports = map[string]string{}
	}
	if existing, ok := l.currentImports[alias]; ok && existing == importPath {
		return alias
	}
	chosen := alias
	for i := 1; ; i++ {
		if l.importAliasAvailable(chosen, importPath) {
			l.currentImports[chosen] = importPath
			if l.reservedGoIdentifiers != nil {
				l.reservedGoIdentifiers[chosen] = true
			}
			return chosen
		}
		chosen = fmt.Sprintf("%s_%d", alias, i)
	}
}

func (l *lowerer) importAliasAvailable(alias string, importPath string) bool {
	if existing, ok := l.currentImports[alias]; ok {
		return existing == importPath
	}
	if fixedPath, ok := generatedImportAliasPath(alias); ok && fixedPath != importPath {
		return false
	}
	if l.reservedGoIdentifiers == nil && l.program != nil {
		l.reservedGoIdentifiers = l.buildReservedGoIdentifiers()
	}
	if l.reservedGoIdentifiers[alias] && !l.aliasReservedForImport(alias, importPath) {
		return false
	}
	return !l.importAliasCollidesWithTopLevel(alias)
}

func (l *lowerer) aliasReservedForImport(alias string, importPath string) bool {
	return false
}

func (l *lowerer) importAliasCollidesWithTopLevel(alias string) bool {
	if l.program == nil {
		return false
	}
	if !l.useModulePackages {
		return l.importAliasCollidesWithProgramTopLevel(alias)
	}
	if l.currentModule < 0 || int(l.currentModule) >= len(l.program.Modules) {
		return false
	}
	return l.importAliasCollidesWithModuleTopLevel(alias, l.currentModule)
}

func (l *lowerer) importAliasCollidesWithProgramTopLevel(alias string) bool {
	for _, typ := range l.program.Types {
		if l.typeTopLevelNameCollidesWithImportAlias(typ, alias) {
			return true
		}
	}
	for _, global := range l.program.Globals {
		if globalName(l.program, global) == alias {
			return true
		}
	}
	for _, fn := range l.program.Functions {
		if functionName(l.program, fn) == alias {
			return true
		}
	}
	for _, trait := range l.program.Traits {
		if l.traitInterfaceTypeName(trait) == alias {
			return true
		}
	}
	return false
}

func (l *lowerer) importAliasCollidesWithModuleTopLevel(alias string, moduleID air.ModuleID) bool {
	for _, typ := range l.typesForModule(moduleID, moduleID) {
		if l.typeTopLevelNameCollidesWithImportAlias(typ, alias) {
			return true
		}
	}
	for _, globalID := range l.program.Modules[moduleID].Globals {
		if globalID >= 0 && int(globalID) < len(l.program.Globals) && globalName(l.program, l.program.Globals[globalID]) == alias {
			return true
		}
	}
	for _, functionID := range l.functionsForModule(moduleID) {
		if validFunctionID(l.program, functionID) && functionName(l.program, l.program.Functions[functionID]) == alias {
			return true
		}
	}
	for _, trait := range l.program.Traits {
		owner, ok := l.ownerModuleForTrait(trait.ID)
		if ok && owner == moduleID && l.traitInterfaceTypeName(trait) == alias {
			return true
		}
	}
	return false
}

func (l *lowerer) typeTopLevelNameCollidesWithImportAlias(typ air.TypeInfo, alias string) bool {
	if typeName(l.program, typ) == alias {
		return true
	}
	for _, variant := range typ.Variants {
		if enumVariantName(l.program, typ, variant) == alias {
			return true
		}
	}
	return false
}

func (l *lowerer) toStringExpr(typeID air.TypeID, expr ast.Expr) ast.Expr {
	if validTypeID(l.program, typeID) {
		switch l.program.Types[typeID-1].Kind {
		case air.TypeFloat:
			return &ast.CallExpr{Fun: l.qualified("strconv", "strconv", "FormatFloat"), Args: []ast.Expr{expr, &ast.BasicLit{Kind: token.CHAR, Value: "'f'"}, &ast.BasicLit{Kind: token.INT, Value: "2"}, &ast.BasicLit{Kind: token.INT, Value: "64"}}}
		case air.TypeRune:
			return &ast.CallExpr{Fun: ast.NewIdent("string"), Args: []ast.Expr{expr}}
		}
	}
	return &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{expr}}
}

func (l *lowerer) lowerUnionWrap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("union wrap missing target")
	}
	if !validTypeID(l.program, expr.Type) {
		return loweredExpr{}, fmt.Errorf("invalid union type id %d", expr.Type)
	}
	unionType := l.program.Types[expr.Type-1]
	if unionType.Kind != air.TypeUnion {
		return loweredExpr{}, fmt.Errorf("union wrap with non-union type %s", unionType.Name)
	}
	fieldName := ""
	memberType := air.NoType
	for _, member := range unionType.Members {
		if member.Tag == expr.Tag {
			fieldName = unionMemberFieldName(unionType, member)
			memberType = member.Type
			break
		}
	}
	if fieldName == "" {
		return loweredExpr{}, fmt.Errorf("invalid union tag %d for %s", expr.Tag, unionType.Name)
	}
	var target loweredExpr
	var err error
	if memberType != air.NoType {
		target, err = l.lowerExprWithExpectedType(fn, *expr.Target, memberType)
	} else {
		target, err = l.lowerExpr(fn, *expr.Target)
	}
	if err != nil {
		return loweredExpr{}, err
	}
	fieldValue := target.expr
	if validTypeID(l.program, memberType) && l.program.Types[memberType-1].Kind == air.TypeVoid {
		target = l.materializeVoidValue(target)
		fieldValue = target.expr
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: l.compositeTypeExpr(unionType), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(unionTagFieldName(unionType)), Value: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", expr.Tag)}},
		&ast.KeyValueExpr{Key: ast.NewIdent(fieldName), Value: fieldValue},
	}}}, nil
}

func (l *lowerer) lowerMatchUnion(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("union match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("invalid union target type %d", expr.Target.Type)
	}
	unionType := l.program.Types[expr.Target.Type-1]
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.UnionCases)+1)
	for _, unionCase := range expr.UnionCases {
		fieldName := ""
		for _, member := range unionType.Members {
			if member.Tag == unionCase.Tag {
				fieldName = unionMemberFieldName(unionType, member)
				break
			}
		}
		if fieldName == "" {
			return loweredExpr{}, fmt.Errorf("invalid union case tag %d", unionCase.Tag)
		}
		localName := l.localName(fn, unionCase.Local)
		l.declaredLocals[unionCase.Local] = true
		bind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(localName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(fieldName)}}}
		body, err := l.lowerValueBlock(fn, unionCase.Body, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		body = append([]ast.Stmt{bind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(localName)}}}, body...)
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", unionCase.Tag)}}, Body: body})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(unionTagFieldName(unionType))}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchInt(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("int match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultTypeID := expr.Type
	if l.isWeakContextType(resultTypeID) || l.isVoidType(resultTypeID) {
		for _, intCase := range expr.IntCases {
			if intCase.Body.Result != nil && intCase.Body.Result.Kind == air.ExprMakeMaybeSome && intCase.Body.Result.Target != nil {
				if inferred := l.findMaybeTypeByElem(intCase.Body.Result.Target.Type); inferred != air.NoType {
					resultTypeID = inferred
					break
				}
			}
		}
		if resultTypeID == expr.Type {
			for _, rangeCase := range expr.RangeCases {
				if rangeCase.Body.Result != nil && rangeCase.Body.Result.Kind == air.ExprMakeMaybeSome && rangeCase.Body.Result.Target != nil {
					if inferred := l.findMaybeTypeByElem(rangeCase.Body.Result.Target.Type); inferred != air.NoType {
						resultTypeID = inferred
						break
					}
				}
			}
		}
		if resultTypeID == expr.Type {
			if inferred := l.resolveExpectedTypeFromExpr(resultTypeID, &expr); inferred != air.NoType {
				resultTypeID = inferred
			}
		}
	}
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.IntCases)+len(expr.RangeCases)+1)
	for _, intCase := range expr.IntCases {
		body, err := l.lowerValueBlock(fn, intCase.Body, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{&ast.BinaryExpr{X: target.expr, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", intCase.Value)}}}, Body: body})
	}
	for _, rangeCase := range expr.RangeCases {
		body, err := l.lowerValueBlock(fn, rangeCase.Body, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cond := &ast.BinaryExpr{X: &ast.BinaryExpr{X: target.expr, Op: token.GEQ, Y: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", rangeCase.Start)}}, Op: token.LAND, Y: &ast.BinaryExpr{X: target.expr, Op: token.LEQ, Y: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", rangeCase.End)}}}
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{cond}, Body: body})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: ast.NewIdent("true"), Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchStr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("str match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultTypeID := expr.Type
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.StrCases)+1)
	for _, strCase := range expr.StrCases {
		body, err := l.lowerValueBlock(fn, strCase.Body, resultTypeID, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(strCase.Value)}}, Body: body})
	}
	body, err := l.lowerValueBlock(fn, expr.CatchAll, resultTypeID, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	cases = append(cases, &ast.CaseClause{Body: body})
	stmts = append(stmts, &ast.SwitchStmt{Tag: target.expr, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchEnum(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("enum match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	cases := make([]ast.Stmt, 0, len(expr.EnumCases)+1)
	for _, enumCase := range expr.EnumCases {
		body, err := l.lowerValueBlock(fn, enumCase.Body, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", enumCase.Discriminant)}},
			Body: body,
		})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: target.expr, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeExpect(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe expect missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe expect expects one argument")
	}
	message, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Target.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	stmts := append(target.stmts, message.stmts...)
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	if l.isVoidType(expr.Type) {
		stmts = append(stmts, &ast.IfStmt{
			Cond: l.maybeIsSomeExpr(resultExpr),
			Body: &ast.BlockStmt{},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{message.expr}}}}},
		})
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(resultExpr),
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(resultExpr)}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{message.expr}}}}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerMaybeIsNone(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe is_none missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: l.maybeIsNoneExpr(target.expr)}, nil
}

func (l *lowerer) lowerMaybeIsSome(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe is_some missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: l.maybeIsSomeExpr(target.expr)}, nil
}

func (l *lowerer) lowerMaybeOr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe or expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	defaultValue, err := l.lowerExprWithExpectedType(fn, expr.Args[0], expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	stmts := append(target.stmts, defaultValue.stmts...)
	defaultExpr := defaultValue.expr
	if l.isVoidType(expr.Type) || isVoidExpr(defaultExpr) {
		stmts = l.appendVoidValueEval(stmts, defaultExpr)
		defaultExpr = l.voidValueExpr()
	}
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(targetExpr),
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{defaultExpr}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultOr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result or expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	defaultValue, err := l.lowerExprWithExpectedType(fn, expr.Args[0], expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	stmts := append(target.stmts, defaultValue.stmts...)
	defaultExpr := defaultValue.expr
	if l.isVoidType(expr.Type) || isVoidExpr(defaultExpr) {
		stmts = l.appendVoidValueEval(stmts, defaultExpr)
		defaultExpr = l.voidValueExpr()
	}
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{defaultExpr}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeMap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe map expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{l.maybeValueExpr(targetExpr)}}
	var valueExpr ast.Expr = call
	var someBody []ast.Stmt
	if l.maybeElemIsVoid(expr.Type) || isVoidExpr(call) {
		valueExpr = l.voidValueExpr()
		someBody = l.appendVoidValueEval(someBody, call)
	}
	someExpr, err := l.maybeSomeExpr(expr.Type, valueExpr)
	if err != nil {
		return loweredExpr{}, err
	}
	someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}})
	noneExpr, err := l.maybeNoneExpr(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(targetExpr),
		Body: &ast.BlockStmt{List: someBody},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeAndThen(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe and_then expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{l.maybeValueExpr(targetExpr)}}
	noneExpr, err := l.maybeNoneExpr(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(targetExpr),
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultIsOk(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result is_ok missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Ok")}}, nil
}

func (l *lowerer) lowerResultIsErr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result is_err missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.NOT, X: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Ok")}}}, nil
}

func (l *lowerer) lowerResultMap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result map expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	var valueExpr ast.Expr = call
	var okBody []ast.Stmt
	if l.resultValueIsVoid(expr.Type) || isVoidExpr(call) {
		valueExpr = l.voidValueExpr()
		okBody = l.appendVoidValueEval(okBody, call)
	}
	okBody = append(okBody, &ast.AssignStmt{
		Lhs: []ast.Expr{resultExpr},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
			&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
		}}},
	})
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: okBody},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}},
				}}},
			},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultMapErr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result map_err expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}}}
	var errExpr ast.Expr = call
	var errBody []ast.Stmt
	if l.resultErrorIsVoid(expr.Type) || isVoidExpr(call) {
		errExpr = l.voidValueExpr()
		errBody = l.appendVoidValueEval(errBody, call)
	}
	errBody = append(errBody, &ast.AssignStmt{
		Lhs: []ast.Expr{resultExpr},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		}}},
	})
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}},
					&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: ast.NewIdent("true")},
				}}},
			},
		}},
		Else: &ast.BlockStmt{List: errBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultAndThen(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result and_then expects target and callback")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	callback, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
		}},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}},
				}}},
			},
		}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchResult(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	okName := l.localName(fn, expr.OkLocal)
	errName := l.localName(fn, expr.ErrLocal)
	l.declaredLocals[expr.OkLocal] = true
	l.declaredLocals[expr.ErrLocal] = true
	okBind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}}
	errBind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}}}
	okBody, err := l.lowerValueBlock(fn, expr.Ok, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	okBody = append([]ast.Stmt{okBind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(okName)}}}, okBody...)
	errBody, err := l.lowerValueBlock(fn, expr.Err, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	errBody = append([]ast.Stmt{errBind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(errName)}}}, errBody...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: okBody},
		Else: &ast.BlockStmt{List: errBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerResultExpect(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result expect missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("result expect expects one argument")
	}
	message, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Target.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := ast.NewIdent(resultTemp)
	panicMsg := &ast.BinaryExpr{X: message.expr, Op: token.ADD, Y: &ast.BinaryExpr{X: &ast.BasicLit{Kind: token.STRING, Value: `": "`}, Op: token.ADD, Y: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Err")}}}}}
	stmts := append(target.stmts, message.stmts...)
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	if l.isVoidType(expr.Type) {
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Ok")},
			Body: &ast.BlockStmt{},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{panicMsg}}}}},
		})
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: ast.NewIdent("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{panicMsg}}}}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerTryResult(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("try result missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(expr.Target.Type, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var resultExpr ast.Expr = ast.NewIdent("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		resultExpr = ast.NewIdent(temp)
		assignTarget = resultExpr
	}
	okBody := []ast.Stmt{}
	if assignTarget != nil {
		okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{assignTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}})
		if expr.HasCatch {
			okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignTarget}})
		}
	} else {
		okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Value")}}})
	}
	var elseBody []ast.Stmt
	if expr.HasCatch {
		var catchDecls []ast.Stmt
		var catchTarget ast.Expr
		if !l.isVoidType(fn.Signature.Return) {
			catchTargetName := l.nextTemp()
			var err error
			catchDecls, err = l.declareTemp(fn.Signature.Return, catchTargetName)
			if err != nil {
				return loweredExpr{}, err
			}
			catchTarget = ast.NewIdent(catchTargetName)
		}
		errName := l.localName(fn, expr.CatchLocal)
		l.declaredLocals[expr.CatchLocal] = true
		errBind := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}}}
		catchBody, err := l.lowerValueBlock(fn, expr.Catch, fn.Signature.Return, catchTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		elseBody = append(catchDecls, errBind, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(errName)}})
		elseBody = append(elseBody, catchBody...)
		if !l.isVoidType(fn.Signature.Return) {
			elseBody = append(elseBody, &ast.ReturnStmt{Results: []ast.Expr{catchTarget}})
		} else {
			elseBody = append(elseBody, &ast.ReturnStmt{})
		}
	} else {
		returnExpr := ast.Expr(targetExpr)
		if fn.Signature.Return != expr.Target.Type {
			returnType, err := l.goType(fn.Signature.Return)
			if err != nil {
				return loweredExpr{}, err
			}
			returnExpr = &ast.CompositeLit{Type: returnType, Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Err")}},
			}}
		}
		elseBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{returnExpr}}}
	}
	stmts = append(stmts, &ast.IfStmt{Cond: &ast.SelectorExpr{X: targetExpr, Sel: ast.NewIdent("Ok")}, Body: &ast.BlockStmt{List: okBody}, Else: &ast.BlockStmt{List: elseBody}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerTryMaybe(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("try maybe missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTypeID := l.resolvedExprType(fn, *expr.Target)
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(targetTypeID, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	stmts := append(target.stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	resultTypeID := expr.Type
	if l.isVoidType(resultTypeID) {
		if inferred := l.inferTypeFromExprShape(&expr); inferred != air.NoType {
			resultTypeID = inferred
		}
	}
	var resultExpr ast.Expr = ast.NewIdent("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		resultExpr = ast.NewIdent(temp)
		assignTarget = resultExpr
	}
	someBody := []ast.Stmt{}
	if assignTarget != nil {
		someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{assignTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}})
		if expr.HasCatch {
			someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignTarget}})
		}
	} else {
		someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}})
	}
	var noneBody []ast.Stmt
	if expr.HasCatch {
		var catchDecls []ast.Stmt
		var catchTarget ast.Expr
		if !l.isVoidType(fn.Signature.Return) {
			catchTargetName := l.nextTemp()
			var err error
			catchDecls, err = l.declareTemp(fn.Signature.Return, catchTargetName)
			if err != nil {
				return loweredExpr{}, err
			}
			catchTarget = ast.NewIdent(catchTargetName)
		}
		catchBody, err := l.lowerValueBlock(fn, expr.Catch, fn.Signature.Return, catchTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		noneBody = append(catchDecls, catchBody...)
		if !l.isVoidType(fn.Signature.Return) {
			noneBody = append(noneBody, &ast.ReturnStmt{Results: []ast.Expr{catchTarget}})
		} else {
			noneBody = append(noneBody, &ast.ReturnStmt{})
		}
	} else {
		returnExpr := ast.Expr(targetExpr)
		if fn.Signature.Return != targetTypeID {
			returnType, err := l.goType(fn.Signature.Return)
			if err != nil {
				return loweredExpr{}, err
			}
			returnExpr = &ast.CompositeLit{Type: returnType}
		}
		noneBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{returnExpr}}}
	}
	stmts = append(stmts, &ast.IfStmt{Cond: l.maybeIsSomeExpr(targetExpr), Body: &ast.BlockStmt{List: someBody}, Else: &ast.BlockStmt{List: noneBody}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMatchMaybe(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("maybe match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	targetTypeID := l.resolvedExprType(fn, *expr.Target)
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(targetTypeID, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := ast.NewIdent(targetTemp)
	resultExpr := ast.NewIdent("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}
	someName := l.localName(fn, expr.SomeLocal)
	l.declaredLocals[expr.SomeLocal] = true
	someDecl := &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(someName)}, Tok: token.DEFINE, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}}
	someBody, err := l.lowerValueBlock(fn, expr.Some, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	someBody = append([]ast.Stmt{someDecl, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(someName)}}}, someBody...)
	var noneBody []ast.Stmt
	if l.shouldPropagateMaybeNone(expr) {
		noneBody = nil
	} else {
		noneBody, err = l.lowerValueBlock(fn, expr.None, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(targetExpr),
		Body: &ast.BlockStmt{List: someBody},
		Else: &ast.BlockStmt{List: noneBody},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

// lowerJSONParse lowers json::parse<T>(input) to a native encoding/json/v2
// Unmarshal into the typed target. Struct json tags, runtime.Maybe's
// UnmarshalJSON, and enums-as-int give the Ard JSON shape without a generated
// decoder (ADR 0031).
func (l *lowerer) lowerJSONParse(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("json::parse missing input argument")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if !validTypeID(l.program, expr.Type) {
		return loweredExpr{}, fmt.Errorf("invalid json::parse result type %d", expr.Type)
	}
	resultInfo := l.program.Types[expr.Type-1]
	if resultInfo.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("json::parse expected result return, got %s", resultInfo.Name)
	}
	valueType, err := l.goType(resultInfo.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	errTemp := l.nextTemp()
	outTemp := l.nextTemp()
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts,
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(outTemp)}, Type: valueType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `""`}}},
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("err")}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.qualified("json", "encoding/json/v2", "Unmarshal"), Args: []ast.Expr{
				&ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Args: []ast.Expr{target.expr}},
				&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(outTemp)},
			}}}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent("err"), Op: token.NEQ, Y: ast.NewIdent("nil")},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("err"), Sel: ast.NewIdent("Error")}}}}}},
		},
	)
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, expr.Type), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: ast.NewIdent(outTemp)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: ast.NewIdent(errTemp)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: &ast.BasicLit{Kind: token.STRING, Value: `""`}}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMakeList(fn air.Function, expr air.Expr) (loweredExpr, error) {
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	elts := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	elemType := air.NoType
	if validTypeID(l.program, expr.Type) {
		if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeList {
			elemType = info.Elem
		}
	}
	for _, arg := range expr.Args {
		var loweredArg loweredExpr
		if elemType != air.NoType {
			loweredArg, err = l.lowerExprWithExpectedType(fn, arg, elemType)
		} else {
			loweredArg, err = l.lowerExpr(fn, arg)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		argExpr := loweredArg.expr
		if l.isVoidType(elemType) || isVoidExpr(argExpr) {
			stmts = l.appendVoidValueEval(stmts, argExpr)
			argExpr = l.voidValueExpr()
		}
		elts = append(elts, argExpr)
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: typ, Elts: elts}}, nil
}

func (l *lowerer) lowerMakeClosure(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if !validFunctionID(l.program, expr.Function) {
		return loweredExpr{}, fmt.Errorf("invalid closure function %d", expr.Function)
	}
	closureFn := l.program.Functions[expr.Function]
	if l.inlineClosures[expr.Function] {
		return l.lowerInlineClosure(fn, expr, closureFn)
	}
	closureType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	funcType, _ := closureType.(*ast.FuncType)
	callArgs := make([]ast.Expr, 0, len(expr.CaptureLocals)+len(closureFn.Signature.Params))
	stmts := []ast.Stmt{}
	for i, local := range expr.CaptureLocals {
		argExpr := ast.Expr(ast.NewIdent(l.localName(fn, local)))
		if i < len(closureFn.Captures) {
			capture := closureFn.Captures[i]
			captureParam := air.Param{Name: capture.Name, Type: capture.Type}
			if int(capture.Local) >= 0 && int(capture.Local) < len(closureFn.Locals) {
				captureParam.Mutable = closureFn.Locals[capture.Local].Mutable
			}
			var setup []ast.Stmt
			var post []ast.Stmt
			argExpr, setup, post, err = l.adaptCallArgWithStmts(fn, air.Expr{Kind: air.ExprLoadLocal, Type: capture.Type, Local: local}, argExpr, captureParam)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, setup...)
			stmts = append(stmts, post...)
		}
		callArgs = append(callArgs, argExpr)
	}
	params := []*ast.Field{}
	for i, param := range closureFn.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return loweredExpr{}, err
		}
		name := l.localName(closureFn, air.LocalID(i))
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: paramType})
		callArgs = append(callArgs, ast.NewIdent(name))
	}
	bodyStmts := []ast.Stmt{}
	closureFun := l.functionExpr(closureFn)
	if len(closureFn.TypeParams) > 0 {
		closureFun = l.indexWithTypeParamNames(closureFun, closureFn.TypeParams)
	}
	call := &ast.CallExpr{Fun: closureFun, Args: callArgs}
	if funcType == nil {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}}
	} else {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}, Results: funcType.Results}
	}
	if (funcType.Results == nil || len(funcType.Results.List) == 0) && closureFn.Body.Result != nil && !l.isVoidType(closureFn.Body.Result.Type) {
		returnType, err := l.goType(closureFn.Body.Result.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
	}
	if funcType.Results == nil || len(funcType.Results.List) == 0 {
		bodyStmts = append(bodyStmts, &ast.ExprStmt{X: call})
	} else {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	funcLit := &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: bodyStmts}}
	return loweredExpr{stmts: stmts, expr: funcLit}, nil
}

func (l *lowerer) lowerInlineClosure(parent air.Function, expr air.Expr, closureFn air.Function) (loweredExpr, error) {
	inlineFn := closureFn
	inlineFn.Captures = append([]air.Capture(nil), closureFn.Captures...)
	inlineFn.Locals = append([]air.Local(nil), closureFn.Locals...)
	for i := range inlineFn.Captures {
		if i >= len(expr.CaptureLocals) {
			break
		}
		capture := &inlineFn.Captures[i]
		if int(capture.Local) < 0 || int(capture.Local) >= len(inlineFn.Locals) {
			continue
		}
		outerName := l.localName(parent, expr.CaptureLocals[i])
		capture.Name = outerName
		inlineFn.Locals[capture.Local].Name = outerName
		// Inline closures directly close over the outer Go local. Do not treat
		// captures as pointer parameters; mutable argument lowering can still take
		// the address of the outer local when a callee requires it.
		inlineFn.Locals[capture.Local].Mutable = false
	}
	// inlineFn is a mutated copy sharing the original closure's FunctionID. Drop
	// any cached name table (e.g. populated eagerly by buildReservedGoIdentifiers)
	// so names recompute from the rewritten capture names, and restore the entry
	// afterwards so the original closure's table is never observed as the inline
	// one.
	prevLocalNames, hadLocalNames := l.localNameCache[inlineFn.ID]
	delete(l.localNameCache, inlineFn.ID)
	defer func() {
		if hadLocalNames {
			l.localNameCache[inlineFn.ID] = prevLocalNames
		} else {
			delete(l.localNameCache, inlineFn.ID)
		}
	}()

	closureType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	funcType, _ := closureType.(*ast.FuncType)
	params := []*ast.Field{}
	for i, param := range inlineFn.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return loweredExpr{}, err
		}
		name := l.localName(inlineFn, air.LocalID(i))
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: paramType})
	}
	if funcType == nil {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}}
	} else {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}, Results: funcType.Results}
	}
	returnTypeID := inlineFn.Signature.Return
	if (funcType.Results == nil || len(funcType.Results.List) == 0) && inlineFn.Body.Result != nil && !l.isVoidType(inlineFn.Body.Result.Type) {
		returnType, err := l.goType(inlineFn.Body.Result.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		funcType.Results = &ast.FieldList{List: []*ast.Field{{Type: returnType}}}
		returnTypeID = inlineFn.Body.Result.Type
	}

	savedDeclared := l.declaredLocals
	l.declaredLocals = map[air.LocalID]bool{}
	defer func() { l.declaredLocals = savedDeclared }()
	for _, capture := range inlineFn.Captures {
		l.declaredLocals[capture.Local] = true
	}
	for _, local := range inlineFn.Locals {
		if int(local.ID) < len(inlineFn.Signature.Params) {
			l.declaredLocals[local.ID] = true
		}
	}
	body, err := l.lowerBlock(inlineFn, inlineFn.Body, returnTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{expr: &ast.FuncLit{Type: funcType, Body: body}}, nil
}

func (l *lowerer) lowerCallClosure(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("call closure missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	var targetInfo air.TypeInfo
	hasFunctionType := false
	if validTypeID(l.program, expr.Target.Type) {
		info := l.program.Types[expr.Target.Type-1]
		if info.Kind == air.TypeFunction {
			targetInfo = info
			hasFunctionType = true
		}
	}
	params := []air.Param{}
	if hasFunctionType {
		params = make([]air.Param, len(targetInfo.Params))
		for i, paramType := range targetInfo.Params {
			params[i] = air.Param{Type: paramType}
			if i < len(targetInfo.ParamMutable) {
				params[i].Mutable = targetInfo.ParamMutable[i]
			}
		}
	}
	args, stmts, writeback, err := l.lowerCallArgs(fn, expr.Args, params)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(append([]ast.Stmt{}, target.stmts...), stmts...)
	call := &ast.CallExpr{Fun: target.expr, Args: args}
	return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
}

func (l *lowerer) lowerListSet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("list set expects target and two args")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	index, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	elemType := air.NoType
	if validTypeID(l.program, expr.Target.Type) {
		if info := l.program.Types[expr.Target.Type-1]; info.Kind == air.TypeList {
			elemType = info.Elem
		}
	}
	var value loweredExpr
	if elemType != air.NoType {
		value, err = l.lowerExprWithExpectedType(fn, expr.Args[1], elemType)
	} else {
		value, err = l.lowerExpr(fn, expr.Args[1])
	}
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, index.stmts...)
	stmts = append(stmts, value.stmts...)
	valueExpr := value.expr
	if l.isVoidType(elemType) || isVoidExpr(valueExpr) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: index.expr}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("true")}, nil
}

func (l *lowerer) lowerListSwap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("list swap expects target and two indexes")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	left, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	right, err := l.lowerExpr(fn, expr.Args[1])
	if err != nil {
		return loweredExpr{}, err
	}
	leftName := l.nextTemp()
	rightName := l.nextTemp()
	stmts := append(target.stmts, left.stmts...)
	stmts = append(stmts, right.stmts...)
	stmts = append(stmts,
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(leftName)}, Tok: token.DEFINE, Rhs: []ast.Expr{left.expr}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(rightName)}, Tok: token.DEFINE, Rhs: []ast.Expr{right.expr}},
		&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: ast.NewIdent(leftName)}, &ast.IndexExpr{X: target.expr, Index: ast.NewIdent(rightName)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: ast.NewIdent(rightName)}, &ast.IndexExpr{X: target.expr, Index: ast.NewIdent(leftName)}}},
	)
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
}

func (l *lowerer) lowerListPrepend(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list prepend expects target and value")
	}
	if expr.Target.Kind != air.ExprLoadLocal {
		return loweredExpr{}, fmt.Errorf("list prepend currently requires local target")
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("invalid list prepend target type")
	}
	listInfo := l.program.Types[expr.Target.Type-1]
	if listInfo.Kind != air.TypeList {
		return loweredExpr{}, fmt.Errorf("list prepend target type kind %d", listInfo.Kind)
	}
	value, err := l.lowerExprWithExpectedType(fn, expr.Args[0], listInfo.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	elemType, err := l.goType(listInfo.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	target := l.localValueExpr(fn, expr.Target.Local)
	valueExpr := value.expr
	stmts := append([]ast.Stmt{}, value.stmts...)
	if l.isVoidType(listInfo.Elem) || isVoidExpr(valueExpr) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	assign := &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{&ast.CompositeLit{Type: &ast.ArrayType{Elt: elemType}, Elts: []ast.Expr{valueExpr}}, target}, Ellipsis: 2}}}
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target}}}, nil
}

func (l *lowerer) lowerListSort(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list sort expects target and comparator")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	cmp, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	sortAlias := l.registerImport("sort", "sort")
	lessFunc := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent("i")}, Type: ast.NewIdent("int")},
				{Names: []*ast.Ident{ast.NewIdent("j")}, Type: ast.NewIdent("int")},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: cmp.expr, Args: []ast.Expr{
				&ast.IndexExpr{X: target.expr, Index: ast.NewIdent("i")},
				&ast.IndexExpr{X: target.expr, Index: ast.NewIdent("j")},
			}}}},
		}},
	}
	stmts := append(target.stmts, cmp.stmts...)
	stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(sortAlias), Sel: ast.NewIdent("SliceStable")}, Args: []ast.Expr{target.expr, lessFunc}}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
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
	var value loweredExpr
	var err error
	if validTypeID(l.program, expr.Target.Type) {
		if info := l.program.Types[expr.Target.Type-1]; info.Kind == air.TypeList {
			value, err = l.lowerExprWithExpectedType(fn, expr.Args[0], info.Elem)
		} else {
			value, err = l.lowerExpr(fn, expr.Args[0])
		}
	} else {
		value, err = l.lowerExpr(fn, expr.Args[0])
	}
	if err != nil {
		return loweredExpr{}, err
	}
	target := l.localValueExpr(fn, expr.Target.Local)
	valueExpr := value.expr
	stmts := append([]ast.Stmt{}, value.stmts...)
	if validTypeID(l.program, expr.Target.Type) {
		if info := l.program.Types[expr.Target.Type-1]; info.Kind == air.TypeList && l.isVoidType(info.Elem) {
			stmts = l.appendVoidValueEval(stmts, valueExpr)
			valueExpr = l.voidValueExpr()
		}
	}
	if isVoidExpr(valueExpr) {
		valueExpr = l.voidValueExpr()
	}
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{target},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("append"), Args: []ast.Expr{target, valueExpr}}},
	}
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{target}}}, nil
}

func (l *lowerer) lowerMakeMap(fn air.Function, expr air.Expr) (loweredExpr, error) {
	keyType, valueType := l.mapKeyValueTypes(expr.Type)
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	elts := make([]ast.Expr, 0, len(expr.Entries))
	stmts := []ast.Stmt{}
	for _, entry := range expr.Entries {
		var key loweredExpr
		if keyType != air.NoType {
			key, err = l.lowerExprWithExpectedType(fn, entry.Key, keyType)
		} else {
			key, err = l.lowerExpr(fn, entry.Key)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		var value loweredExpr
		if valueType != air.NoType {
			value, err = l.lowerExprWithExpectedType(fn, entry.Value, valueType)
		} else {
			value, err = l.lowerExpr(fn, entry.Value)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, key.stmts...)
		keyExpr := key.expr
		if l.isVoidType(keyType) || isVoidExpr(keyExpr) {
			stmts = l.appendVoidValueEval(stmts, keyExpr)
			keyExpr = l.voidValueExpr()
		}
		stmts = append(stmts, value.stmts...)
		valueExpr := value.expr
		if l.isVoidType(valueType) || isVoidExpr(valueExpr) {
			stmts = l.appendVoidValueEval(stmts, valueExpr)
			valueExpr = l.voidValueExpr()
		}
		elts = append(elts, &ast.KeyValueExpr{Key: keyExpr, Value: valueExpr})
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: typ, Elts: elts}}, nil
}

func (l *lowerer) lowerMapHas(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map has expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerMapKeyArg(fn, expr.Target.Type, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	okName := l.nextTemp()
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, decls...)
	lookup := &ast.IndexExpr{X: target.expr, Index: key.expr}
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{lookup}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(okName)}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

// lowerMakeChannel lowers ard/channel::new to `make(chan T, capacity)`.
func (l *lowerer) lowerMakeChannel(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("make channel expects one arg")
	}
	chanType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	capacity, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{chanType, capacity.expr}}
	return loweredExpr{stmts: capacity.stmts, expr: call}, nil
}

// lowerChannelSend lowers ard/channel::send to `ch <- value` and yields Void.
func (l *lowerer) lowerChannelSend(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("channel send expects two args")
	}
	ch, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	value, err := l.lowerExpr(fn, expr.Args[1])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(ch.stmts, value.stmts...)
	stmts = append(stmts, &ast.SendStmt{Chan: ch.expr, Value: value.expr})
	return loweredExpr{stmts: stmts, expr: l.voidValueExpr()}, nil
}

// lowerChannelRecv lowers ard/channel::recv to `v, ok := <-ch` wrapped into a
// Maybe (some on a live receive, none on a closed-and-drained channel).
func (l *lowerer) lowerChannelRecv(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("channel recv expects one arg")
	}
	ch, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	okName := l.nextTemp()
	recv := ast.Expr(&ast.UnaryExpr{Op: token.ARROW, X: ch.expr})
	stmts := append(ch.stmts, decls...)
	someExpr, err := l.maybeSomeExpr(expr.Type, ast.NewIdent(valueTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{recv}},
		Cond: ast.NewIdent(okName),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

// lowerChannelClose lowers ard/channel::close to `close(ch)` and yields Void.
func (l *lowerer) lowerChannelClose(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("channel close expects one arg")
	}
	ch, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(ch.stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("close"), Args: []ast.Expr{ch.expr}}})
	return loweredExpr{stmts: stmts, expr: l.voidValueExpr()}, nil
}

// lowerChannelNarrow converts a bidirectional channel to a directional view via
// a Go conversion to the result's directional channel type (ADR 0032 Layer 2).
func (l *lowerer) lowerChannelNarrow(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("channel narrow expects one arg")
	}
	ch, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	// (<-chan T)(ch) / (chan<- T)(ch). Parenthesize the channel type so it parses
	// as a conversion.
	return loweredExpr{stmts: ch.stmts, expr: &ast.CallExpr{
		Fun:  &ast.ParenExpr{X: typ},
		Args: []ast.Expr{ch.expr},
	}}, nil
}

// lowerSelect emits a native Go select statement (ADR 0032). Channel and send
// operands are hoisted before the select so they are evaluated once; recv arms
// with a binding build the element Maybe from the comma-ok receive.
func (l *lowerer) lowerSelect(fn air.Function, expr air.Expr) (loweredExpr, error) {
	var preStmts []ast.Stmt
	var resultExpr ast.Expr = ast.NewIdent("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		preStmts = append(preStmts, decls...)
		assignTarget = ast.NewIdent(temp)
		resultExpr = ast.NewIdent(temp)
	}

	clauses := []ast.Stmt{}
	for _, arm := range expr.SelectCases {
		clause := &ast.CommClause{}
		switch arm.Kind {
		case air.SelectArmDefault:
			body, err := l.lowerValueBlock(fn, arm.Body, expr.Type, assignTarget)
			if err != nil {
				return loweredExpr{}, err
			}
			clause.Body = body

		case air.SelectArmSend:
			if arm.Channel == nil || arm.Value == nil {
				return loweredExpr{}, fmt.Errorf("select send arm missing channel or value")
			}
			ch, err := l.lowerExpr(fn, *arm.Channel)
			if err != nil {
				return loweredExpr{}, err
			}
			val, err := l.lowerExpr(fn, *arm.Value)
			if err != nil {
				return loweredExpr{}, err
			}
			preStmts = append(preStmts, ch.stmts...)
			preStmts = append(preStmts, val.stmts...)
			clause.Comm = &ast.SendStmt{Chan: ch.expr, Value: val.expr}
			body, err := l.lowerValueBlock(fn, arm.Body, expr.Type, assignTarget)
			if err != nil {
				return loweredExpr{}, err
			}
			clause.Body = body

		case air.SelectArmRecv:
			if arm.Channel == nil {
				return loweredExpr{}, fmt.Errorf("select recv arm missing channel")
			}
			ch, err := l.lowerExpr(fn, *arm.Channel)
			if err != nil {
				return loweredExpr{}, err
			}
			preStmts = append(preStmts, ch.stmts...)
			recv := ast.Expr(&ast.UnaryExpr{Op: token.ARROW, X: ch.expr})
			if arm.HasBind {
				valueTemp := l.nextTemp()
				okTemp := l.nextTemp()
				clause.Comm = &ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okTemp)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{recv},
				}
				bindName := l.localName(fn, arm.BindLocal)
				l.declaredLocals[arm.BindLocal] = true
				maybeTypeID := fn.Locals[arm.BindLocal].Type
				decls, err := l.declareTemp(maybeTypeID, bindName)
				if err != nil {
					return loweredExpr{}, err
				}
				someExpr, err := l.maybeSomeExpr(maybeTypeID, ast.NewIdent(valueTemp))
				if err != nil {
					return loweredExpr{}, err
				}
				body, err := l.lowerValueBlock(fn, arm.Body, expr.Type, assignTarget)
				if err != nil {
					return loweredExpr{}, err
				}
				prefix := append([]ast.Stmt{}, decls...)
				prefix = append(prefix,
					&ast.IfStmt{
						Cond: ast.NewIdent(okTemp),
						Body: &ast.BlockStmt{List: []ast.Stmt{
							&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(bindName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
						}},
					},
					&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(bindName)}},
				)
				clause.Body = append(prefix, body...)
			} else {
				clause.Comm = &ast.ExprStmt{X: recv}
				body, err := l.lowerValueBlock(fn, arm.Body, expr.Type, assignTarget)
				if err != nil {
					return loweredExpr{}, err
				}
				clause.Body = body
			}

		default:
			return loweredExpr{}, fmt.Errorf("unknown select arm kind %d", arm.Kind)
		}
		clauses = append(clauses, clause)
	}

	preStmts = append(preStmts, &ast.SelectStmt{Body: &ast.BlockStmt{List: clauses}})
	return loweredExpr{stmts: preStmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMapGet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map get expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerMapKeyArg(fn, expr.Target.Type, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	okName := l.nextTemp()
	lookup := ast.Expr(&ast.IndexExpr{X: target.expr, Index: key.expr})
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, decls...)
	someExpr, err := l.maybeSomeExpr(expr.Type, ast.NewIdent(valueTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{lookup}},
		Cond: ast.NewIdent(okName),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerMapSet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("map set expects target and two args")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	keyType, valueType := l.mapKeyValueTypes(expr.Target.Type)
	var key loweredExpr
	if keyType != air.NoType {
		key, err = l.lowerExprWithExpectedType(fn, expr.Args[0], keyType)
	} else {
		key, err = l.lowerExpr(fn, expr.Args[0])
	}
	if err != nil {
		return loweredExpr{}, err
	}
	var value loweredExpr
	if valueType != air.NoType {
		value, err = l.lowerExprWithExpectedType(fn, expr.Args[1], valueType)
	} else {
		value, err = l.lowerExpr(fn, expr.Args[1])
	}
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, key.stmts...)
	keyExpr := key.expr
	if l.isVoidType(keyType) || isVoidExpr(keyExpr) {
		stmts = l.appendVoidValueEval(stmts, keyExpr)
		keyExpr = l.voidValueExpr()
	}
	stmts = append(stmts, value.stmts...)
	valueExpr := value.expr
	if l.isVoidType(valueType) || isVoidExpr(valueExpr) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: keyExpr}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("true")}, nil
}

func (l *lowerer) lowerMapDrop(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map drop expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	key, err := l.lowerMapKeyArg(fn, expr.Target.Type, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("delete"), Args: []ast.Expr{target.expr, key.expr}}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
}

func (l *lowerer) lowerMapKeys(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("map keys missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	helper, err := l.mapKeyHelper(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: ast.NewIdent(helper), Args: []ast.Expr{target.expr}}}, nil
}

func (l *lowerer) lowerMapKeyAt(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map key_at expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	index, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, index.stmts...)
	helper, err := l.mapKeyHelper(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	keys := &ast.CallExpr{Fun: ast.NewIdent(helper), Args: []ast.Expr{target.expr}}
	return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: keys, Index: index.expr}}, nil
}

func (l *lowerer) lowerMapValueAt(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map value_at expects target and one arg")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	index, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(target.stmts, index.stmts...)
	helper, err := l.mapKeyHelper(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	keyExpr := &ast.IndexExpr{X: &ast.CallExpr{Fun: ast.NewIdent(helper), Args: []ast.Expr{target.expr}}, Index: index.expr}
	return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: target.expr, Index: keyExpr}}, nil
}

func (l *lowerer) mapKeyHelper(typeID air.TypeID) (string, error) {
	if !validTypeID(l.program, typeID) {
		return "", fmt.Errorf("invalid map type %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind != air.TypeMap {
		return "", fmt.Errorf("type %s is not a map", info.Name)
	}
	keyType := l.program.Types[info.Key-1]
	switch keyType.Kind {
	case air.TypeInt:
		l.markRuntimeHelper("sorted_int_keys")
		return "ardSortedIntKeys", nil
	case air.TypeStr:
		l.markRuntimeHelper("sorted_string_keys")
		return "ardSortedStringKeys", nil
	case air.TypeAny:
		l.markRuntimeHelper("sorted_any_keys")
		return "ardSortedAnyKeys", nil
	default:
		return "", fmt.Errorf("unsupported map key type %s for ordered iteration", keyType.Name)
	}
}

func mustTypeExpr(l *lowerer, typeID air.TypeID) ast.Expr {
	typ, err := l.goType(typeID)
	if err != nil {
		panic(err)
	}
	return typ
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
	targetIsTraitObject := validTypeID(l.program, expr.Target.Type) && l.program.Types[expr.Target.Type-1].Kind == air.TypeTraitObject
	if trait.Name == "ToString" && method.Name == "to_str" && !targetIsTraitObject {
		return loweredExpr{stmts: target.stmts, expr: l.toStringExpr(expr.Target.Type, target.expr)}, nil
	}
	if !targetIsTraitObject {
		return loweredExpr{}, fmt.Errorf("unsupported trait call %s.%s", trait.Name, method.Name)
	}
	if l.exprIsMutableReference(fn, *expr.Target) {
		return l.lowerMutableTraitRefCall(fn, target, expr)
	}
	if l.usesNativeTraitInterface(expr.Target.Type) {
		return l.lowerNativeTraitInterfaceCall(fn, target, expr)
	}
	return l.lowerTraitObjectCall(fn, target, expr)
}

func (l *lowerer) lowerNativeTraitInterfaceCall(fn air.Function, target loweredExpr, expr air.Expr) (loweredExpr, error) {
	if expr.Trait < 0 || int(expr.Trait) >= len(l.program.Traits) {
		return loweredExpr{}, fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := l.program.Traits[expr.Trait]
	if expr.Method < 0 || expr.Method >= len(trait.Methods) {
		return loweredExpr{}, fmt.Errorf("invalid trait method %d for %s", expr.Method, trait.Name)
	}
	method := trait.Methods[expr.Method]
	methodName, ok := goMethodName(method.Name)
	if !ok {
		return loweredExpr{}, fmt.Errorf("trait method %s cannot be lowered as a Go method", method.Name)
	}
	args, argStmts, writeback, err := l.lowerCallArgs(fn, expr.Args, method.Signature.Params)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, argStmts...)
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(methodName)}, Args: args}
	return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
}

func (l *lowerer) exprIsMutableReference(fn air.Function, expr air.Expr) bool {
	switch expr.Kind {
	case air.ExprLoadLocal:
		return l.localIsPointerParam(fn, expr.Local)
	case air.ExprGetField:
		if expr.Target == nil || !validTypeID(l.program, expr.Target.Type) {
			return false
		}
		targetType := l.program.Types[expr.Target.Type-1]
		return targetType.Kind == air.TypeStruct && expr.Field >= 0 && expr.Field < len(targetType.Fields) && targetType.Fields[expr.Field].Mutable
	default:
		return false
	}
}

func (l *lowerer) lowerMutableTraitRefCall(fn air.Function, target loweredExpr, expr air.Expr) (loweredExpr, error) {
	if expr.Trait < 0 || int(expr.Trait) >= len(l.program.Traits) {
		return loweredExpr{}, fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := l.program.Traits[expr.Trait]
	if expr.Method < 0 || expr.Method >= len(trait.Methods) {
		return loweredExpr{}, fmt.Errorf("invalid trait method %d for %s", expr.Method, trait.Name)
	}
	method := trait.Methods[expr.Method]
	args, argStmts, writeback, err := l.lowerCallArgs(fn, expr.Args, method.Signature.Params)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, argStmts...)
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(mutableTraitMethodFieldName(trait.ID, expr.Method))}, Args: args}
	return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
}

func (l *lowerer) lowerTraitObjectCall(fn air.Function, target loweredExpr, expr air.Expr) (loweredExpr, error) {
	isVoid := l.isVoidType(expr.Type)
	stmts := append([]ast.Stmt{}, target.stmts...)

	var resultTemp string
	if !isVoid {
		resultTemp = l.nextTemp()
		resultType, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: resultType}}}})
	}

	traitMethod := l.program.Traits[expr.Trait].Methods[expr.Method]
	loweredArgs := make([]loweredExpr, len(expr.Args))
	for i, arg := range expr.Args {
		var loweredArg loweredExpr
		var err error
		if i < len(traitMethod.Signature.Params) && !traitMethod.Signature.Params[i].Mutable {
			loweredArg, err = l.lowerExprWithExpectedType(fn, arg, traitMethod.Signature.Params[i].Type)
		} else {
			loweredArg, err = l.lowerExpr(fn, arg)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		if i < len(traitMethod.Signature.Params) && l.isVoidType(traitMethod.Signature.Params[i].Type) {
			stmts = l.appendVoidValueEval(stmts, loweredArg.expr)
			loweredArg.expr = l.voidValueExpr()
		}
		loweredArgs[i] = loweredArg
	}

	switchVar := l.nextTemp()
	switchVarExpr := ast.NewIdent(switchVar)
	cases := []ast.Stmt{}
	if validTraitID(l.program, expr.Trait) && expr.Method >= 0 && expr.Method < len(l.program.Traits[expr.Trait].Methods) {
		if l.mutableTraitRefs != nil {
			l.mutableTraitRefs[expr.Trait] = true
		}
		trait := l.program.Traits[expr.Trait]
		method := trait.Methods[expr.Method]
		for _, caseType := range []ast.Expr{ast.NewIdent(mutableTraitRefTypeName(trait)), &ast.StarExpr{X: ast.NewIdent(mutableTraitRefTypeName(trait))}} {
			args := make([]ast.Expr, 0, len(loweredArgs))
			body := []ast.Stmt{}
			writeback := []ast.Stmt{}
			for i, loweredArg := range loweredArgs {
				argExpr := loweredArg.expr
				if i < len(method.Signature.Params) {
					var setup []ast.Stmt
					var post []ast.Stmt
					var adaptErr error
					argExpr, setup, post, adaptErr = l.adaptCallArgWithStmts(fn, expr.Args[i], argExpr, method.Signature.Params[i])
					if adaptErr != nil {
						return loweredExpr{}, adaptErr
					}
					body = append(body, setup...)
					writeback = append(writeback, post...)
				}
				args = append(args, argExpr)
			}
			call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: switchVarExpr, Sel: ast.NewIdent(mutableTraitMethodFieldName(trait.ID, expr.Method))}, Args: args}
			if isVoid {
				body = append(body, &ast.ExprStmt{X: call})
			} else {
				body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
			}
			body = append(body, writeback...)
			cases = append(cases, &ast.CaseClause{List: []ast.Expr{caseType}, Body: body})
		}
	}
	for _, impl := range l.program.Impls {
		if impl.Trait != expr.Trait || expr.Method >= len(impl.Methods) || !validTypeID(l.program, impl.ForType) {
			continue
		}
		methodFn := l.program.Functions[impl.Methods[expr.Method]]
		receiver := ast.Expr(switchVarExpr)
		if len(methodFn.Signature.Params) > 0 {
			receiverParam := methodFn.Signature.Params[0]
			if receiverParam.Mutable && validTypeID(l.program, receiverParam.Type) && l.program.Types[receiverParam.Type-1].Kind == air.TypeStruct {
				receiver = &ast.UnaryExpr{Op: token.AND, X: receiver}
			}
		}
		args := []ast.Expr{receiver}
		body := []ast.Stmt{}
		writeback := []ast.Stmt{}
		for i, loweredArg := range loweredArgs {
			argExpr := loweredArg.expr
			paramIndex := i + 1 // skip receiver
			if paramIndex < len(methodFn.Signature.Params) {
				var setup []ast.Stmt
				var post []ast.Stmt
				var adaptErr error
				argExpr, setup, post, adaptErr = l.adaptCallArgWithStmts(fn, expr.Args[i], argExpr, methodFn.Signature.Params[paramIndex])
				if adaptErr != nil {
					return loweredExpr{}, adaptErr
				}
				body = append(body, setup...)
				writeback = append(writeback, post...)
			}
			args = append(args, argExpr)
		}
		callResult := ast.Expr(&ast.CallExpr{Fun: l.functionExpr(methodFn), Args: args})
		if l.isBuiltinToStringTraitCall(expr, impl.ForType) && len(args) == 1 {
			callResult = l.toStringExpr(impl.ForType, args[0])
		}
		if isVoid {
			body = append(body, &ast.ExprStmt{X: callResult})
		} else {
			body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{callResult}})
		}
		body = append(body, writeback...)
		if traitObjectWritebackAllowed(fn, *expr.Target) && len(methodFn.Signature.Params) > 0 {
			receiverParam := methodFn.Signature.Params[0]
			if receiverParam.Mutable && validTypeID(l.program, receiverParam.Type) && l.program.Types[receiverParam.Type-1].Kind == air.TypeStruct {
				body = append(body, &ast.AssignStmt{
					Lhs: []ast.Expr{target.expr},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{switchVarExpr},
				})
			}
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{mustTypeExpr(l, impl.ForType)},
			Body: body,
		})
	}
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("panic"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: "\"unsupported trait object dispatch\""}}}}}})
	stmts = append(stmts, &ast.TypeSwitchStmt{Assign: &ast.AssignStmt{Lhs: []ast.Expr{switchVarExpr}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: target.expr}}}, Body: &ast.BlockStmt{List: cases}})
	if isVoid {
		return loweredExpr{stmts: stmts, expr: ast.NewIdent("nil")}, nil
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
}

func (l *lowerer) isBuiltinToStringTraitCall(expr air.Expr, typeID air.TypeID) bool {
	if expr.Trait < 0 || int(expr.Trait) >= len(l.program.Traits) || expr.Method < 0 {
		return false
	}
	trait := l.program.Traits[expr.Trait]
	if expr.Method >= len(trait.Methods) || trait.Name != "ToString" || trait.Methods[expr.Method].Name != "to_str" {
		return false
	}
	if !validTypeID(l.program, typeID) {
		return false
	}
	switch l.program.Types[typeID-1].Kind {
	case air.TypeInt, air.TypeFloat, air.TypeBool, air.TypeByte, air.TypeRune, air.TypeStr:
		return true
	default:
		return false
	}
}

func traitObjectWritebackAllowed(fn air.Function, expr air.Expr) bool {
	switch expr.Kind {
	case air.ExprLoadLocal:
		if int(expr.Local) < 0 || int(expr.Local) >= len(fn.Locals) {
			return false
		}
		return fn.Locals[expr.Local].Mutable
	case air.ExprGetField:
		if expr.Target == nil {
			return false
		}
		return traitObjectWritebackAllowed(fn, *expr.Target)
	default:
		return false
	}
}

func exportedFieldName(name string) string {
	return naturalGoIdentifier(name, true)
}

func (l *lowerer) goFieldName(typ air.TypeInfo, fieldName string) string {
	// Struct fields are always exported so every struct is serializable through
	// encoding/json regardless of the struct's visibility (ADR 0031). The JSON
	// wire name is pinned to the Ard field name via a struct tag.
	return naturalGoIdentifier(fieldName, true)
}

func (l *lowerer) convertStdlibError(typeID air.TypeID, expr ast.Expr) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid error type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind == air.TypeStr {
		return &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{expr}}, nil
	}
	if info.Kind != air.TypeStruct {
		return nil, fmt.Errorf("unsupported stdlib error target kind %d", info.Kind)
	}
	elts := make([]ast.Expr, 0, len(info.Fields))
	for _, field := range info.Fields {
		elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(l.goFieldName(info, field.Name)), Value: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent(exportedFieldName(field.Name))}})
	}
	return &ast.CompositeLit{Type: l.compositeTypeExpr(info), Elts: elts}, nil
}

func (l *lowerer) wrapValueErrorCall(resultTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	if !validTypeID(l.program, resultTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid result type id %d", resultTypeID)
	}
	resultType := l.program.Types[resultTypeID-1]
	if resultType.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("expected result type, got kind %d", resultType.Kind)
	}
	valueType, err := l.goType(resultType.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	valueTemp := l.nextTemp()
	errTemp := l.nextTemp()
	nativeTraitValue := l.usesNativeTraitInterface(resultType.Value)
	valueDeclType := valueType
	valueExpr := ast.Expr(ast.NewIdent(valueTemp))
	if nativeTraitValue {
		valueDeclType = ast.NewIdent("any")
		valueExpr, err = l.nativeTraitInterfaceAssertion(resultType.Value, ast.NewIdent(valueTemp))
		if err != nil {
			return loweredExpr{}, err
		}
	}
	decls := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueTemp)}, Type: valueDeclType}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("error")}}}},
	}
	stmts := append([]ast.Stmt{}, decls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(valueTemp), ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	errExpr, err := l.convertStdlibError(resultType.Error, ast.NewIdent(errTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	if nativeTraitValue {
		resultTemp := l.nextTemp()
		resultTypeExpr, err := l.goType(resultTypeID)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts,
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: resultTypeExpr}}}},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Value")}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}},
					&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Ok")}}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent("true")}},
				}},
				Else: &ast.BlockStmt{List: []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Err")}}, Tok: token.ASSIGN, Rhs: []ast.Expr{errExpr}},
				}},
			},
		)
		return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, nil
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) wrapErrorCall(resultTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
	if !validTypeID(l.program, resultTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid result type id %d", resultTypeID)
	}
	resultType := l.program.Types[resultTypeID-1]
	if resultType.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("expected result type, got kind %d", resultType.Kind)
	}
	if !validTypeID(l.program, resultType.Value) || l.program.Types[resultType.Value-1].Kind != air.TypeVoid {
		return loweredExpr{}, fmt.Errorf("expected void result value, got type %d", resultType.Value)
	}
	errTemp := l.nextTemp()
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("error")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	errExpr, err := l.convertStdlibError(resultType.Error, ast.NewIdent(errTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: l.voidValueExpr()},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: ast.NewIdent("nil")}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerUnionArgToAny(expr ast.Expr, typeID air.TypeID) (loweredExpr, error) {
	if !validTypeID(l.program, typeID) {
		return loweredExpr{}, fmt.Errorf("invalid union type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	if info.Kind != air.TypeUnion {
		return loweredExpr{expr: expr}, nil
	}
	temp := l.nextTemp()
	wrappedExpr := expr
	if _, ok := expr.(*ast.CompositeLit); ok {
		wrappedExpr = &ast.ParenExpr{X: expr}
	}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(temp)}, Type: ast.NewIdent("any")}}}},
	}
	cases := make([]ast.Stmt, 0, len(info.Members))
	for _, member := range info.Members {
		fieldName := unionMemberFieldName(info, member)
		valueExpr := ast.Expr(&ast.SelectorExpr{X: wrappedExpr, Sel: ast.NewIdent(fieldName)})
		if validTypeID(l.program, member.Type) && l.program.Types[member.Type-1].Kind == air.TypeVoid {
			valueExpr = ast.NewIdent("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}}},
		})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: wrappedExpr, Sel: ast.NewIdent(unionTagFieldName(info))}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(temp)}, nil
}

func (l *lowerer) lowerUnionSliceArgToAny(expr ast.Expr, typeID air.TypeID) (loweredExpr, error) {
	if !validTypeID(l.program, typeID) {
		return loweredExpr{}, fmt.Errorf("invalid list type id %d", typeID)
	}
	listInfo := l.program.Types[typeID-1]
	if listInfo.Kind != air.TypeList || !validTypeID(l.program, listInfo.Elem) {
		return loweredExpr{expr: expr}, nil
	}
	elemInfo := l.program.Types[listInfo.Elem-1]
	if elemInfo.Kind != air.TypeUnion {
		l.markRuntimeHelper("list_to_any_slice")
		return loweredExpr{expr: &ast.CallExpr{Fun: ast.NewIdent("ardListToAnySlice"), Args: []ast.Expr{expr}}}, nil
	}
	valueTemp := l.nextTemp()
	indexTemp := l.nextTemp()
	outTemp := l.nextTemp()
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(outTemp)}, Type: &ast.ArrayType{Elt: ast.NewIdent("any")}}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(outTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{&ast.ArrayType{Elt: ast.NewIdent("any")}, &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{expr}}}}}},
		&ast.RangeStmt{Key: ast.NewIdent(indexTemp), Value: ast.NewIdent(valueTemp), Tok: token.DEFINE, X: expr, Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{Tag: &ast.SelectorExpr{X: ast.NewIdent(valueTemp), Sel: ast.NewIdent(unionTagFieldName(elemInfo))}, Body: &ast.BlockStmt{List: unionSliceCaseClauses(l.program, elemInfo, outTemp, indexTemp, valueTemp)}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(outTemp)}, nil
}

func unionSliceCaseClauses(program *air.Program, unionInfo air.TypeInfo, outTemp string, indexTemp string, valueTemp string) []ast.Stmt {
	cases := make([]ast.Stmt, 0, len(unionInfo.Members))
	for _, member := range unionInfo.Members {
		valueExpr := ast.Expr(&ast.SelectorExpr{X: ast.NewIdent(valueTemp), Sel: ast.NewIdent(unionMemberFieldName(unionInfo, member))})
		if validTypeID(program, member.Type) && program.Types[member.Type-1].Kind == air.TypeVoid {
			valueExpr = ast.NewIdent("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(outTemp), Index: ast.NewIdent(indexTemp)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}}},
		})
	}
	return cases
}

func (l *lowerer) nativeTraitInterfaceAssertion(typeID air.TypeID, value ast.Expr) (ast.Expr, error) {
	traitType, err := l.goType(typeID)
	if err != nil {
		return nil, err
	}
	return &ast.TypeAssertExpr{X: &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{value}}, Type: traitType}, nil
}

func isVoidExpr(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "nil"
	}
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	switch typ := lit.Type.(type) {
	case *ast.Ident:
		return typ.Name == "Void"
	case *ast.SelectorExpr:
		return typ.Sel != nil && typ.Sel.Name == "Void"
	case *ast.StructType:
		return typ.Fields == nil || len(typ.Fields.List) == 0
	default:
		return false
	}
}

func (l *lowerer) maybeElemTypeExpr(maybeTypeID air.TypeID) (ast.Expr, error) {
	if !validTypeID(l.program, maybeTypeID) {
		return nil, fmt.Errorf("invalid maybe type id %d", maybeTypeID)
	}
	info := l.program.Types[maybeTypeID-1]
	if info.Kind != air.TypeMaybe {
		return nil, fmt.Errorf("expected maybe type, got kind %d", info.Kind)
	}
	return l.goType(info.Elem)
}

func (l *lowerer) maybeSomeExpr(maybeTypeID air.TypeID, value ast.Expr) (ast.Expr, error) {
	elemType, err := l.maybeElemTypeExpr(maybeTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "Some"), Index: elemType}, Args: []ast.Expr{value}}, nil
}

func (l *lowerer) maybeNoneExpr(maybeTypeID air.TypeID) (ast.Expr, error) {
	elemType, err := l.maybeElemTypeExpr(maybeTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "None"), Index: elemType}}, nil
}

func (l *lowerer) maybeIsSomeExpr(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent("IsSome")}}
}

func (l *lowerer) maybeIsNoneExpr(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent("IsNone")}}
}

func (l *lowerer) maybeValueExpr(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent("Value")}}
}

type closureUseInfo struct {
	total    int
	local    int
	retained bool
}

func (l *lowerer) collectInlineClosureFunctions() map[air.FunctionID]bool {
	uses := map[air.FunctionID]*closureUseInfo{}
	directRefs := map[air.FunctionID]bool{}
	for _, fn := range l.program.Functions {
		l.collectClosureUsesInBlock(fn.Body, closureUseValue, uses, directRefs)
	}
	inline := map[air.FunctionID]bool{}
	for _, fn := range l.program.Functions {
		use := uses[fn.ID]
		if use == nil || use.total != 1 || use.local != 1 || use.retained || directRefs[fn.ID] {
			continue
		}
		if !l.canInlineClosureFunction(fn) {
			continue
		}
		inline[fn.ID] = true
	}
	return inline
}

func (l *lowerer) canInlineClosureFunction(fn air.Function) bool {
	if !strings.HasPrefix(fn.Name, "anon_func_") {
		return false
	}
	for _, capture := range fn.Captures {
		if int(capture.Local) >= 0 && int(capture.Local) < len(fn.Locals) && fn.Locals[capture.Local].Mutable {
			return false
		}
	}
	return !functionDirectlyReferences(fn.Body, fn.ID)
}

type closureUseContext uint8

const (
	closureUseValue closureUseContext = iota
	closureUseLocal
)

func (l *lowerer) collectClosureUsesInBlock(block air.Block, context closureUseContext, uses map[air.FunctionID]*closureUseInfo, directRefs map[air.FunctionID]bool) {
	for _, stmt := range block.Stmts {
		if stmt.Value != nil {
			l.collectClosureUsesInExpr(*stmt.Value, closureUseValue, uses, directRefs)
		}
		if stmt.Expr != nil {
			l.collectClosureUsesInExpr(*stmt.Expr, closureUseValue, uses, directRefs)
		}
		if stmt.Target != nil {
			l.collectClosureUsesInExpr(*stmt.Target, closureUseValue, uses, directRefs)
		}
		if stmt.Condition != nil {
			l.collectClosureUsesInExpr(*stmt.Condition, closureUseValue, uses, directRefs)
		}
		l.collectClosureUsesInBlock(stmt.Body, closureUseValue, uses, directRefs)
	}
	if block.Result != nil {
		l.collectClosureUsesInExpr(*block.Result, context, uses, directRefs)
	}
}

func (l *lowerer) collectClosureUsesInExpr(expr air.Expr, context closureUseContext, uses map[air.FunctionID]*closureUseInfo, directRefs map[air.FunctionID]bool) {
	switch expr.Kind {
	case air.ExprMakeClosure:
		use := uses[expr.Function]
		if use == nil {
			use = &closureUseInfo{}
			uses[expr.Function] = use
		}
		use.total++
		if context == closureUseLocal {
			use.local++
		} else {
			use.retained = true
		}
	case air.ExprCall, air.ExprFunctionRef:
		directRefs[expr.Function] = true
	}

	argContext := closureUseValue
	if closureArgConsumedImmediately(expr.Kind) {
		argContext = closureUseLocal
	}
	for i := range expr.Args {
		l.collectClosureUsesInExpr(expr.Args[i], argContext, uses, directRefs)
	}
	for i := range expr.Entries {
		l.collectClosureUsesInExpr(expr.Entries[i].Key, closureUseValue, uses, directRefs)
		l.collectClosureUsesInExpr(expr.Entries[i].Value, closureUseValue, uses, directRefs)
	}
	for i := range expr.Fields {
		l.collectClosureUsesInExpr(expr.Fields[i].Value, closureUseValue, uses, directRefs)
	}
	if expr.Target != nil {
		targetContext := closureUseValue
		if expr.Kind == air.ExprCallClosure {
			targetContext = closureUseLocal
		}
		l.collectClosureUsesInExpr(*expr.Target, targetContext, uses, directRefs)
	}
	if expr.Left != nil {
		l.collectClosureUsesInExpr(*expr.Left, closureUseValue, uses, directRefs)
	}
	if expr.Right != nil {
		l.collectClosureUsesInExpr(*expr.Right, closureUseValue, uses, directRefs)
	}
	if expr.Condition != nil {
		l.collectClosureUsesInExpr(*expr.Condition, closureUseValue, uses, directRefs)
	}
	l.collectClosureUsesInBlock(expr.Body, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.Then, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.Else, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.CatchAll, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.Some, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.None, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.Ok, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.Err, closureUseValue, uses, directRefs)
	l.collectClosureUsesInBlock(expr.Catch, closureUseValue, uses, directRefs)
	for i := range expr.EnumCases {
		l.collectClosureUsesInBlock(expr.EnumCases[i].Body, closureUseValue, uses, directRefs)
	}
	for i := range expr.IntCases {
		l.collectClosureUsesInBlock(expr.IntCases[i].Body, closureUseValue, uses, directRefs)
	}
	for i := range expr.StrCases {
		l.collectClosureUsesInBlock(expr.StrCases[i].Body, closureUseValue, uses, directRefs)
	}
	for i := range expr.RangeCases {
		l.collectClosureUsesInBlock(expr.RangeCases[i].Body, closureUseValue, uses, directRefs)
	}
	for i := range expr.UnionCases {
		l.collectClosureUsesInBlock(expr.UnionCases[i].Body, closureUseValue, uses, directRefs)
	}
}

func closureArgConsumedImmediately(kind air.ExprKind) bool {
	switch kind {
	case air.ExprListSort,
		air.ExprMaybeMap,
		air.ExprMaybeAndThen,
		air.ExprResultMap,
		air.ExprResultMapErr,
		air.ExprResultAndThen:
		return true
	default:
		return false
	}
}

func functionDirectlyReferences(block air.Block, function air.FunctionID) bool {
	found := false
	walkBlockExprs(block, func(expr air.Expr) {
		if found {
			return
		}
		switch expr.Kind {
		case air.ExprCall, air.ExprFunctionRef:
			found = expr.Function == function
		}
	})
	return found
}

func walkBlockExprs(block air.Block, visit func(air.Expr)) {
	for _, stmt := range block.Stmts {
		if stmt.Value != nil {
			walkExpr(*stmt.Value, visit)
		}
		if stmt.Expr != nil {
			walkExpr(*stmt.Expr, visit)
		}
		if stmt.Target != nil {
			walkExpr(*stmt.Target, visit)
		}
		if stmt.Condition != nil {
			walkExpr(*stmt.Condition, visit)
		}
		walkBlockExprs(stmt.Body, visit)
	}
	if block.Result != nil {
		walkExpr(*block.Result, visit)
	}
}

func walkExpr(expr air.Expr, visit func(air.Expr)) {
	visit(expr)
	for i := range expr.Args {
		walkExpr(expr.Args[i], visit)
	}
	for i := range expr.Entries {
		walkExpr(expr.Entries[i].Key, visit)
		walkExpr(expr.Entries[i].Value, visit)
	}
	for i := range expr.Fields {
		walkExpr(expr.Fields[i].Value, visit)
	}
	if expr.Target != nil {
		walkExpr(*expr.Target, visit)
	}
	if expr.Left != nil {
		walkExpr(*expr.Left, visit)
	}
	if expr.Right != nil {
		walkExpr(*expr.Right, visit)
	}
	if expr.Condition != nil {
		walkExpr(*expr.Condition, visit)
	}
	walkBlockExprs(expr.Body, visit)
	walkBlockExprs(expr.Then, visit)
	walkBlockExprs(expr.Else, visit)
	walkBlockExprs(expr.CatchAll, visit)
	walkBlockExprs(expr.Some, visit)
	walkBlockExprs(expr.None, visit)
	walkBlockExprs(expr.Ok, visit)
	walkBlockExprs(expr.Err, visit)
	walkBlockExprs(expr.Catch, visit)
	for i := range expr.EnumCases {
		walkBlockExprs(expr.EnumCases[i].Body, visit)
	}
	for i := range expr.IntCases {
		walkBlockExprs(expr.IntCases[i].Body, visit)
	}
	for i := range expr.StrCases {
		walkBlockExprs(expr.StrCases[i].Body, visit)
	}
	for i := range expr.RangeCases {
		walkBlockExprs(expr.RangeCases[i].Body, visit)
	}
	for i := range expr.UnionCases {
		walkBlockExprs(expr.UnionCases[i].Body, visit)
	}
	for i := range expr.SelectCases {
		arm := expr.SelectCases[i]
		if arm.Channel != nil {
			walkExpr(*arm.Channel, visit)
		}
		if arm.Value != nil {
			walkExpr(*arm.Value, visit)
		}
		walkBlockExprs(arm.Body, visit)
	}
}

func validFunctionID(program *air.Program, id air.FunctionID) bool {
	return id >= 0 && int(id) < len(program.Functions)
}

func validTypeID(program *air.Program, id air.TypeID) bool {
	return id > 0 && int(id) <= len(program.Types)
}

func validTraitID(program *air.Program, id air.TraitID) bool {
	return id >= 0 && int(id) < len(program.Traits)
}

func validImplID(program *air.Program, id air.ImplID) bool {
	return id >= 0 && int(id) < len(program.Impls)
}

func (l *lowerer) canDefineMethodsOnType(info air.TypeInfo) bool {
	if !l.useModulePackages {
		return true
	}
	owner, ok := l.ownerModuleForType(info.ID)
	return !ok || owner == l.currentModule
}

// writeJSONDecodePrimitiveListLoop emits specialized loops for primitive JSON arrays.
// The generic element-helper path eagerly constructs item paths with fmt.Sprintf
// for every successful element. Primitive lists are common and can validate tokens
// directly, keeping detailed item paths only on error. A small default capacity
// avoids repeated growth for typical short JSON arrays while preserving [] for
// empty arrays instead of a nil slice.
func (l *lowerer) buildReservedGoIdentifiers() map[string]bool {
	return map[string]bool{}
}
