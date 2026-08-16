package gotarget

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
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

type importAliasKey struct {
	alias string
	path  string
}

type lowerer struct {
	program                 *air.Program
	packageName             string
	tempCounter             int
	currentImports          map[string]string
	resolvedImportAliases   map[importAliasKey]string
	currentModule           air.ModuleID
	importErr               error
	reservedGoIdentifiers   map[string]bool
	topLevelReserved        map[string]bool
	localNameCache          map[air.FunctionID]map[air.LocalID]string
	goTypeCache             map[air.TypeID]ast.Expr
	identCache              map[string]*ast.Ident
	declaredLocals          map[air.LocalID]bool
	runtimeHelpers          map[string]bool
	projectInfo             *checker.ProjectInfo
	generatedModulePath     string
	inlineClosures          map[air.FunctionID]bool
	goMethodCollisions      map[string]bool
	functionComparable      map[air.FunctionID]map[string]bool
	functionModules         map[air.FunctionID]air.ModuleID
	moduleByPath            map[string]air.ModuleID
	typeModulePaths         []string
	typeOwnerModules        []air.ModuleID
	typeHasOwner            []bool
	typesByModule           map[air.ModuleID][]*air.TypeInfo
	ownerlessTypes          []*air.TypeInfo
	declaredTypes           []bool
	declaredTypeCounts      map[air.ModuleID]int
	emitTypeOwnerModules    []air.ModuleID
	emitTypeHasOwner        []bool
	suppressMain            bool
	includeTests            bool
	useModulePackages       bool
	forceValueResultReturns bool
	namePlan                *namePlan

	// When the entry root lives in a module named `main` (main.ard) that no
	// other module imports, that module is emitted as the root `package main`
	// with the root lowered to `func main()`, instead of an importable package
	// plus a separate synthetic main (ADR 0031).
	entryAsMainPackage  bool
	entryMainModuleID   air.ModuleID
	entryMainFunctionID air.FunctionID
}

func (l *lowerer) ident(name string) *ast.Ident {
	if ident, ok := l.identCache[name]; ok {
		return ident
	}
	if l.identCache == nil {
		l.identCache = map[string]*ast.Ident{}
	}
	ident := ast.NewIdent(name)
	l.identCache[name] = ident
	return ident
}

func lowerProgram(program *air.Program, options Options) (map[string]*ast.File, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	l := &lowerer{program: program, packageName: defaultPackageName(options.PackageName), runtimeHelpers: map[string]bool{}, projectInfo: options.ProjectInfo, generatedModulePath: generatedModulePath(options.ProjectInfo), suppressMain: options.SuppressMain, includeTests: options.IncludeTests, useModulePackages: true}
	l.indexModuleOwnership()
	l.inlineClosures = l.collectInlineClosureFunctions()
	l.functionComparable = l.collectFunctionComparableTypeParams()
	l.goMethodCollisions = l.collectGoMethodCollisions()
	l.functionModules = l.collectFunctionEmitModules()
	l.namePlan = newNamePlan(l)
	l.topLevelReserved = l.namePlan.localReserved
	l.reservedGoIdentifiers = l.buildReservedGoIdentifiers()
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
	importPath := l.moduleImportPath(entryModuleID)
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: l.ident(alias), Sel: l.ident(l.functionName(fn))}}
	var stmt ast.Stmt
	if l.isVoidType(fn.Signature.Return) {
		stmt = &ast.ExprStmt{X: call}
	} else {
		stmt = &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}}
	}
	importDecl := &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{&ast.ImportSpec{
		Name: l.ident(alias),
		Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(importPath)},
	}}}
	mainDecl := &ast.FuncDecl{Name: l.ident("main"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}}
	return &ast.File{Name: l.ident("main"), Decls: []ast.Decl{importDecl, mainDecl}}, nil
}

func (l *lowerer) lowerModule(module air.Module) (*ast.File, error) {
	previousModule := l.currentModule
	l.currentModule = module.ID
	defer func() { l.currentModule = previousModule }()
	l.currentImports = map[string]string{}
	l.resolvedImportAliases = map[importAliasKey]string{}
	l.goTypeCache = map[air.TypeID]ast.Expr{}
	l.importErr = nil
	l.runtimeHelpers = map[string]bool{}
	decls := []ast.Decl{}
	rootID, hasRoot := findRootFunction(l.program)
	mainModuleID := l.mainModuleID(rootID, hasRoot)
	for _, typ := range l.typesForModule(module.ID, mainModuleID) {
		typeDecls, err := l.lowerTypeDecls(*typ)
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
					Name: l.ident(alias),
					Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", usedImports[alias])},
				})
			}
			decls = append([]ast.Decl{importDecl}, decls...)
		}
	}
	return &ast.File{Name: l.ident(l.modulePackageName(module.ID, mainModuleID)), Decls: decls}, nil
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
	case air.TypeList, air.TypeSlice, air.TypeMaybe:
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
	return filepath.Join(l.modulePackageDir(module.ID), modulePackageFileName(l.program, module.ID))
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
	return l.functionName(fn)
}

func modulePackageFileName(program *air.Program, module air.ModuleID) string {
	name := modulePackageName(program, module)
	if strings.HasSuffix(name, "_test") {
		name += "_ard"
	}
	return name + ".go"
}

// typesForModule returns pointers to avoid copying large TypeInfo values. Go
// lowering only appends to Program.Types; it never mutates existing entries, so
// pointers remain valid snapshots even if an append moves the backing array.
func (l *lowerer) typesForModule(moduleID air.ModuleID, mainModuleID air.ModuleID) []*air.TypeInfo {
	if l.typesByModule != nil {
		types := l.typesByModule[moduleID]
		if moduleID != mainModuleID || len(l.ownerlessTypes) == 0 {
			return types
		}
		// Declared types lead in module order. Undeclared owned and ownerless
		// types then retain Program.Types order, matching the uncached path.
		declaredCount := l.declaredTypeCounts[moduleID]
		withOwnerless := make([]*air.TypeInfo, 0, len(types)+len(l.ownerlessTypes))
		withOwnerless = append(withOwnerless, types[:declaredCount]...)
		for i := range l.program.Types {
			typ := &l.program.Types[i]
			if l.declaredTypes[typ.ID] {
				continue
			}
			if !l.emitTypeHasOwner[typ.ID] || l.emitTypeOwnerModules[typ.ID] == moduleID {
				withOwnerless = append(withOwnerless, typ)
			}
		}
		return withOwnerless
	}
	declaredInAnyModule := map[air.TypeID]bool{}
	for _, module := range l.program.Modules {
		for _, typeID := range module.Types {
			declaredInAnyModule[typeID] = true
		}
	}
	out := []*air.TypeInfo{}
	if int(moduleID) >= 0 && int(moduleID) < len(l.program.Modules) {
		for _, typeID := range l.program.Modules[moduleID].Types {
			if validTypeID(l.program, typeID) {
				out = append(out, &l.program.Types[typeID-1])
			}
		}
	}
	for i := range l.program.Types {
		typ := &l.program.Types[i]
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

func (l *lowerer) indexModuleOwnership() {
	l.moduleByPath = make(map[string]air.ModuleID, len(l.program.Modules))
	l.typeModulePaths = make([]string, len(l.program.Types)+1)
	l.typeOwnerModules = make([]air.ModuleID, len(l.program.Types)+1)
	l.typeHasOwner = make([]bool, len(l.program.Types)+1)
	for _, module := range l.program.Modules {
		if module.Path != "" {
			if _, exists := l.moduleByPath[module.Path]; !exists {
				l.moduleByPath[module.Path] = module.ID
			}
		}
	}
	for i := range l.program.Types {
		typ := &l.program.Types[i]
		if typ.ModulePath != "" {
			l.typeModulePaths[typ.ID] = typ.ModulePath
		}
	}
	for _, module := range l.program.Modules {
		for _, typeID := range module.Types {
			if validTypeID(l.program, typeID) && l.typeModulePaths[typeID] == "" {
				l.typeModulePaths[typeID] = module.Path
			}
		}
	}
	for typeID, modulePath := range l.typeModulePaths {
		if moduleID, ok := l.moduleByPath[modulePath]; ok {
			l.typeOwnerModules[typeID] = moduleID
			l.typeHasOwner[typeID] = true
		}
	}
	l.indexTypesByModule()
}

func (l *lowerer) indexTypesByModule() {
	l.typesByModule = make(map[air.ModuleID][]*air.TypeInfo, len(l.program.Modules))
	l.declaredTypes = make([]bool, len(l.program.Types)+1)
	l.declaredTypeCounts = make(map[air.ModuleID]int, len(l.program.Modules))
	l.emitTypeOwnerModules = make([]air.ModuleID, len(l.program.Types)+1)
	l.emitTypeHasOwner = make([]bool, len(l.program.Types)+1)
	for _, module := range l.program.Modules {
		for _, typeID := range module.Types {
			if validTypeID(l.program, typeID) {
				l.declaredTypes[typeID] = true
				l.declaredTypeCounts[module.ID]++
				l.typesByModule[module.ID] = append(l.typesByModule[module.ID], &l.program.Types[typeID-1])
			}
		}
	}
	for i := range l.program.Types {
		typ := &l.program.Types[i]
		if l.declaredTypes[typ.ID] {
			continue
		}
		if typ.Kind == air.TypeTraitObject {
			if moduleID, ok := l.ownerModuleForTrait(typ.Trait); ok {
				l.emitTypeOwnerModules[typ.ID] = moduleID
				l.emitTypeHasOwner[typ.ID] = true
				l.typesByModule[moduleID] = append(l.typesByModule[moduleID], typ)
				continue
			}
		}
		if moduleID, ok := l.ownerModuleForType(typ.ID); ok {
			l.emitTypeOwnerModules[typ.ID] = moduleID
			l.emitTypeHasOwner[typ.ID] = true
			l.typesByModule[moduleID] = append(l.typesByModule[moduleID], typ)
			continue
		}
		l.ownerlessTypes = append(l.ownerlessTypes, typ)
	}
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
	if l.typeHasOwner == nil {
		return l.moduleForPath(l.modulePathForType(typeID))
	}
	return l.typeOwnerModules[typeID], l.typeHasOwner[typeID]
}

func (l *lowerer) ownerModuleForImpl(impl air.Impl) (air.ModuleID, bool) {
	if owner, ok := l.ownerModuleForType(impl.ForType); ok {
		return owner, true
	}
	for _, methodID := range impl.Methods {
		if validFunctionID(l.program, methodID) {
			return l.functionModule(l.program.Functions[methodID]), true
		}
	}
	return 0, false
}

func (l *lowerer) moduleForPath(modulePath string) (air.ModuleID, bool) {
	if modulePath == "" {
		return 0, false
	}
	if l.moduleByPath != nil {
		module, ok := l.moduleByPath[modulePath]
		return module, ok
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
			fieldType, err := l.goType(field.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{l.ident(l.goFieldName(typ, field.Name))},
				Type:  fieldType,
				Tag:   &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("`json:%q`", field.Name)},
			})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: l.ident(l.typeName(typ)), TypeParams: l.goTypeParamList(typ), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeUnion:
		fields := []*ast.Field{{Names: []*ast.Ident{l.ident(unionTagFieldName(typ))}, Type: l.ident("uint32")}}
		for _, member := range typ.Members {
			memberType, err := l.goType(member.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{Names: []*ast.Ident{l.ident(unionMemberFieldName(typ, member))}, Type: memberType})
		}
		unionDecl := &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: l.ident(l.typeName(typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}
		return []ast.Decl{unionDecl, l.unionMarshalJSONDecl(typ)}, nil
	case air.TypeTraitObject:
		if l.isBuiltinErrorType(typ.ID) {
			return nil, nil
		}
		return l.lowerTraitObjectDecls(typ)
	case air.TypeEnum:
		typeSpec := &ast.TypeSpec{Name: l.ident(l.typeName(typ)), Type: l.ident("int")}
		specs := []ast.Spec{typeSpec}
		for _, variant := range typ.Variants {
			value := ast.Expr(&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", variant.Discriminant)})
			specs = append(specs, &ast.ValueSpec{Names: []*ast.Ident{l.ident(l.enumVariantName(typ, variant))}, Type: l.ident(l.typeName(typ)), Values: []ast.Expr{value}})
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
	decls := []ast.Decl{}
	for _, impl := range l.program.Impls {
		owner, ok := l.ownerModuleForImpl(impl)
		if !ok || owner != l.currentModule || !l.traitHasReferenceTypeUse(impl.Trait) || !validTraitID(l.program, impl.Trait) {
			continue
		}
		trait := l.program.Traits[impl.Trait]
		if l.usesNativeTraitInterface(l.traitObjectTypeID(trait.ID)) {
			continue
		}
		methods, err := l.mutableTraitDispatchMethodDecls(trait, impl)
		if err != nil {
			return nil, err
		}
		decls = append(decls, methods...)
	}
	return decls, nil
}

func (l *lowerer) mutableTraitDispatchMethodDecls(trait air.Trait, impl air.Impl) ([]ast.Decl, error) {
	if !validTypeID(l.program, impl.ForType) {
		return nil, fmt.Errorf("mutable trait dispatch has invalid impl type %d", impl.ForType)
	}
	concreteType, err := l.goType(impl.ForType)
	if err != nil {
		return nil, err
	}
	pointerReceiver := l.implRequiresPointerReceiver(impl.ID)
	receiverType := concreteType
	if pointerReceiver {
		receiverType = &ast.StarExpr{X: concreteType}
	}
	decls := make([]ast.Decl, 0, len(trait.Methods))
	for methodIndex, traitMethod := range trait.Methods {
		if methodIndex >= len(impl.Methods) || !validFunctionID(l.program, impl.Methods[methodIndex]) {
			return nil, fmt.Errorf("impl %d missing method %d for trait %s", impl.ID, methodIndex, trait.Name)
		}
		methodFn := l.program.Functions[impl.Methods[methodIndex]]
		methodTypeExpr, err := l.mutableTraitMethodFuncType(traitMethod)
		if err != nil {
			return nil, err
		}
		methodType := methodTypeExpr.(*ast.FuncType)
		receiver := ast.Expr(l.ident("receiver"))
		if pointerReceiver && len(methodFn.Signature.Params) > 0 && !l.isReferenceType(methodFn.Signature.Params[0].Type) {
			receiver = &ast.StarExpr{X: receiver}
		}
		args := []ast.Expr{}
		if len(methodFn.Signature.Params) > 0 {
			args = append(args, receiver)
		}
		for index := range traitMethod.Signature.Params {
			args = append(args, l.ident(fmt.Sprintf("arg%d", index)))
		}
		call := l.functionCallExpr(methodFn, args, nil)
		body := []ast.Stmt{}
		if l.isVoidType(traitMethod.Signature.Return) {
			body = append(body, &ast.ExprStmt{X: call})
		} else {
			body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
		}
		decls = append(decls, &ast.FuncDecl{
			Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{l.ident("receiver")}, Type: receiverType}}},
			Name: l.ident(mutableTraitDispatchMethodName(trait.ID, methodIndex)),
			Type: methodType,
			Body: &ast.BlockStmt{List: body},
		})
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
				Args: []ast.Expr{&ast.SelectorExpr{X: l.ident(recv), Sel: l.ident(unionMemberFieldName(typ, member))}},
			}}}},
		})
	}
	body := &ast.BlockStmt{List: []ast.Stmt{
		&ast.SwitchStmt{Tag: &ast.SelectorExpr{X: l.ident(recv), Sel: l.ident(unionTagFieldName(typ))}, Body: &ast.BlockStmt{List: cases}},
		&ast.ReturnStmt{Results: []ast.Expr{l.ident("nil"), &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Errorf"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"invalid union tag"`}}}}},
	}}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{l.ident(recv)}, Type: l.ident(l.typeName(typ))}}},
		Name: l.ident("MarshalJSON"),
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Elt: l.ident("byte")}}, {Type: l.ident("error")}}}},
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
	// Mutable trait values keep raw concrete or trait-storage pointers under any.
	// The trait owner emits only the helpers needed to load/project those pointer
	// shapes; fallback implementations add collision-proof dispatch methods.
	if l.traitHasMutableTraitUse(trait.ID) || l.traitHasReferenceTypeUse(trait.ID) {
		mutableDecls, err := l.lowerMutableTraitRefTypeDecls(trait)
		if err != nil {
			return nil, err
		}
		decls = append(decls, mutableDecls...)
	}
	return decls, nil
}

// traitHasReferenceTypeUse reports whether any first-class reference type in
// the program points at this trait (`mut Trait` locals, parameters, fields,
// returns, or containers intern such a type).
func (l *lowerer) traitHasReferenceTypeUse(traitID air.TraitID) bool {
	for _, typ := range l.program.Types {
		if typ.Kind == air.TypeReference && l.typeIDIsTrait(typ.Elem, traitID) {
			return true
		}
	}
	return false
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
		methods = append(methods, &ast.Field{Names: []*ast.Ident{l.ident(methodName)}, Type: methodType})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: l.ident(l.traitInterfaceTypeName(trait)), Type: &ast.InterfaceType{Methods: &ast.FieldList{List: methods}}}}}, true, nil
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
	// Ordinary Trait values and mut Trait references have distinct AIR types and
	// Go representations. Mutable use must not force otherwise representable
	// ordinary values back to any/type switches.
	if !l.isTraitObjectType(typeID) {
		return false
	}
	traitID := l.program.Types[typeID-1].Trait
	if !l.traitInterfaceAvailable(traitID) {
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
			if _, ok := l.directGoMethodName(methodFn); !ok {
				return false
			}
		}
	}
	return true
}

func (l *lowerer) traitHasMutableTraitUse(traitID air.TraitID) bool {
	for _, fn := range l.program.Functions {
		for _, param := range fn.Signature.Params {
			if l.referenceTypeIsTrait(param.Type, traitID) {
				return true
			}
		}
	}
	for _, typ := range l.program.Types {
		for _, field := range typ.Fields {
			if l.referenceTypeIsTrait(field.Type, traitID) {
				return true
			}
		}
		for _, paramTypeID := range typ.Params {
			if l.referenceTypeIsTrait(paramTypeID, traitID) {
				return true
			}
		}
	}
	return false
}

func (l *lowerer) referenceTypeIsTrait(typeID air.TypeID, traitID air.TraitID) bool {
	if !l.isReferenceType(typeID) {
		return false
	}
	return l.typeIDIsTrait(l.program.Types[typeID-1].Elem, traitID)
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
	results, err := l.goSignatureReturnFields(method.Signature, method.Signature.Return)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		fnType.Results = &ast.FieldList{List: results}
	}
	return fnType, nil
}

func (l *lowerer) lowerMutableTraitRefTypeDecls(trait air.Trait) ([]ast.Decl, error) {
	traitTypeID := l.traitObjectTypeID(trait.ID)
	if traitTypeID == air.NoType {
		return nil, fmt.Errorf("missing trait object type for %s", trait.Name)
	}
	ordinaryTraitType, err := l.goType(traitTypeID)
	if err != nil {
		return nil, err
	}
	native := l.usesNativeTraitInterface(traitTypeID)
	decls := []ast.Decl{}
	if !native {
		dispatch, err := l.mutableTraitDispatchDecl(trait)
		if err != nil {
			return nil, err
		}
		decls = append(decls, dispatch)
	}
	decls = append(decls,
		l.mutableTraitCurrentDecl(trait, ordinaryTraitType, native),
		l.mutableTraitLoadDecl(trait, ordinaryTraitType, native),
		l.mutableTraitProjectDecl(trait, ordinaryTraitType, native),
	)
	return decls, nil
}

func (l *lowerer) mutableTraitDispatchDecl(trait air.Trait) (ast.Decl, error) {
	methods := make([]*ast.Field, 0, len(trait.Methods))
	for index, method := range trait.Methods {
		methodType, err := l.mutableTraitMethodFuncType(method)
		if err != nil {
			return nil, err
		}
		methods = append(methods, &ast.Field{
			Names: []*ast.Ident{l.ident(mutableTraitDispatchMethodName(trait.ID, index))},
			Type:  methodType,
		})
	}
	return &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
		Name: l.ident(mutableTraitDispatchTypeName(trait)),
		Type: &ast.InterfaceType{Methods: &ast.FieldList{List: methods}},
	}}}, nil
}

func (l *lowerer) mutableTraitCurrentDecl(trait air.Trait, ordinaryTraitType ast.Expr, native bool) ast.Decl {
	reference := l.ident("reference")
	slot := l.ident("slot")
	ok := l.ident("ok")
	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{slot, ok},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.TypeAssertExpr{X: reference, Type: &ast.StarExpr{X: ordinaryTraitType}}},
		},
		&ast.IfStmt{
			Cond: ok,
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.StarExpr{X: slot}}}}},
		},
	}
	result := ast.Expr(reference)
	if native {
		result = &ast.TypeAssertExpr{X: reference, Type: ordinaryTraitType}
	}
	body = append(body, &ast.ReturnStmt{Results: []ast.Expr{result}})
	return &ast.FuncDecl{
		Name: l.ident(mutableTraitCurrentFuncName(trait)),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{reference}, Type: l.ident("any")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ordinaryTraitType}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

func (l *lowerer) mutableTraitLoadDecl(trait air.Trait, ordinaryTraitType ast.Expr, native bool) ast.Decl {
	reference := l.ident("reference")
	current := l.ident("current")
	value := l.ident("value")
	copyName := l.ident("copy")
	result := l.ident("result")
	ok := l.ident("ok")
	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{current},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident(mutableTraitCurrentFuncName(trait)), Args: []ast.Expr{reference}}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{value},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: l.qualified("reflect", "reflect", "ValueOf"), Args: []ast.Expr{current}}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.CallExpr{Fun: &ast.SelectorExpr{X: value, Sel: l.ident("Kind")}},
				Op: token.NEQ,
				Y:  l.qualified("reflect", "reflect", "Pointer"),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{current}}}},
		},
		&ast.IfStmt{
			Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: value, Sel: l.ident("IsNil")}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun:  l.ident("panic"),
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"nil mutable trait reference"`}},
			}}}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{copyName},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  l.qualified("reflect", "reflect", "New"),
				Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: value, Sel: l.ident("Elem")}}, Sel: l.ident("Type")}}},
			}},
		},
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: copyName, Sel: l.ident("Elem")}}, Sel: l.ident("Set")},
			Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: value, Sel: l.ident("Elem")}}},
		}},
	}
	pointerCopy := ast.Expr(&ast.CallExpr{Fun: &ast.SelectorExpr{X: copyName, Sel: l.ident("Interface")}})
	valueCopy := &ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: copyName, Sel: l.ident("Elem")}}, Sel: l.ident("Interface")}}
	copyType := ordinaryTraitType
	if !native {
		copyType = l.ident(mutableTraitDispatchTypeName(trait))
	}
	body = append(body,
		&ast.IfStmt{
			Init: &ast.AssignStmt{
				Lhs: []ast.Expr{result, ok},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.TypeAssertExpr{X: valueCopy, Type: copyType}},
			},
			Cond: ok,
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{result}}}},
		},
	)
	if native {
		pointerCopy = &ast.TypeAssertExpr{X: pointerCopy, Type: ordinaryTraitType}
	}
	body = append(body, &ast.ReturnStmt{Results: []ast.Expr{pointerCopy}})
	return &ast.FuncDecl{
		Name: l.ident(mutableTraitLoadFuncName(trait)),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{reference}, Type: l.ident("any")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ordinaryTraitType}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

func (l *lowerer) mutableTraitProjectDecl(trait air.Trait, ordinaryTraitType ast.Expr, native bool) ast.Decl {
	reference := l.ident("reference")
	slot := l.ident("slot")
	ok := l.ident("ok")
	current := l.ident("current")
	value := l.ident("value")
	projected := l.ident("projected")
	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{slot, ok},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.TypeAssertExpr{X: reference, Type: &ast.StarExpr{X: ordinaryTraitType}}},
		},
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: ok},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{reference}}}},
		},
		&ast.AssignStmt{Lhs: []ast.Expr{current}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.StarExpr{X: slot}}},
		&ast.AssignStmt{
			Lhs: []ast.Expr{value},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: l.qualified("reflect", "reflect", "ValueOf"), Args: []ast.Expr{current}}},
		},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.CallExpr{Fun: &ast.SelectorExpr{X: value, Sel: l.ident("Kind")}},
				Op: token.EQL,
				Y:  l.qualified("reflect", "reflect", "Pointer"),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{current}}}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{projected},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  l.qualified("reflect", "reflect", "New"),
				Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: value, Sel: l.ident("Type")}}},
			}},
		},
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: projected, Sel: l.ident("Elem")}}, Sel: l.ident("Set")},
			Args: []ast.Expr{value},
		}},
	}
	assigned := ast.Expr(&ast.CallExpr{Fun: &ast.SelectorExpr{X: projected, Sel: l.ident("Interface")}})
	if native {
		assigned = &ast.TypeAssertExpr{X: assigned, Type: ordinaryTraitType}
	}
	body = append(body,
		&ast.AssignStmt{Lhs: []ast.Expr{&ast.StarExpr{X: slot}}, Tok: token.ASSIGN, Rhs: []ast.Expr{assigned}},
		&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: projected, Sel: l.ident("Interface")}}}},
	)
	return &ast.FuncDecl{
		Name: l.ident(mutableTraitProjectFuncName(trait)),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{reference}, Type: l.ident("any")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: l.ident("any")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

func (l *lowerer) mutableTraitMethodFuncType(method air.TraitMethod) (ast.Expr, error) {
	params := make([]*ast.Field, 0, len(method.Signature.Params))
	for i, param := range method.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{Names: []*ast.Ident{l.ident(fmt.Sprintf("arg%d", i))}, Type: paramType})
	}
	fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	results, err := l.goSignatureReturnFields(method.Signature, method.Signature.Return)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		fnType.Results = &ast.FieldList{List: results}
	}
	return fnType, nil
}

func (l *lowerer) traitObjectTypeID(traitID air.TraitID) air.TypeID {
	for _, info := range l.program.Types {
		if info.Kind == air.TypeTraitObject && info.Trait == traitID {
			return info.ID
		}
	}
	return air.NoType
}

func (l *lowerer) traitInterfaceTypeName(trait air.Trait) string {
	if l.namePlan != nil {
		if name, ok := l.namePlan.traitNames[trait.ID]; ok {
			return name
		}
	}
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
	return l.traitOwnedTypeExpr(trait, l.traitInterfaceTypeName(trait))
}

func (l *lowerer) mutableTraitCurrentExpr(trait air.Trait, reference ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: l.traitOwnedTypeExpr(trait, mutableTraitCurrentFuncName(trait)), Args: []ast.Expr{reference}}
}

func (l *lowerer) mutableTraitLoadExpr(trait air.Trait, reference ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: l.traitOwnedTypeExpr(trait, mutableTraitLoadFuncName(trait)), Args: []ast.Expr{reference}}
}

func (l *lowerer) mutableTraitProjectExpr(trait air.Trait, reference ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: l.traitOwnedTypeExpr(trait, mutableTraitProjectFuncName(trait)), Args: []ast.Expr{reference}}
}

func (l *lowerer) traitOwnedTypeExpr(trait air.Trait, name string) ast.Expr {
	if !l.useModulePackages {
		return l.ident(name)
	}
	owner, ok := l.ownerModuleForTrait(trait.ID)
	if !ok || owner == l.currentModule {
		return l.ident(name)
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
	comparable := l.comparableTypeParams(air.Signature{Params: fields}, nil, nil, nil)
	return l.typeParamFieldList(typ.TypeParams, comparable)
}

func (l *lowerer) namedTypeExpr(info air.TypeInfo) ast.Expr {
	// A generic type parameter lowers to its Go identifier inside the generic
	// definition's scope (ADR 0031).
	if info.Kind == air.TypeParam {
		return l.ident(info.Name)
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
	name := l.typeName(info)
	if !l.useModulePackages {
		return l.ident(name)
	}
	owner, ok := l.ownerModuleForType(info.ID)
	if !ok || owner == l.currentModule {
		return l.ident(name)
	}
	return l.moduleQualified(owner, name)
}

func (l *lowerer) compositeTypeExpr(info air.TypeInfo) ast.Expr {
	return l.namedTypeExpr(info)
}

func (l *lowerer) enumVariantExpr(typ air.TypeInfo, variant air.VariantInfo) ast.Expr {
	name := l.enumVariantName(typ, variant)
	if !l.useModulePackages {
		return l.ident(name)
	}
	owner, ok := l.ownerModuleForType(typ.ID)
	if !ok || owner == l.currentModule {
		return l.ident(name)
	}
	return l.moduleQualified(owner, name)
}

func (l *lowerer) functionExpr(fn air.Function) ast.Expr {
	name := l.goFunctionName(fn)
	module := l.functionModule(fn)
	if !l.useModulePackages || module == l.currentModule {
		return l.ident(name)
	}
	return l.moduleQualified(module, name)
}

func (l *lowerer) globalExpr(global air.Global) ast.Expr {
	name := l.globalName(global)
	if !l.useModulePackages || global.Module == l.currentModule {
		return l.ident(name)
	}
	return l.moduleQualified(global.Module, name)
}

func (l *lowerer) modulePackageDir(module air.ModuleID) string {
	projectName := ""
	if l.projectInfo != nil {
		projectName = l.projectInfo.ProjectName
	}
	return modulePackageDirWithProject(l.program, module, projectName)
}

func (l *lowerer) moduleQualified(module air.ModuleID, name string) ast.Expr {
	return l.qualified(l.moduleImportAlias(module), l.moduleImportPath(module), name)
}

func (l *lowerer) moduleImportPath(module air.ModuleID) string {
	projectName := ""
	if l.projectInfo != nil {
		projectName = l.projectInfo.ProjectName
	}
	return moduleImportPathForProject(l.program, module, l.generatedModulePath, projectName)
}

func (l *lowerer) moduleImportAlias(module air.ModuleID) string {
	base := modulePackageName(l.program, module)
	importPath := l.moduleImportPath(module)
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

func mutableTraitDispatchTypeName(trait air.Trait) string {
	return fmt.Sprintf("%sMutTraitDispatch_%s_%d", mutableTraitNamePrefix(trait), sanitizeName(trait.Name), trait.ID)
}

func mutableTraitCurrentFuncName(trait air.Trait) string {
	return fmt.Sprintf("%sMutTraitCurrent_%s_%d", mutableTraitNamePrefix(trait), sanitizeName(trait.Name), trait.ID)
}

func mutableTraitLoadFuncName(trait air.Trait) string {
	return fmt.Sprintf("%sMutTraitLoad_%s_%d", mutableTraitNamePrefix(trait), sanitizeName(trait.Name), trait.ID)
}

func mutableTraitProjectFuncName(trait air.Trait) string {
	return fmt.Sprintf("%sMutTraitProject_%s_%d", mutableTraitNamePrefix(trait), sanitizeName(trait.Name), trait.ID)
}

func mutableTraitNamePrefix(trait air.Trait) string {
	if trait.Private {
		return "ard"
	}
	return "Ard"
}

func mutableTraitDispatchMethodName(trait air.TraitID, methodIndex int) string {
	return fmt.Sprintf("ArdMutTraitMethod_%d_%d", trait, methodIndex)
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
		// Wrap statement-producing initializers (match, try, etc.) in an
		// immediately-invoked function so they remain valid Go package
		// variable initializers.
		body := append([]ast.Stmt{}, value.stmts...)
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{valueExpr}})
		valueExpr = &ast.CallExpr{Fun: &ast.FuncLit{
			Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: globalType}}}},
			Body: &ast.BlockStmt{List: body},
		}}
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names:  []*ast.Ident{l.ident(l.globalName(global))},
		Type:   globalType,
		Values: []ast.Expr{valueExpr},
	}}}, nil
}

func (l *lowerer) lowerFunction(fn air.Function) (ast.Decl, error) {
	l.declaredLocals = map[air.LocalID]bool{}
	methodName, directMethod := l.directGoMethodName(fn)
	if fn.RequiredGoMethodName != "" && !directMethod {
		return nil, fmt.Errorf("required Go interface method %s.%s cannot be emitted as a receiver method", fn.Name, fn.RequiredGoMethodName)
	}
	params := []*ast.Field{}
	for _, capture := range fn.Captures {
		captureType, err := l.goType(capture.Type)
		if err != nil {
			return nil, err
		}
		if capture.Mode == air.CaptureSlot {
			captureType = &ast.StarExpr{X: captureType}
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{l.ident(l.localName(fn, capture.Local))},
			Type:  captureType,
		})
		l.declaredLocals[capture.Local] = true
	}
	paramStart := 0
	var receiver *ast.FieldList
	if directMethod {
		receiverType, err := l.goMethodReceiverType(fn, nil)
		if err != nil {
			return nil, err
		}
		receiver = &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{l.ident(l.localName(fn, 0))},
			Type:  receiverType,
		}}}
		paramStart = 1
	}
	for i := paramStart; i < len(fn.Signature.Params); i++ {
		paramType, err := l.goFunctionParamType(fn, fn.Signature.Params[i])
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{
			Names: []*ast.Ident{l.ident(l.localName(fn, air.LocalID(i)))},
			Type:  paramType,
		})
	}
	for _, local := range fn.Locals {
		if int(local.ID) < len(fn.Signature.Params) {
			l.declaredLocals[local.ID] = true
		}
	}
	returnTypeID := fn.Signature.Return
	body, err := l.lowerBlock(fn, fn.Body, returnTypeID)
	if err != nil {
		return nil, err
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !directMethod {
		funcType.TypeParams = l.goFuncTypeParamList(fn)
	}
	results, err := l.goSignatureReturnFields(fn.Signature, returnTypeID)
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		funcType.Results = &ast.FieldList{List: results}
	}
	name := l.goFunctionName(fn)
	if directMethod {
		name = methodName
	}
	return &ast.FuncDecl{
		Recv: receiver,
		Name: l.ident(name),
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
		return &ast.IndexExpr{X: fun, Index: l.ident(names[0])}
	}
	indices := make([]ast.Expr, len(names))
	for i, n := range names {
		indices[i] = l.ident(n)
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
	comparable := l.functionComparableTypeParams(fn)
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
		fields[i] = &ast.Field{Names: []*ast.Ident{l.ident(p)}, Type: l.ident(constraint)}
	}
	return &ast.FieldList{List: fields}
}

// comparableTypeParams returns the set of type parameter names that appear as a
// map key within the given signature and locals, and therefore require the
// `comparable` constraint.
func (l *lowerer) comparableTypeParams(signature air.Signature, locals []air.Local, body *air.Block, comparableRoots []air.TypeID) map[string]bool {
	result := map[string]bool{}
	seen := map[air.TypeID]uint8{}
	var walk func(id air.TypeID, requireComparable bool)
	walk = func(id air.TypeID, requireComparable bool) {
		if id == air.NoType {
			return
		}
		flag := uint8(1)
		if requireComparable {
			flag = 2
		}
		if seen[id]&flag != 0 {
			return
		}
		seen[id] |= flag
		info, ok := l.typeInfo(id)
		if !ok {
			return
		}
		if info.Kind == air.TypeParam {
			if requireComparable {
				result[info.Name] = true
			}
			return
		}
		if info.Kind == air.TypeMap {
			walk(info.Key, true)
			walk(info.Value, false)
			return
		}

		structural := requireComparable && (info.Kind == air.TypeStruct || info.Kind == air.TypeFixedArray)
		walk(info.Elem, structural)
		walk(info.Key, false)
		walk(info.Value, false)
		walk(info.Return, false)
		walk(info.Error, false)
		for _, p := range info.Params {
			walk(p, false)
		}
		for _, f := range info.Fields {
			walk(f.Type, structural)
		}
		for _, m := range info.Members {
			walk(m.Type, false)
		}
		for i, ga := range info.GenericArgs {
			requiresComparable := i < len(info.GenericComparable) && info.GenericComparable[i]
			walk(ga, requiresComparable)
		}
	}
	for _, p := range signature.Params {
		walk(p.Type, false)
	}
	walk(signature.Return, false)
	for _, loc := range locals {
		walk(loc.Type, false)
	}
	if body != nil {
		walkBlockExprs(*body, func(expr air.Expr) {
			walk(expr.Type, false)
		})
	}
	for _, root := range comparableRoots {
		walk(root, true)
	}
	return result
}

func (l *lowerer) functionComparableTypeParams(fn air.Function) map[string]bool {
	if comparable, ok := l.functionComparable[fn.ID]; ok {
		return comparable
	}
	return l.comparableTypeParams(fn.Signature, fn.Locals, &fn.Body, nil)
}

func (l *lowerer) collectFunctionComparableTypeParams() map[air.FunctionID]map[string]bool {
	result := make(map[air.FunctionID]map[string]bool, len(l.program.Functions))
	for _, fn := range l.program.Functions {
		result[fn.ID] = l.comparableTypeParams(fn.Signature, fn.Locals, &fn.Body, nil)
	}
	for changed := true; changed; {
		changed = false
		for _, fn := range l.program.Functions {
			walkBlockExprs(fn.Body, func(expr air.Expr) {
				if expr.Kind != air.ExprCall && expr.Kind != air.ExprFunctionRef && expr.Kind != air.ExprMakeClosure {
					return
				}
				if !validFunctionID(l.program, expr.Function) {
					return
				}
				callee := l.program.Functions[expr.Function]
				calleeComparable := result[callee.ID]
				if expr.Kind == air.ExprMakeClosure {
					// Lifted closures inherit the enclosing generic parameter names and
					// are instantiated with those names at creation time.
					for _, typeParam := range callee.TypeParams {
						if calleeComparable[typeParam] && !result[fn.ID][typeParam] {
							result[fn.ID][typeParam] = true
							changed = true
						}
					}
					return
				}
				comparableArgs := []air.TypeID{}
				for i, typeParam := range callee.TypeParams {
					if !calleeComparable[typeParam] || i >= len(expr.TypeArgs) {
						continue
					}
					comparableArgs = append(comparableArgs, expr.TypeArgs[i])
				}
				mapped := l.comparableTypeParams(air.Signature{}, nil, nil, comparableArgs)
				for typeParam := range mapped {
					if !result[fn.ID][typeParam] {
						result[fn.ID][typeParam] = true
						changed = true
					}
				}
			})
		}
	}
	return result
}

func (l *lowerer) directGoMethodName(fn air.Function) (string, bool) {
	key, methodName, ok := l.goMethodKey(fn)
	if !ok || l.goMethodCollisions[key] || len(fn.Captures) != 0 {
		return "", false
	}
	receiverTypeID := fn.Receiver
	if receiverTypeID == air.NoType && len(fn.Signature.Params) > 0 {
		receiverTypeID = fn.Signature.Params[0].Type
	}
	// Go receiver declarations may bind a generic definition's type
	// parameters, but cannot attach methods to a concrete instantiation such as
	// Box[int]. Keep the standalone fallback for that uncommon shape.
	if validTypeID(l.program, receiverTypeID) {
		receiver := l.program.Types[receiverTypeID-1]
		if receiver.Generic != air.NoType && len(fn.TypeParams) == 0 {
			return "", false
		}
	}
	if !l.goMethodReceiverProvidesConstraints(fn, receiverTypeID) {
		return "", false
	}
	return methodName, true
}

func (l *lowerer) goMethodReceiverProvidesConstraints(fn air.Function, receiverTypeID air.TypeID) bool {
	required := l.functionComparableTypeParams(fn)
	if len(required) == 0 {
		return true
	}
	if !validTypeID(l.program, receiverTypeID) {
		return false
	}
	receiver := l.program.Types[receiverTypeID-1]
	if receiver.Generic != air.NoType && validTypeID(l.program, receiver.Generic) {
		receiver = l.program.Types[receiver.Generic-1]
	}
	fields := make([]air.Param, len(receiver.Fields))
	for i, field := range receiver.Fields {
		fields[i] = air.Param{Type: field.Type}
	}
	available := l.comparableTypeParams(air.Signature{Params: fields}, nil, nil, nil)
	for typeParam := range required {
		if !available[typeParam] {
			return false
		}
	}
	return true
}

func (l *lowerer) goMethodReceiverType(fn air.Function, typeArgs []air.TypeID) (ast.Expr, error) {
	if len(fn.Signature.Params) == 0 {
		return nil, fmt.Errorf("method %s has no receiver parameter", fn.Name)
	}
	receiverTypeID := fn.Receiver
	if receiverTypeID == air.NoType {
		receiverTypeID = fn.Signature.Params[0].Type
	}
	receiverType, err := l.goType(receiverTypeID)
	if len(typeArgs) > 0 && validTypeID(l.program, receiverTypeID) {
		receiver := l.program.Types[receiverTypeID-1]
		baseTypeID := receiverTypeID
		if receiver.Generic != air.NoType {
			baseTypeID = receiver.Generic
		}
		receiverType, err = l.goType(baseTypeID)
		if err == nil {
			receiverType = l.indexWithTypeArgs(receiverType, typeArgs)
		}
	}
	if err != nil {
		return nil, err
	}
	if l.isReferenceType(fn.Signature.Params[0].Type) {
		receiverType = &ast.StarExpr{X: receiverType}
	}
	return receiverType, nil
}

func (l *lowerer) functionCallExpr(fn air.Function, args []ast.Expr, typeArgs []air.TypeID) *ast.CallExpr {
	if methodName, ok := l.directGoMethodName(fn); ok && len(args) > 0 {
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: args[0], Sel: l.ident(methodName)},
			Args: args[1:],
		}
	}
	fun := l.functionExpr(fn)
	if len(typeArgs) > 0 {
		fun = l.indexWithTypeArgs(fun, typeArgs)
	}
	return &ast.CallExpr{Fun: fun, Args: args}
}

func (l *lowerer) functionReferenceExpr(fn air.Function, typeArgs []air.TypeID) (ast.Expr, error) {
	methodName, ok := l.directGoMethodName(fn)
	if !ok {
		value := l.functionExpr(fn)
		if len(typeArgs) > 0 {
			value = l.indexWithTypeArgs(value, typeArgs)
		}
		return value, nil
	}
	receiverType, err := l.goMethodReceiverType(fn, typeArgs)
	if err != nil {
		return nil, err
	}
	return &ast.SelectorExpr{X: receiverType, Sel: l.ident(methodName)}, nil
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
	methodName := fn.RequiredGoMethodName
	ok := methodName != "" && token.IsIdentifier(methodName)
	if methodName == "" {
		methodName, ok = goMethodName(fn.MethodName)
	}
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
		if l.usesABIResultReturn(returnType) {
			returnStmts, err := l.lowerABIReturn(fn, *block.Result, returnType)
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, returnStmts...)
		} else {
			result, err := l.lowerExprWithExpectedType(fn, *block.Result, returnType)
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, result.stmts...)
			if returnType == air.NoType || l.isVoidType(returnType) {
				if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
					stmts = l.appendVoidValueEval(stmts, result.expr)
				} else {
					stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
				}
			} else {
				stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{result.expr}})
			}
		}
	}
	return &ast.BlockStmt{List: stmts}, nil
}

func (l *lowerer) lowerABIReturn(fn air.Function, expr air.Expr, returnType air.TypeID) ([]ast.Stmt, error) {
	if !validTypeID(l.program, returnType) {
		return nil, fmt.Errorf("invalid ABI return type %d", returnType)
	}
	info := l.program.Types[returnType-1]
	if expr.Type == returnType && expr.Kind == air.ExprCall {
		call, err := l.lowerRawCall(fn, expr)
		if err == nil {
			stmts := append([]ast.Stmt{}, call.stmts...)
			stmts = append(stmts, &ast.ReturnStmt{Results: l.unpackABIResultExprs(returnType, call.expr)})
			return stmts, nil
		}
	}
	switch info.Kind {
	case air.TypeResult:
		switch expr.Kind {
		case air.ExprMakeResultOk:
			if expr.Target == nil {
				return nil, fmt.Errorf("result ok missing target")
			}
			if l.isVoidType(info.Value) {
				value, err := l.lowerExpr(fn, *expr.Target)
				if err != nil {
					return nil, err
				}
				return append(l.appendVoidValueEval(value.stmts, value.expr), &ast.ReturnStmt{Results: []ast.Expr{l.ident("nil")}}), nil
			}
			value, err := l.lowerExprWithExpectedType(fn, *expr.Target, info.Value)
			if err != nil {
				return nil, err
			}
			return append(value.stmts, &ast.ReturnStmt{Results: []ast.Expr{value.expr, l.ident("nil")}}), nil
		case air.ExprMakeResultErr:
			if expr.Target == nil {
				return nil, fmt.Errorf("result err missing target")
			}
			errValue, err := l.lowerExprWithExpectedType(fn, *expr.Target, info.Error)
			if err != nil {
				return nil, err
			}
			errExpr := l.goErrorValueExpr(info.Error, errValue.expr)
			if l.isVoidType(info.Value) {
				return append(errValue.stmts, &ast.ReturnStmt{Results: []ast.Expr{errExpr}}), nil
			}
			zero, err := l.zeroValueExpr(info.Value)
			if err != nil {
				return nil, err
			}
			return append(errValue.stmts, &ast.ReturnStmt{Results: []ast.Expr{zero, errExpr}}), nil
		}
	case air.TypeMaybe:
		switch expr.Kind {
		case air.ExprMakeMaybeSome:
			if expr.Target == nil {
				return nil, fmt.Errorf("maybe some missing target")
			}
			if l.isVoidType(info.Elem) {
				value, err := l.lowerExpr(fn, *expr.Target)
				if err != nil {
					return nil, err
				}
				return append(l.appendVoidValueEval(value.stmts, value.expr), &ast.ReturnStmt{Results: []ast.Expr{l.ident("true")}}), nil
			}
			value, err := l.lowerExprWithExpectedType(fn, *expr.Target, info.Elem)
			if err != nil {
				return nil, err
			}
			return append(value.stmts, &ast.ReturnStmt{Results: []ast.Expr{value.expr, l.ident("true")}}), nil
		case air.ExprMakeMaybeNone:
			if l.isVoidType(info.Elem) {
				return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{l.ident("false")}}}, nil
			}
			zero, err := l.maybeABIZeroValue(info)
			if err != nil {
				return nil, err
			}
			return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{zero, l.ident("false")}}}, nil
		}
	}
	value, err := l.lowerExprWithExpectedType(fn, expr, returnType)
	if err != nil {
		return nil, err
	}
	stmts := append([]ast.Stmt{}, value.stmts...)
	returnStmts, err := l.returnPackedABIValue(returnType, value.expr)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, returnStmts...)
	return stmts, nil
}

func (l *lowerer) unpackABIResultExprs(typeID air.TypeID, expr ast.Expr) []ast.Expr {
	if !validTypeID(l.program, typeID) {
		return []ast.Expr{expr}
	}
	info := l.program.Types[typeID-1]
	if info.Kind == air.TypeResult && l.resultUsesGoErrorABI(typeID) {
		if l.isVoidType(info.Value) {
			return []ast.Expr{expr}
		}
		return []ast.Expr{expr}
	}
	return []ast.Expr{expr}
}

func selectorExpr(target ast.Expr, field string) ast.Expr {
	if _, ok := target.(*ast.CompositeLit); ok {
		target = &ast.ParenExpr{X: target}
	}
	return &ast.SelectorExpr{X: target, Sel: ast.NewIdent(field)}
}

func (l *lowerer) goErrorValueExpr(errorType air.TypeID, expr ast.Expr) ast.Expr {
	if l.isBuiltinErrorType(errorType) {
		return expr
	}
	return &ast.CallExpr{Fun: l.qualified("errors", "errors", "New"), Args: []ast.Expr{expr}}
}

func (l *lowerer) returnPackedABIValue(typeID air.TypeID, expr ast.Expr) ([]ast.Stmt, error) {
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeResult:
		errSel := selectorExpr(expr, "Err")
		errExpr := l.goErrorValueExpr(info.Error, errSel)
		if l.isVoidType(info.Value) {
			return []ast.Stmt{
				&ast.IfStmt{Cond: &ast.UnaryExpr{Op: token.NOT, X: selectorExpr(expr, "Ok")}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{errExpr}}}}},
				&ast.ReturnStmt{Results: []ast.Expr{l.ident("nil")}},
			}, nil
		}
		zero, err := l.zeroValueExpr(info.Value)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{
			&ast.IfStmt{Cond: &ast.UnaryExpr{Op: token.NOT, X: selectorExpr(expr, "Ok")}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{zero, errExpr}}}}},
			&ast.ReturnStmt{Results: []ast.Expr{selectorExpr(expr, "Value"), l.ident("nil")}},
		}, nil
	case air.TypeMaybe:
		// runtime.Maybe exposes IsSome()/Value() methods, not Result-style
		// Ok/Value fields.
		if l.isVoidType(info.Elem) {
			return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{l.maybeIsSomeExpr(expr)}}}, nil
		}
		zero, err := l.maybeABIZeroValue(info)
		if err != nil {
			return nil, err
		}
		valueCall := &ast.CallExpr{Fun: &ast.SelectorExpr{X: expr, Sel: l.ident("Value")}}
		return []ast.Stmt{
			&ast.IfStmt{Cond: l.maybeIsNoneExpr(expr), Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{zero, l.ident("false")}}}}},
			&ast.ReturnStmt{Results: []ast.Expr{valueCall, l.ident("true")}},
		}, nil
	default:
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{expr}}}, nil
	}
}

func (l *lowerer) maybeABIZeroValue(info air.TypeInfo) (ast.Expr, error) {
	return l.zeroValueExpr(info.Elem)
}

func (l *lowerer) functionTypeInfo(typeID air.TypeID) (air.TypeInfo, bool) {
	if !validTypeID(l.program, typeID) {
		return air.TypeInfo{}, false
	}
	info := l.program.Types[typeID-1]
	if info.Kind != air.TypeFunction {
		return air.TypeInfo{}, false
	}
	return info, true
}

func (l *lowerer) packABICallResult(exprType, returnType air.TypeID, stmts []ast.Stmt, call *ast.CallExpr) (loweredExpr, error) {
	if !validTypeID(l.program, returnType) {
		return loweredExpr{stmts: stmts, expr: call}, nil
	}
	info := l.program.Types[returnType-1]
	switch info.Kind {
	case air.TypeResult:
		if l.isVoidType(info.Value) {
			return l.lowerGoErrorOnlyResultCall(air.Expr{Type: exprType}, stmts, call)
		}
		return l.lowerGoValueErrorResultCall(air.Expr{Type: exprType}, stmts, call, info)
	case air.TypeMaybe:
		if l.isVoidType(info.Elem) {
			maybeTemp := l.nextTemp()
			okTemp := l.nextTemp()
			decls, err := l.declareTemp(exprType, maybeTemp)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, decls...)
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(okTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}})
			someExpr, err := l.maybeSomeExpr(exprType, l.voidValueExpr())
			if err != nil {
				return loweredExpr{}, err
			}
			noneExpr, err := l.maybeNoneExpr(exprType)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, &ast.IfStmt{Cond: l.ident(okTemp), Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(maybeTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}}}}, Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(maybeTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}}}}})
			return loweredExpr{stmts: stmts, expr: l.ident(maybeTemp)}, nil
		}
		return l.lowerGoValueBoolMaybeCall(air.Expr{Type: exprType}, stmts, call)
	default:
		return loweredExpr{stmts: stmts, expr: call}, nil
	}
}

func concreteCallParams(expr air.Expr, target air.Function) []air.Param {
	params := target.Signature.Params
	if len(expr.TypeArgs) == 0 {
		return params
	}
	params = append([]air.Param(nil), params...)
	for i := range params {
		if i < len(expr.Args) && expr.Args[i].Type != air.NoType {
			params[i].Type = expr.Args[i].Type
		}
	}
	return params
}

func (l *lowerer) lowerRawCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Kind != air.ExprCall || !validFunctionID(l.program, expr.Function) {
		return loweredExpr{}, fmt.Errorf("not a valid call")
	}
	target := l.program.Functions[expr.Function]
	args, stmts, writeback, err := l.lowerCallArgs(fn, expr.Args, concreteCallParams(expr, target))
	if err != nil {
		return loweredExpr{}, err
	}
	call := l.functionCallExpr(target, args, expr.TypeArgs)
	if len(writeback) > 0 {
		return loweredExpr{}, fmt.Errorf("raw ABI call with writeback args is not supported")
	}
	return loweredExpr{stmts: stmts, expr: call}, nil
}

func (l *lowerer) lowerBlockValueReturn(fn air.Function, block air.Block, returnType air.TypeID) (*ast.BlockStmt, error) {
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
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
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
		localType := fn.Locals[stmt.Local].Type
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, localType)
		if err != nil {
			return nil, err
		}
		if stmt.Value.Kind == air.ExprForeignCall && stmt.Value.ForeignPointer && !l.localIsReference(fn, stmt.Local) {
			// A value-typed binding of a pointer-returning Go call snapshots the
			// referenced storage instead of aliasing it.
			value.expr = &ast.StarExpr{X: value.expr}
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
				Lhs: []ast.Expr{l.ident(name)},
				Tok: tok,
				Rhs: []ast.Expr{l.voidValueExpr()},
			})
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(name)}})
			return out, nil
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(name)},
			Tok: tok,
			Rhs: []ast.Expr{value.expr},
		})
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(name)}})
		return out, nil
	case air.StmtAssign:
		if stmt.Value == nil {
			return nil, fmt.Errorf("assign statement missing value")
		}
		localType := fn.Locals[stmt.Local].Type
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
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(name)}, Tok: tok, Rhs: []ast.Expr{l.voidValueExpr()}})
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(name)}})
			return out, nil
		}
		if l.localIsPointerParam(fn, stmt.Local) && l.isTraitObjectType(localType) {
			return nil, fmt.Errorf("whole-referent assignment through mutable trait reference is unsupported")
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{l.localAssignExpr(fn, stmt.Local)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value.expr},
		})
		return out, nil
	case air.StmtAssignGlobal:
		if stmt.Value == nil {
			return nil, fmt.Errorf("global assignment missing value")
		}
		if stmt.Global < 0 || int(stmt.Global) >= len(l.program.Globals) {
			return nil, fmt.Errorf("assignment to unknown global %d", stmt.Global)
		}
		global := l.program.Globals[stmt.Global]
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, global.Type)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, value.stmts...)
		valueExpr := value.expr
		if l.isVoidType(global.Type) || isVoidExpr(valueExpr) {
			out = l.appendVoidValueEval(out, valueExpr)
			valueExpr = l.voidValueExpr()
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{l.globalExpr(global)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{valueExpr},
		})
		return out, nil
	case air.StmtSetForeignValue:
		if stmt.Value == nil {
			return nil, fmt.Errorf("foreign value set statement missing value")
		}
		if stmt.ForeignTarget != "go" {
			return nil, fmt.Errorf("unsupported foreign value set target %q", stmt.ForeignTarget)
		}
		if stmt.ForeignNamespace == "" || stmt.ForeignSymbol == "" {
			return nil, fmt.Errorf("invalid go foreign value set %q::%q", stmt.ForeignNamespace, stmt.ForeignSymbol)
		}
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, stmt.Type)
		if err != nil {
			return nil, err
		}
		qualifier := stmt.ForeignQualifier
		if qualifier == "" {
			qualifier = stmt.ForeignNamespace
			if slash := strings.LastIndex(qualifier, "/"); slash >= 0 {
				qualifier = qualifier[slash+1:]
			}
		}
		out := append([]ast.Stmt{}, value.stmts...)
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{l.qualified(qualifier, stmt.ForeignNamespace, stmt.ForeignSymbol)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{value.expr},
		})
		return out, nil
	case air.StmtSetForeignField:
		if stmt.Target == nil {
			return nil, fmt.Errorf("foreign field set statement missing target")
		}
		if stmt.Value == nil {
			return nil, fmt.Errorf("foreign field set statement missing value")
		}
		if stmt.ForeignTarget != "go" {
			return nil, fmt.Errorf("unsupported foreign field set target %q", stmt.ForeignTarget)
		}
		target, err := l.lowerExpr(fn, *stmt.Target)
		if err != nil {
			return nil, err
		}
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, stmt.Type)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, target.stmts...)
		out = append(out, value.stmts...)
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: target.expr, Sel: l.ident(stmt.ForeignSymbol)}},
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
		if targetType.Kind == air.TypeReference {
			if !validTypeID(l.program, targetType.Elem) {
				return nil, fmt.Errorf("field set target has invalid referent %d", targetType.Elem)
			}
			targetType = l.program.Types[targetType.Elem-1]
		}
		if targetType.Kind != air.TypeStruct {
			return nil, fmt.Errorf("field set target must be struct, got kind %d", targetType.Kind)
		}
		if stmt.Field < 0 || stmt.Field >= len(targetType.Fields) {
			return nil, fmt.Errorf("invalid field set index %d", stmt.Field)
		}
		field := targetType.Fields[stmt.Field]
		value, err := l.lowerExprWithExpectedType(fn, *stmt.Value, field.Type)
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, target.stmts...)
		out = append(out, value.stmts...)
		fieldTarget := ast.Expr(&ast.SelectorExpr{X: target.expr, Sel: l.ident(l.goFieldName(targetType, field.Name))})
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
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr.expr}})
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
		return []ast.Stmt{l.labelLoopBreaks(&ast.ForStmt{Cond: condition.expr, Body: body}, body)}, nil
	case air.StmtForMap:
		if stmt.Target == nil {
			return nil, fmt.Errorf("map for statement missing target")
		}
		target, err := l.lowerExpr(fn, *stmt.Target)
		if err != nil {
			return nil, err
		}
		if len(target.stmts) != 0 {
			return nil, fmt.Errorf("map for targets with setup statements are not supported yet")
		}
		keyName := l.localName(fn, stmt.Local)
		valueName := l.localName(fn, stmt.ValueLocal)
		l.declaredLocals[stmt.Local] = true
		l.declaredLocals[stmt.ValueLocal] = true
		body, err := l.lowerBlock(fn, stmt.Body, air.NoType)
		if err != nil {
			return nil, err
		}
		body.List = append([]ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(keyName)}},
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(valueName)}},
		}, body.List...)
		rangeStmt := &ast.RangeStmt{Key: l.ident(keyName), Value: l.ident(valueName), Tok: token.DEFINE, X: target.expr, Body: body}
		return []ast.Stmt{l.labelLoopBreaks(rangeStmt, body)}, nil
	case air.StmtBreak:
		return []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}, nil
	case air.StmtDefer:
		bodyBlock := stmt.Body
		if stmt.Expr != nil {
			bodyBlock = air.Block{Stmts: []air.Stmt{{Kind: air.StmtExpr, Expr: stmt.Expr}}}
		}
		body, err := l.lowerBlock(fn, bodyBlock, air.NoType)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: body}}}}, nil
	default:
		return nil, fmt.Errorf("unsupported statement kind %d", stmt.Kind)
	}
}

// labelLoopBreaks rewrites Ard `break` statements that lower lexically inside
// an emitted Go switch, type switch, or select so they exit the enclosing
// loop instead of that construct. In Go a bare `break` binds to the nearest
// switch/select/for, but Ard's `break` always means the nearest loop, so the
// loop gains a label and the intercepted breaks become `break <label>`.
// Nested loops and function literals are not descended into: breaks there
// bind to their own construct.
func (l *lowerer) labelLoopBreaks(loop ast.Stmt, body *ast.BlockStmt) ast.Stmt {
	label := fmt.Sprintf("_loop_%d", l.tempCounter)
	labeled := false
	var rewrite func(node ast.Node, intercepted bool)
	rewrite = func(node ast.Node, intercepted bool) {
		switch node := node.(type) {
		case nil:
			return
		case *ast.BranchStmt:
			if node.Tok == token.BREAK && node.Label == nil && intercepted {
				node.Label = l.ident(label)
				labeled = true
			}
		case *ast.ForStmt, *ast.RangeStmt, *ast.FuncLit:
			// A nested loop owns its own breaks; a function literal is a
			// different frame entirely.
			return
		case *ast.SwitchStmt:
			rewrite(node.Body, true)
		case *ast.TypeSwitchStmt:
			rewrite(node.Body, true)
		case *ast.SelectStmt:
			rewrite(node.Body, true)
		case *ast.BlockStmt:
			for _, stmt := range node.List {
				rewrite(stmt, intercepted)
			}
		case *ast.CaseClause:
			for _, stmt := range node.Body {
				rewrite(stmt, intercepted)
			}
		case *ast.CommClause:
			for _, stmt := range node.Body {
				rewrite(stmt, intercepted)
			}
		case *ast.IfStmt:
			rewrite(node.Body, intercepted)
			rewrite(node.Else, intercepted)
		case *ast.LabeledStmt:
			rewrite(node.Stmt, intercepted)
		}
	}
	rewrite(body, false)
	if !labeled {
		return loop
	}
	l.tempCounter++
	return &ast.LabeledStmt{Label: l.ident(label), Stmt: loop}
}

func (l *lowerer) lowerExpr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	switch expr.Kind {
	case air.ExprConstVoid:
		return loweredExpr{expr: l.voidValueExpr()}, nil
	case air.ExprConstInt:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.INT, Value: expr.Int}}, nil
	case air.ExprConstFloat:
		return loweredExpr{expr: &ast.BasicLit{Kind: token.FLOAT, Value: expr.Float}}, nil
	case air.ExprConstBool:
		if expr.Bool {
			return loweredExpr{expr: l.ident("true")}, nil
		}
		return loweredExpr{expr: l.ident("false")}, nil
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
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("panic"), Args: []ast.Expr{target.expr}}})
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
		value, err := l.functionReferenceExpr(l.program.Functions[expr.Function], expr.TypeArgs)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{expr: value}, nil
	case air.ExprUnionWrap:
		return l.lowerUnionWrap(fn, expr)
	case air.ExprMatchUnion:
		return l.lowerMatchUnion(fn, expr)
	case air.ExprMatchForeignType:
		return l.lowerMatchForeignType(fn, expr)
	case air.ExprScalarConvert:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("scalar convert missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		convertType, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: convertType, Args: []ast.Expr{target.expr}}}, nil
	case air.ExprTraitRefProject:
		return l.lowerTraitReferenceProjection(fn, expr)
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
				stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(temp)}, Type: tempType}}}})
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
				return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: traitType, Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: l.ident(temp)}}}}, nil
			}
			target, err := l.lowerExpr(fn, *expr.Target)
			if err != nil {
				return loweredExpr{}, err
			}
			return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: traitType, Args: []ast.Expr{target.expr}}}, nil
		}
		if l.implRequiresPointerReceiver(expr.Impl) {
			place, setup, ok, err := l.mutableTraitUpcastPlace(fn, *expr.Target)
			if err != nil {
				return loweredExpr{}, err
			}
			if ok {
				return loweredExpr{stmts: setup, expr: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{addressOfPlace(place)}}}, nil
			}
			target, err := l.lowerExpr(fn, *expr.Target)
			if err != nil {
				return loweredExpr{}, err
			}
			temp := l.nextTemp()
			stmts := append([]ast.Stmt{}, target.stmts...)
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{target.expr}})
			return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: l.ident(temp)}}}}, nil
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{target.expr}}}, nil
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
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("int"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprToF64:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to_f64 missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("float64"), Args: []ast.Expr{target.expr}}}, nil
	case air.ExprMutRef:
		return l.lowerMutRef(fn, expr)
	case air.ExprDeref:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("deref missing operand")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		if l.isReferenceType(expr.Target.Type) {
			reference := l.program.Types[expr.Target.Type-1]
			if l.isTraitObjectType(reference.Elem) {
				trait := l.program.Traits[l.program.Types[reference.Elem-1].Trait]
				return loweredExpr{stmts: target.stmts, expr: l.mutableTraitLoadExpr(trait, target.expr)}, nil
			}
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.StarExpr{X: target.expr}}, nil
	case air.ExprMakeClosure:
		return l.lowerMakeClosure(fn, expr)
	case air.ExprCallClosure:
		return l.lowerCallClosure(fn, expr)
	case air.ExprMakeError:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("error constructor missing message")
		}
		message, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: message.stmts, expr: &ast.CallExpr{Fun: l.qualified("errors", "errors", "New"), Args: []ast.Expr{message.expr}}}, nil
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
			&ast.KeyValueExpr{Key: l.ident("Value"), Value: valueExpr},
			&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")},
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
			&ast.KeyValueExpr{Key: l.ident("Err"), Value: errExpr},
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
	case air.ExprMaybeSet:
		return l.lowerMaybeSet(fn, expr)
	case air.ExprMaybeClear:
		return l.lowerMaybeClear(fn, expr)
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
	case air.ExprMakeFixedArray:
		return l.lowerMakeList(fn, expr)
	case air.ExprAsyncStart:
		return l.lowerAsyncStart(fn, expr)
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
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{target.expr}}}, nil
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
		return loweredExpr{stmts: target.stmts, expr: &ast.BinaryExpr{X: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{target.expr}}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}}, nil
	case air.ExprStrBytes:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str bytes missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.ArrayType{Elt: l.ident("byte")}, Args: []ast.Expr{target.expr}}}, nil
	case air.ExprStrRunes:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str runes missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.ArrayType{Elt: l.ident("rune")}, Args: []ast.Expr{target.expr}}}, nil
	case air.ExprStrSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("str size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{target.expr}}}, nil
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
				&ast.AssignStmt{Lhs: []ast.Expr{l.ident(runesTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.ArrayType{Elt: l.ident("rune")}, Args: []ast.Expr{target.expr}}}},
				&ast.AssignStmt{Lhs: []ast.Expr{l.ident(indexTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{index.expr}},
			)
			cond := &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: l.ident(indexTemp), Op: token.LSS, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
				Op: token.LOR,
				Y:  &ast.BinaryExpr{X: l.ident(indexTemp), Op: token.GEQ, Y: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(runesTemp)}}},
			}
			elemTypeID := l.program.Types[expr.Type-1].Elem
			elemType := mustTypeExpr(l, elemTypeID)
			noneCall := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("None"), Index: elemType}}
			someValue := ast.Expr(&ast.IndexExpr{X: l.ident(runesTemp), Index: l.ident(indexTemp)})
			if validTypeID(l.program, elemTypeID) && l.program.Types[elemTypeID-1].Kind == air.TypeStr {
				someValue = &ast.CallExpr{Fun: l.ident("string"), Args: []ast.Expr{someValue}}
			}
			someCall := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: elemType}, Args: []ast.Expr{someValue}}
			stmts = append(stmts, &ast.IfStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneCall}}}},
				Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someCall}}}},
			})
			return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
		}
		byteExpr := &ast.IndexExpr{X: target.expr, Index: index.expr}
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.ident("string"), Args: []ast.Expr{byteExpr}}}, nil
	case air.ExprStrSlice:
		return l.lowerCheckedSlice(fn, expr, false)
	case air.ExprListSize:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("list size missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)}}}, nil
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
		return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: l.valueThroughReference(expr.Target.Type, target.expr), Index: index.expr}}, nil
	case air.ExprListAtChecked:
		// User-facing list.at: a bounds-checked access producing Maybe(elem).
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("list at missing target")
		}
		if len(expr.Args) != 1 {
			return loweredExpr{}, fmt.Errorf("list at expects one arg")
		}
		if !validTypeID(l.program, expr.Type) || l.program.Types[expr.Type-1].Kind != air.TypeMaybe {
			return loweredExpr{}, fmt.Errorf("checked list at lowered with non-Maybe type %d", expr.Type)
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
		resultTemp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, resultTemp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		sliceTemp := l.nextTemp()
		indexTemp := l.nextTemp()
		stmts = append(stmts,
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident(sliceTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)}},
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident(indexTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{index.expr}},
		)
		cond := &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: l.ident(indexTemp), Op: token.LSS, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
			Op: token.LOR,
			Y:  &ast.BinaryExpr{X: l.ident(indexTemp), Op: token.GEQ, Y: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(sliceTemp)}}},
		}
		elemTypeID := l.program.Types[expr.Type-1].Elem
		elemType, err := l.goType(elemTypeID)
		if err != nil {
			return loweredExpr{}, err
		}
		noneCall := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("None"), Index: elemType}}
		someCall := &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: elemType}, Args: []ast.Expr{&ast.IndexExpr{X: l.ident(sliceTemp), Index: l.ident(indexTemp)}}}
		stmts = append(stmts, &ast.IfStmt{
			Cond: cond,
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneCall}}}},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someCall}}}},
		})
		return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
	case air.ExprListSlice:
		return l.lowerCheckedSlice(fn, expr, true)
	case air.ExprListIsEmpty:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("slice is_empty missing target")
		}
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: target.stmts, expr: &ast.BinaryExpr{X: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)}}, Op: token.EQL, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}}, nil
	case air.ExprListToList:
		return l.lowerSliceToList(fn, expr)
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
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)}}}, nil
	case air.ExprMapHas:
		return l.lowerMapHas(fn, expr)
	case air.ExprMapGet:
		return l.lowerMapGet(fn, expr)
	case air.ExprMapSet:
		return l.lowerMapSet(fn, expr)
	case air.ExprMapDelete:
		return l.lowerMapDelete(fn, expr)
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
			if hasFieldInfo {
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
			elts = append(elts, &ast.KeyValueExpr{Key: l.ident(l.goFieldName(typ, field.Name)), Value: fieldValue})
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
		if targetType.Kind == air.TypeReference {
			if !validTypeID(l.program, targetType.Elem) {
				return loweredExpr{}, fmt.Errorf("invalid reference elem type id %d", targetType.Elem)
			}
			targetType = l.program.Types[targetType.Elem-1]
		}
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
			targetExpr := l.ident(targetTemp)
			resultExpr := l.ident(resultTemp)
			stmts := append([]ast.Stmt{}, target.stmts...)
			stmts = append(stmts, targetDecls...)
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
			stmts = append(stmts, resultDecls...)
			fieldExpr := &ast.SelectorExpr{X: l.maybeValueExpr(targetExpr), Sel: l.ident(l.goFieldName(elemType, field.Name))}
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
		fieldExpr := &ast.SelectorExpr{X: target.expr, Sel: l.ident(l.goFieldName(targetType, field.Name))}
		return loweredExpr{stmts: target.stmts, expr: fieldExpr}, nil
	case air.ExprBlock:
		return l.lowerBlockExpr(fn, expr)
	case air.ExprUnsafeBlock:
		return l.lowerUnsafeBlockExpr(fn, expr)
	case air.ExprIf:
		return l.lowerIfExpr(fn, expr)
	case air.ExprForeignCall:
		return l.lowerForeignCall(fn, expr)
	case air.ExprForeignMethodCall:
		return l.lowerForeignMethodCall(fn, expr)
	case air.ExprForeignMethodValue:
		return l.lowerForeignMethodValue(fn, expr)
	case air.ExprForeignFieldAccess:
		return l.lowerForeignFieldAccess(fn, expr)
	case air.ExprForeignStructInstance:
		return l.lowerForeignStructInstance(fn, expr)
	case air.ExprForeignValue:
		return l.lowerForeignValue(fn, expr)
	case air.ExprInterfaceConversion:
		return l.lowerInterfaceConversion(fn, expr)
	case air.ExprDiscardingFunctionCoercion:
		return l.lowerDiscardingFunctionCoercion(fn, expr)
	case air.ExprUnsafeCast:
		return l.lowerUnsafeCast(fn, expr)
	case air.ExprUnsafeIsNil:
		return l.lowerUnsafeIsNil(fn, expr)
	case air.ExprCall:
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid function id %d", expr.Function)
		}
		target := l.program.Functions[expr.Function]
		args, stmts, writeback, err := l.lowerCallArgs(fn, expr.Args, concreteCallParams(expr, target))
		if err != nil {
			return loweredExpr{}, err
		}
		call := l.functionCallExpr(target, args, expr.TypeArgs)
		if l.abiReturnShapeAvailable(target.Signature.Return) && len(writeback) == 0 {
			return l.packABICallResult(expr.Type, target.Signature.Return, stmts, call)
		}
		return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
	case air.ExprEq, air.ExprNotEq:
		leftTypeID := expr.Left.Type
		rightTypeID := expr.Right.Type
		left, err := l.lowerExpr(fn, *expr.Left)
		if err != nil {
			return loweredExpr{}, err
		}
		right, err := l.lowerExpr(fn, *expr.Right)
		if err != nil {
			return loweredExpr{}, err
		}
		_, leftIsTraitReference := l.traitReference(leftTypeID)
		_, rightIsTraitReference := l.traitReference(rightTypeID)
		if leftIsTraitReference != rightIsTraitReference {
			left.expr = &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{left.expr}}
			right.expr = &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{right.expr}}
		}
		l.castEnumIntComparisonOperands(&left, leftTypeID, &right, rightTypeID)
		var equality ast.Expr = &ast.BinaryExpr{X: left.expr, Op: l.binaryToken(expr.Kind), Y: right.expr}
		if l.isMaybeType(leftTypeID) || l.isMaybeType(rightTypeID) {
			equality = &ast.CallExpr{Fun: l.runtimeQualified("MaybeEqual"), Args: []ast.Expr{left.expr, right.expr}}
			if expr.Kind == air.ExprNotEq {
				equality = &ast.UnaryExpr{Op: token.NOT, X: equality}
			}
		}
		return loweredExpr{stmts: append(left.stmts, right.stmts...), expr: equality}, nil
	case air.ExprAnd, air.ExprOr:
		left, err := l.lowerExpr(fn, *expr.Left)
		if err != nil {
			return loweredExpr{}, err
		}
		right, err := l.lowerExpr(fn, *expr.Right)
		if err != nil {
			return loweredExpr{}, err
		}
		if len(right.stmts) == 0 {
			return loweredExpr{
				stmts: left.stmts,
				expr:  &ast.BinaryExpr{X: left.expr, Op: l.binaryToken(expr.Kind), Y: right.expr},
			}, nil
		}

		resultName := l.nextTemp()
		result := l.ident(resultName)
		stmts := append([]ast.Stmt{}, left.stmts...)
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{result}, Tok: token.DEFINE, Rhs: []ast.Expr{left.expr}})
		body := append([]ast.Stmt{}, right.stmts...)
		body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{result}, Tok: token.ASSIGN, Rhs: []ast.Expr{right.expr}})
		condition := ast.Expr(result)
		if expr.Kind == air.ExprOr {
			condition = &ast.UnaryExpr{Op: token.NOT, X: result}
		}
		stmts = append(stmts, &ast.IfStmt{Cond: condition, Body: &ast.BlockStmt{List: body}})
		return loweredExpr{stmts: stmts, expr: result}, nil
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte, air.ExprStrConcat:
		leftTypeID := expr.Left.Type
		rightTypeID := expr.Right.Type
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
		return loweredExpr{stmts: body, expr: l.ident("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	body, err := l.lowerValueBlock(fn, expr.Body, expr.Type, l.ident(temp))
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: append(decls, body...), expr: l.ident(temp)}, nil
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
		Lhs: []ast.Expr{l.ident(recoveredName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("recover")}},
	}
	recoverCond := &ast.BinaryExpr{X: l.ident(recoveredName), Op: token.NEQ, Y: l.ident("nil")}
	recoverResult := &ast.AssignStmt{
		Lhs: []ast.Expr{l.ident(resultName)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: l.ident("Err"), Value: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{l.ident(recoveredName)}}},
		}}},
	}
	deferRecover := &ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.IfStmt{Init: recoverAssign, Cond: recoverCond, Body: &ast.BlockStmt{List: []ast.Stmt{recoverResult}}}}},
	}}}

	helperFn := fn
	helperFn.Signature.Return = expr.Type
	body := []ast.Stmt{deferRecover}
	prevForceValueResultReturns := l.forceValueResultReturns
	l.forceValueResultReturns = true
	defer func() { l.forceValueResultReturns = prevForceValueResultReturns }()
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
		loweredBody, err := l.lowerValueBlock(helperFn, expr.Body, resultInfo.Value, l.ident(valueName))
		if err != nil {
			return loweredExpr{}, err
		}
		body = append(body, loweredBody...)
		valueExpr = l.ident(valueName)
	}
	body = append(body, &ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Value"), Value: valueExpr},
		&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")},
	}}}})

	return loweredExpr{expr: &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{l.ident(resultName)}, Type: resultType}}}},
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
	resultExpr := l.ident("nil")
	stmts := append([]ast.Stmt{}, condition.stmts...)
	var target ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		target = l.ident(temp)
		resultExpr = l.ident(temp)
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
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{result.expr}})
			}
		} else {
			if l.isVoidType(block.Result.Type) || isVoidExpr(result.expr) {
				return l.appendVoidValueEval(stmts, result.expr), nil
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
	if !l.isTraitObjectType(expectedType) {
		return loweredExpr{}, fmt.Errorf("invalid mutable trait value type %d", expectedType)
	}
	traitID := l.program.Types[expectedType-1].Trait
	if !validTraitID(l.program, traitID) {
		return loweredExpr{}, fmt.Errorf("invalid mutable trait value trait %d", traitID)
	}
	return loweredExpr{stmts: value.stmts, expr: l.mutableTraitLoadExpr(l.program.Traits[traitID], value.expr)}, nil
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
	return l.declareReferenceAwareTemp(typeID, name, false)
}

func (l *lowerer) declareReferenceAwareTemp(typeID air.TypeID, name string, reference bool) ([]ast.Stmt, error) {
	typ, err := l.goType(typeID)
	if reference && l.mutableParamUsesPointer(typeID) {
		typ, err = l.mutableParamType(typeID)
	}
	if err != nil {
		return nil, err
	}
	return []ast.Stmt{&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(name)}, Type: typ}}}}}, nil
}

func (l *lowerer) nextTemp() string {
	name := fmt.Sprintf("_tmp_%d", l.tempCounter)
	l.tempCounter++
	return name
}

func (l *lowerer) lowerForeignStructInstance(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.ForeignTarget != "go" {
		return loweredExpr{}, fmt.Errorf("unsupported foreign struct target %q", expr.ForeignTarget)
	}
	if expr.ForeignNamespace == "" || expr.ForeignSymbol == "" {
		return loweredExpr{}, fmt.Errorf("invalid foreign struct literal")
	}
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	var stmts []ast.Stmt
	elts := make([]ast.Expr, 0, len(expr.Fields))
	fields := make([]*air.StructFieldValue, len(expr.Fields))
	for i := range expr.Fields {
		fields[i] = &expr.Fields[i]
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, field := range fields {
		value, err := l.lowerExpr(fn, field.Value)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, value.stmts...)
		elts = append(elts, &ast.KeyValueExpr{Key: l.ident(field.Name), Value: value.expr})
	}
	return loweredExpr{stmts: stmts, expr: &ast.CompositeLit{Type: typ, Elts: elts}}, nil
}

func (l *lowerer) lowerForeignFieldAccess(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.ForeignTarget != "go" {
		return loweredExpr{}, fmt.Errorf("unsupported foreign field target %q", expr.ForeignTarget)
	}
	if expr.Target == nil || expr.ForeignSymbol == "" {
		return loweredExpr{}, fmt.Errorf("invalid foreign field access")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: l.ident(expr.ForeignSymbol)}}, nil
}

func (l *lowerer) lowerUnsafeCast(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("unsafe::cast missing target")
	}
	if len(expr.TypeArgs) != 1 {
		return loweredExpr{}, fmt.Errorf("unsafe::cast expects one target type, got %d", len(expr.TypeArgs))
	}
	maybeType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	targetType, err := l.goType(expr.TypeArgs[0])
	if err != nil {
		return loweredExpr{}, err
	}
	value, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultElemType := targetType
	if expr.ForeignPointer {
		resultElemType = &ast.StarExpr{X: targetType}
	}

	valueName := l.nextTemp()
	cases := []*ast.CaseClause{}
	if !expr.ForeignPointer {
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{targetType},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: resultElemType}, Args: []ast.Expr{l.ident(valueName)}}}}},
		})
	}
	pointerType := &ast.StarExpr{X: targetType}
	pointerBody := []ast.Stmt{
		&ast.IfStmt{Cond: &ast.BinaryExpr{X: l.ident(valueName), Op: token.NEQ, Y: l.ident("nil")}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: resultElemType}, Args: []ast.Expr{anyCastSomeArg(l.ident(valueName), expr.ForeignPointer)}}}}}}},
	}
	cases = append(cases, &ast.CaseClause{List: []ast.Expr{pointerType}, Body: pointerBody})
	body := []ast.Stmt{
		&ast.TypeSwitchStmt{
			Assign: &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: value.expr, Type: nil}}},
			Body:   &ast.BlockStmt{List: typeSwitchClausesToStmts(cases)},
		},
		&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("None"), Index: resultElemType}}}},
	}
	return loweredExpr{stmts: value.stmts, expr: &ast.CallExpr{Fun: &ast.FuncLit{Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: maybeType}}}}, Body: &ast.BlockStmt{List: body}}}}, nil
}

func (l *lowerer) lowerUnsafeIsNil(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("unsafe::is_nil missing target")
	}
	value, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: value.stmts, expr: &ast.CallExpr{Fun: l.runtimeQualified("IsNil"), Args: []ast.Expr{value.expr}}}, nil
}

func anyCastSomeArg(value ast.Expr, mutable bool) ast.Expr {
	if mutable {
		return value
	}
	return &ast.StarExpr{X: value}
}

func typeSwitchClausesToStmts(cases []*ast.CaseClause) []ast.Stmt {
	stmts := make([]ast.Stmt, len(cases))
	for i, clause := range cases {
		stmts[i] = clause
	}
	return stmts
}

func (l *lowerer) lowerForeignValue(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.ForeignTarget != "go" {
		return loweredExpr{}, fmt.Errorf("unsupported foreign value target %q", expr.ForeignTarget)
	}
	if expr.ForeignNamespace == "" || expr.ForeignSymbol == "" {
		return loweredExpr{}, fmt.Errorf("invalid go foreign value %q::%q", expr.ForeignNamespace, expr.ForeignSymbol)
	}
	qualifier := expr.ForeignQualifier
	if qualifier == "" {
		qualifier = expr.ForeignNamespace
		if slash := strings.LastIndex(qualifier, "/"); slash >= 0 {
			qualifier = qualifier[slash+1:]
		}
	}
	value := loweredExpr{expr: l.qualified(qualifier, expr.ForeignNamespace, expr.ForeignSymbol)}
	if validTypeID(l.program, expr.Type) && l.program.Types[expr.Type-1].Kind == air.TypeFunction {
		return l.lowerForeignCallableValue(fn, expr, value)
	}
	return value, nil
}

func (l *lowerer) lowerInterfaceConversion(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("interface conversion missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	switch expr.InterfaceMode {
	case air.InterfaceValue:
	case air.InterfaceReference:
		if l.isReferenceType(expr.Target.Type) {
			reference := l.program.Types[expr.Target.Type-1]
			if l.isTraitObjectType(reference.Elem) {
				trait := l.program.Traits[l.program.Types[reference.Elem-1].Trait]
				target.expr = l.mutableTraitProjectExpr(trait, target.expr)
			}
		}
	case air.InterfaceOwnedPointer:
		// Materialize fresh storage even when the source is addressable. Taking
		// the caller's address would turn value conversion into borrowed aliasing.
		tmp := l.nextTemp()
		target.stmts = append(target.stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(tmp)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{target.expr},
		})
		target.expr = &ast.UnaryExpr{Op: token.AND, X: l.ident(tmp)}
	default:
		return loweredExpr{}, fmt.Errorf("unsupported interface conversion mode %d", expr.InterfaceMode)
	}
	if validTypeID(l.program, expr.Type) && l.program.Types[expr.Type-1].Kind == air.TypeAny {
		target.expr = &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{target.expr}}
	}
	return target, nil
}

func (l *lowerer) foreignABIValueArg(arg air.Expr, value ast.Expr, mode air.ABIParamMode) ast.Expr {
	if mode == air.ABIParamDescriptorValue {
		return l.valueThroughReference(arg.Type, value)
	}
	return value
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
		mode := expr.ForeignArgABI[i]
		args = append(args, l.foreignABIValueArg(expr.Args[i], arg.expr, mode))
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
	fun := l.qualified(pkgName, importPath, functionName)
	if len(expr.TypeArgs) > 0 {
		fun = l.indexWithTypeArgs(fun, expr.TypeArgs)
	}
	call := &ast.CallExpr{Fun: fun, Args: args}
	if validTypeID(l.program, expr.Type) {
		info := l.program.Types[expr.Type-1]
		switch info.Kind {
		case air.TypeResult:
			if expr.ForeignResultShape == air.ForeignResultUnknown {
				return loweredExpr{}, fmt.Errorf("Go foreign call %s.%s is missing its result shape", importPath, functionName)
			}
			switch expr.ForeignResultShape {
			case air.ForeignResultValueError:
				return l.lowerGoValueErrorResultCall(expr, stmts, call, info)
			case air.ForeignResultErrorOnly:
				return l.lowerGoErrorOnlyResultCall(expr, stmts, call)
			}
		case air.TypeMaybe:
			if expr.ForeignResultShape == air.ForeignResultUnknown {
				return loweredExpr{}, fmt.Errorf("Go foreign call %s.%s is missing its result shape", importPath, functionName)
			}
			if expr.ForeignResultShape == air.ForeignResultValueBool {
				return l.lowerGoValueBoolMaybeCall(expr, stmts, call)
			}
		}
	}
	return loweredExpr{stmts: stmts, expr: call}, nil
}

func (l *lowerer) lowerGoValueBoolMaybeCall(expr air.Expr, stmts []ast.Stmt, call *ast.CallExpr) (loweredExpr, error) {
	maybeTemp := l.nextTemp()
	valueTemp := l.nextTemp()
	okTemp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, maybeTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	info := l.program.Types[expr.Type-1]
	valueType, err := l.goType(info.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(valueTemp)}, Type: valueType}}}})
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(okTemp)}, Type: l.ident("bool")}}}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueTemp), l.ident(okTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	someExpr, err := l.maybeSomeExpr(expr.Type, l.ident(valueTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	noneExpr, err := l.maybeNoneExpr(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.ident(okTemp),
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(maybeTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(maybeTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr}}}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(maybeTemp)}, nil
}

func (l *lowerer) lowerGoErrorOnlyResultCall(expr air.Expr, stmts []ast.Stmt, call *ast.CallExpr) (loweredExpr, error) {
	resultTemp := l.nextTemp()
	errTemp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(errTemp)}, Type: l.ident("error")}}}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	okLit := &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")},
	}}
	errResult := ast.Expr(&ast.CallExpr{Fun: &ast.SelectorExpr{X: l.ident(errTemp), Sel: l.ident("Error")}})
	if resultInfo, ok := l.typeInfo(expr.Type); ok && resultInfo.Kind == air.TypeResult && l.isBuiltinErrorType(resultInfo.Error) {
		errResult = l.ident(errTemp)
	}
	errLit := &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Err"), Value: errResult},
	}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: l.ident(errTemp), Op: token.NEQ, Y: l.ident("nil")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{errLit}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{okLit}}}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
}

func (l *lowerer) lowerForeignCallableValue(_ air.Function, expr air.Expr, callee loweredExpr) (loweredExpr, error) {
	if !validTypeID(l.program, expr.Type) {
		return callee, nil
	}
	fnInfo := l.program.Types[expr.Type-1]
	if fnInfo.Kind != air.TypeFunction {
		return callee, nil
	}
	if len(expr.ForeignArgABI) != len(fnInfo.Params) {
		return loweredExpr{}, fmt.Errorf("foreign callable %s has %d ABI modes for %d parameters", expr.ForeignSymbol, len(expr.ForeignArgABI), len(fnInfo.Params))
	}
	needsArgAdaptation := false
	for _, mode := range expr.ForeignArgABI {
		if mode != air.ABIParamExact {
			needsArgAdaptation = true
			break
		}
	}
	returnInfo := air.TypeInfo{}
	if validTypeID(l.program, fnInfo.Return) {
		returnInfo = l.program.Types[fnInfo.Return-1]
	}
	discardsEmptyResult := returnInfo.Kind == air.TypeResult && l.isVoidType(returnInfo.Value) && expr.ForeignResultShape == air.ForeignResultValueError
	discardsEmptyMaybe := returnInfo.Kind == air.TypeMaybe && l.isVoidType(returnInfo.Elem) && expr.ForeignResultShape == air.ForeignResultValueBool
	// Ard Result/Maybe returns normally use their idiomatic Go ABI in function
	// type position. Empty success values are the exception: Go returns
	// (struct{}, error/bool), while the Ard callable ABI omits struct{}.
	if !needsArgAdaptation && !discardsEmptyResult && !discardsEmptyMaybe {
		return callee, nil
	}

	callableTemp := l.nextTemp()
	stmts := append([]ast.Stmt{}, callee.stmts...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(callableTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{callee.expr}})
	params := make([]*ast.Field, len(fnInfo.Params))
	args := make([]ast.Expr, len(fnInfo.Params))
	var bodyPrefix []ast.Stmt
	for i, paramType := range fnInfo.Params {
		name := fmt.Sprintf("arg%d", i+1)
		typ, err := l.goType(paramType)
		if err != nil {
			return loweredExpr{}, err
		}
		argExpr := ast.Expr(l.ident(name))
		if fnInfo.Variadic && i == len(fnInfo.Params)-1 {
			typ = &ast.Ellipsis{Elt: typ}
			if expr.ForeignArgABI[i] == air.ABIParamDescriptorValue {
				paramInfo := l.program.Types[paramType-1]
				if paramInfo.Kind != air.TypeReference {
					return loweredExpr{}, fmt.Errorf("variadic foreign callable %s descriptor element is not a reference", expr.ForeignSymbol)
				}
				elemType, err := l.goType(paramInfo.Elem)
				if err != nil {
					return loweredExpr{}, err
				}
				projectedName := l.nextTemp()
				indexName := l.nextTemp()
				itemName := l.nextTemp()
				projectedType := &ast.ArrayType{Elt: elemType}
				projectRange := &ast.RangeStmt{
					Key: l.ident(indexName), Value: l.ident(itemName), Tok: token.DEFINE, X: l.ident(name),
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IndexExpr{X: l.ident(projectedName), Index: l.ident(indexName)}}, Tok: token.ASSIGN,
						Rhs: []ast.Expr{l.foreignABIValueArg(air.Expr{Type: paramType}, l.ident(itemName), expr.ForeignArgABI[i])},
					}}},
				}
				bodyPrefix = append(bodyPrefix,
					&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(projectedName)}, Type: projectedType}}}},
					&ast.IfStmt{
						Cond: &ast.BinaryExpr{X: l.ident(name), Op: token.NEQ, Y: l.ident("nil")},
						Body: &ast.BlockStmt{List: []ast.Stmt{
							&ast.AssignStmt{Lhs: []ast.Expr{l.ident(projectedName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("make"), Args: []ast.Expr{projectedType, &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(name)}}}}}},
							projectRange,
						}},
					},
				)
				argExpr = l.ident(projectedName)
			} else if expr.ForeignArgABI[i] != air.ABIParamExact {
				return loweredExpr{}, fmt.Errorf("variadic foreign callable %s has unsupported element ABI mode %d", expr.ForeignSymbol, expr.ForeignArgABI[i])
			}
		} else {
			argExpr = l.foreignABIValueArg(air.Expr{Type: paramType}, argExpr, expr.ForeignArgABI[i])
		}
		params[i] = &ast.Field{Names: []*ast.Ident{l.ident(name)}, Type: typ}
		args[i] = argExpr
	}
	call := &ast.CallExpr{Fun: l.ident(callableTemp), Args: args}
	if fnInfo.Variadic {
		call.Ellipsis = token.Pos(1)
	}
	body := append([]ast.Stmt{}, bodyPrefix...)
	switch {
	case discardsEmptyResult || discardsEmptyMaybe:
		resultName := l.nextTemp()
		body = append(body,
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident("_"), l.ident(resultName)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			&ast.ReturnStmt{Results: []ast.Expr{l.ident(resultName)}},
		)
	case l.isVoidType(fnInfo.Return):
		body = append(body, &ast.ExprStmt{X: call})
	default:
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	results, err := l.goTypeInfoReturnFields(fnInfo)
	if err != nil {
		return loweredExpr{}, err
	}
	if len(results) > 0 {
		funcType.Results = &ast.FieldList{List: results}
	}
	return loweredExpr{stmts: stmts, expr: &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: body}}}, nil
}

func (l *lowerer) lowerForeignMethodValue(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.ForeignTarget != "go" {
		return loweredExpr{}, fmt.Errorf("unsupported foreign method target %q", expr.ForeignTarget)
	}
	if expr.Target == nil || expr.ForeignSymbol == "" {
		return loweredExpr{}, fmt.Errorf("invalid go foreign method value %q", expr.ForeignSymbol)
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	selector := &ast.SelectorExpr{X: target.expr, Sel: l.ident(expr.ForeignSymbol)}
	return l.lowerForeignCallableValue(fn, expr, loweredExpr{stmts: target.stmts, expr: selector})
}

func (l *lowerer) resultErrorReturnIfStmt(resultType ast.Expr, errName ast.Expr) ast.Stmt {
	return &ast.IfStmt{Cond: &ast.BinaryExpr{X: errName, Op: token.NEQ, Y: l.ident("nil")}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{&ast.KeyValueExpr{Key: l.ident("Err"), Value: &ast.CallExpr{Fun: &ast.SelectorExpr{X: errName, Sel: l.ident("Error")}}}}}}}}}}
}

func (l *lowerer) lowerForeignMethodCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.ForeignTarget != "go" {
		return loweredExpr{}, fmt.Errorf("unsupported foreign method target %q", expr.ForeignTarget)
	}
	if expr.Target == nil || expr.ForeignSymbol == "" {
		return loweredExpr{}, fmt.Errorf("invalid go foreign method call %q", expr.ForeignSymbol)
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	args := make([]ast.Expr, 0, len(expr.Args))
	stmts := append([]ast.Stmt{}, target.stmts...)
	for i := range expr.Args {
		arg, err := l.lowerExpr(fn, expr.Args[i])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, arg.stmts...)
		mode := expr.ForeignArgABI[i]
		args = append(args, l.foreignABIValueArg(expr.Args[i], arg.expr, mode))
	}
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: l.ident(expr.ForeignSymbol)}, Args: args}
	if validTypeID(l.program, expr.Type) {
		if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeResult {
			if expr.ForeignResultShape == air.ForeignResultUnknown {
				return loweredExpr{}, fmt.Errorf("Go foreign method call %s.%s is missing its result shape", expr.ForeignReceiver, expr.ForeignSymbol)
			}
			switch expr.ForeignResultShape {
			case air.ForeignResultValueError:
				return l.lowerGoValueErrorResultCall(expr, stmts, call, info)
			case air.ForeignResultErrorOnly:
				return l.lowerGoErrorOnlyResultCall(expr, stmts, call)
			}
		}
		if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeMaybe {
			if expr.ForeignResultShape == air.ForeignResultUnknown {
				return loweredExpr{}, fmt.Errorf("Go foreign method call %s.%s is missing its result shape", expr.ForeignReceiver, expr.ForeignSymbol)
			}
			if expr.ForeignResultShape == air.ForeignResultValueBool {
				return l.lowerGoValueBoolMaybeCall(expr, stmts, call)
			}
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
	// The temp must use the call site's instantiated value type; the callee's
	// declared type may be an uninstantiated type parameter.
	valueTypeID := result.Value
	if exprInfo, ok := l.typeInfo(expr.Type); ok && exprInfo.Kind == air.TypeResult && validTypeID(l.program, exprInfo.Value) {
		valueTypeID = exprInfo.Value
	}
	valueType, err := l.goType(valueTypeID)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(valueTemp)}, Type: valueType}}}})
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(errTemp)}, Type: l.ident("error")}}}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueTemp), l.ident(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	okLit := &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Value"), Value: l.ident(valueTemp)},
		&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")},
	}}
	errResult := ast.Expr(&ast.CallExpr{Fun: &ast.SelectorExpr{X: l.ident(errTemp), Sel: l.ident("Error")}})
	if exprInfo, ok := l.typeInfo(expr.Type); ok && exprInfo.Kind == air.TypeResult && l.isBuiltinErrorType(exprInfo.Error) {
		errResult = l.ident(errTemp)
	}
	errLit := &ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Err"), Value: errResult},
	}}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: l.ident(errTemp), Op: token.NEQ, Y: l.ident("nil")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{errLit}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{okLit}}}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
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
	return append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr}})
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
		return l.ident("nil"), nil
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeInt, air.TypeScalar, air.TypeByte, air.TypeRune, air.TypeEnum:
		return &ast.BasicLit{Kind: token.INT, Value: "0"}, nil
	case air.TypeForeignType:
		if info.ForeignPointer {
			return l.ident("nil"), nil
		}
		if validTypeID(l.program, info.Value) && !validTypeID(l.program, info.Key) {
			return l.zeroValueExpr(info.Value)
		}
		typ, err := l.goType(typeID)
		if err != nil {
			return nil, err
		}
		return &ast.CompositeLit{Type: typ}, nil
	case air.TypeFloat64:
		return &ast.BasicLit{Kind: token.FLOAT, Value: "0"}, nil
	case air.TypeBool:
		return l.ident("false"), nil
	case air.TypeStr:
		return &ast.BasicLit{Kind: token.STRING, Value: "\"\""}, nil
	case air.TypeAny, air.TypeFunction, air.TypeTraitObject, air.TypeReference:
		return l.ident("nil"), nil
	case air.TypeParam:
		// A composite literal T{} is illegal for a type parameter; *new(T)
		// is the canonical zero-value expression for any T.
		typ, err := l.goType(typeID)
		if err != nil {
			return nil, err
		}
		return &ast.StarExpr{X: &ast.CallExpr{Fun: l.ident("new"), Args: []ast.Expr{typ}}}, nil
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
	info, ok := l.typeInfoThroughReference(mapTypeID)
	if !ok {
		return air.NoType, air.NoType
	}
	if info.Kind != air.TypeMap && !(info.Kind == air.TypeForeignType && validTypeID(l.program, info.Key) && validTypeID(l.program, info.Value)) {
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

func (l *lowerer) isReferenceType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeReference
}

func (l *lowerer) traitReference(typeID air.TypeID) (air.Trait, bool) {
	if !l.isReferenceType(typeID) {
		return air.Trait{}, false
	}
	elem := l.program.Types[typeID-1].Elem
	if !l.isTraitObjectType(elem) {
		return air.Trait{}, false
	}
	traitID := l.program.Types[elem-1].Trait
	if !validTraitID(l.program, traitID) {
		return air.Trait{}, false
	}
	return l.program.Traits[traitID], true
}

func (l *lowerer) referentType(typeID air.TypeID) (air.TypeID, bool) {
	if !l.isReferenceType(typeID) {
		return typeID, false
	}
	return l.program.Types[typeID-1].Elem, true
}

func (l *lowerer) valueThroughReference(typeID air.TypeID, value ast.Expr) ast.Expr {
	if l.isReferenceType(typeID) {
		return &ast.StarExpr{X: value}
	}
	return value
}

func (l *lowerer) typeInfoThroughReference(typeID air.TypeID) (air.TypeInfo, bool) {
	if referent, reference := l.referentType(typeID); reference {
		typeID = referent
	}
	if !validTypeID(l.program, typeID) {
		return air.TypeInfo{}, false
	}
	return l.program.Types[typeID-1], true
}

func (l *lowerer) isTraitObjectType(typeID air.TypeID) bool {
	return validTypeID(l.program, typeID) && l.program.Types[typeID-1].Kind == air.TypeTraitObject
}

func (l *lowerer) mutableTraitRefType(typeID air.TypeID) (ast.Expr, error) {
	if !l.isTraitObjectType(typeID) {
		return nil, fmt.Errorf("type %d is not a trait object", typeID)
	}
	return l.ident("any"), nil
}

func (l *lowerer) goTypeInfoReturnFields(info air.TypeInfo) ([]*ast.Field, error) {
	return l.goReturnFields(info.Return)
}

func (l *lowerer) goSignatureReturnFields(_ air.Signature, typeID air.TypeID) ([]*ast.Field, error) {
	return l.goReturnFields(typeID)
}

func (l *lowerer) goReturnFields(typeID air.TypeID) ([]*ast.Field, error) {
	if typeID == air.NoType || l.isVoidType(typeID) {
		return nil, nil
	}
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid return type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeResult:
		if !l.resultUsesGoErrorABI(typeID) {
			typ, err := l.goType(typeID)
			if err != nil {
				return nil, err
			}
			return []*ast.Field{{Type: typ}}, nil
		}
		if l.isVoidType(info.Value) {
			return []*ast.Field{{Type: l.ident("error")}}, nil
		}
		valueType, err := l.goType(info.Value)
		if err != nil {
			return nil, err
		}
		return []*ast.Field{{Type: valueType}, {Type: l.ident("error")}}, nil
	case air.TypeMaybe:
		if l.isVoidType(info.Elem) {
			return []*ast.Field{{Type: l.ident("bool")}}, nil
		}
		elemType, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return []*ast.Field{{Type: elemType}, {Type: l.ident("bool")}}, nil
	default:
		typ, err := l.goType(typeID)
		if err != nil {
			return nil, err
		}
		return []*ast.Field{{Type: typ}}, nil
	}
}

func (l *lowerer) usesABIResultReturn(typeID air.TypeID) bool {
	return !l.forceValueResultReturns && l.abiReturnShapeAvailable(typeID)
}

func (l *lowerer) abiReturnShapeAvailable(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return (info.Kind == air.TypeResult && l.resultUsesGoErrorABI(typeID)) || info.Kind == air.TypeMaybe
}

func (l *lowerer) isBuiltinErrorType(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return info.Kind == air.TypeTraitObject && int(info.Trait) < len(l.program.Traits) && l.program.Traits[info.Trait].BuiltinError
}

func (l *lowerer) resultUsesGoErrorABI(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return info.Kind == air.TypeResult && (l.resultErrorIsStr(typeID) || l.isBuiltinErrorType(info.Error))
}

func (l *lowerer) resultErrorIsStr(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	return info.Kind == air.TypeResult && validTypeID(l.program, info.Error) && l.program.Types[info.Error-1].Kind == air.TypeStr
}

func (l *lowerer) mutableParamUsesPointer(typeID air.TypeID) bool {
	if !validTypeID(l.program, typeID) || l.isVoidType(typeID) {
		return false
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeReference:
		// The type already is the pointer-shaped handle; legacy mutable flags
		// must not add another layer (ADR 0057).
		return false
	case air.TypeMap, air.TypeChannel, air.TypeReceiver, air.TypeSender:
		return false
	case air.TypeForeignType:
		// Named Go map and slice types are descriptors like their unnamed
		// shapes: content mutation flows through the value without a pointer.
		if info.Key != air.NoType && info.Value != air.NoType {
			return false
		}
		if info.Elem != air.NoType {
			return false
		}
		return !info.ForeignPointer && !info.ForeignInterface
	default:
		return true
	}
}

func (l *lowerer) mutableParamType(typeID air.TypeID) (ast.Expr, error) {
	typ, err := l.goType(typeID)
	if err != nil {
		return nil, err
	}
	if !l.mutableParamUsesPointer(typeID) {
		return typ, nil
	}
	if l.isTraitObjectType(typeID) {
		typ, err = l.mutableTraitRefType(typeID)
		if err != nil {
			return nil, err
		}
	}
	return &ast.StarExpr{X: typ}, nil
}

func (l *lowerer) goParamType(param air.Param) (ast.Expr, error) {
	return l.goType(param.Type)
}

func (l *lowerer) goFunctionParamType(_ air.Function, param air.Param) (ast.Expr, error) {
	if param.ABI == air.ABIParamDescriptorValue {
		return l.goType(l.program.Types[param.Type-1].Elem)
	}
	return l.goParamType(param)
}

func (l *lowerer) modulePathForType(typeID air.TypeID) string {
	if validTypeID(l.program, typeID) && l.typeModulePaths != nil {
		return l.typeModulePaths[typeID]
	}
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

func goScalarTypeName(name string) string {
	switch name {
	case "Int8":
		return "int8"
	case "Int16":
		return "int16"
	case "Int32":
		return "int32"
	case "Int64":
		return "int64"
	case "Uint":
		return "uint"
	case "Uint8":
		return "uint8"
	case "Uint16":
		return "uint16"
	case "Uint32":
		return "uint32"
	case "Uint64":
		return "uint64"
	case "Uintptr":
		return "uintptr"
	case "Float32":
		return "float32"
	default:
		return name
	}
}

func (l *lowerer) goType(typeID air.TypeID) (ast.Expr, error) {
	if cached, ok := l.goTypeCache[typeID]; ok {
		return cached, nil
	}
	expr, err := l.buildGoType(typeID)
	if err != nil {
		return nil, err
	}
	if l.goTypeCache == nil {
		l.goTypeCache = map[air.TypeID]ast.Expr{}
	}
	l.goTypeCache[typeID] = expr
	return expr, nil
}

func (l *lowerer) buildGoType(typeID air.TypeID) (ast.Expr, error) {
	if !validTypeID(l.program, typeID) {
		return nil, fmt.Errorf("invalid type id %d", typeID)
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeVoid:
		return l.voidTypeExpr(), nil
	case air.TypeInt:
		return l.ident("int"), nil
	case air.TypeScalar:
		return l.ident(goScalarTypeName(info.Name)), nil
	case air.TypeForeignType:
		if info.ForeignTarget != "go" {
			return nil, fmt.Errorf("unsupported foreign type target %q", info.ForeignTarget)
		}
		typ := l.qualified(info.ForeignQualifier, info.ForeignNamespace, info.ForeignSymbol)
		if len(info.GenericArgs) > 0 {
			args := make([]ast.Expr, 0, len(info.GenericArgs))
			for _, argID := range info.GenericArgs {
				arg, err := l.goType(argID)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
			typ = &ast.IndexListExpr{X: typ, Indices: args}
		}
		if info.ForeignPointer {
			return &ast.StarExpr{X: typ}, nil
		}
		return typ, nil
	case air.TypeByte:
		return l.ident("byte"), nil
	case air.TypeRune:
		return l.ident("rune"), nil
	case air.TypeFloat64:
		return l.ident("float64"), nil
	case air.TypeBool:
		return l.ident("bool"), nil
	case air.TypeStr:
		return l.ident("string"), nil
	case air.TypeMaybe:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: l.runtimeQualified("Maybe"), Index: elem}, nil
	case air.TypeFunction:
		params := make([]*ast.Field, 0, len(info.Params))
		for i, paramTypeID := range info.Params {
			paramType, err := l.goType(paramTypeID)
			if err != nil {
				return nil, err
			}
			if info.Variadic && i == len(info.Params)-1 {
				paramType = &ast.Ellipsis{Elt: paramType}
			}
			params = append(params, &ast.Field{Type: paramType})
		}
		fnType := &ast.FuncType{Params: &ast.FieldList{List: params}}
		results, err := l.goTypeInfoReturnFields(info)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			fnType.Results = &ast.FieldList{List: results}
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
		return &ast.IndexListExpr{X: l.runtimeQualified("Result"), Indices: []ast.Expr{value, errType}}, nil
	case air.TypeList, air.TypeSlice:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Elt: elem}, nil
	case air.TypeFixedArray:
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Len: l.ident(fmt.Sprintf("%d", info.Length)), Elt: elem}, nil
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
	case air.TypeReference:
		if l.isTraitObjectType(info.Elem) {
			return l.mutableTraitRefType(info.Elem)
		}
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.StarExpr{X: elem}, nil
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
		return l.ident(info.Name), nil
	case air.TypeStruct, air.TypeEnum:
		return l.namedTypeExpr(info), nil
	case air.TypeUnion:
		return l.namedTypeExpr(info), nil
	case air.TypeAny:
		return l.ident("any"), nil
	case air.TypeTraitObject:
		if l.isBuiltinErrorType(typeID) {
			return l.ident("error"), nil
		}
		if l.usesNativeTraitInterface(typeID) {
			return l.traitInterfaceTypeExpr(l.program.Traits[info.Trait]), nil
		}
		return l.ident("any"), nil
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

func (l *lowerer) lowerCallArgs(fn air.Function, rawArgs []air.Expr, params []air.Param) ([]ast.Expr, []ast.Stmt, []ast.Stmt, error) {
	args := make([]ast.Expr, 0, len(rawArgs))
	stmts := []ast.Stmt{}
	writeback := []ast.Stmt{}
	for i, arg := range rawArgs {
		var loweredArg loweredExpr
		var err error
		if i < len(params) && !l.isReferenceType(params[i].Type) {
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
		return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
	}
	resultTemp := l.nextTemp()
	resultType, err := l.goType(typeID)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts,
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(resultTemp)}, Type: resultType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	)
	stmts = append(stmts, writeback...)
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
}

func (l *lowerer) lowerDiscardingFunctionCoercion(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("discarding function coercion missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	actualType, err := l.goType(expr.Target.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	expectedTypeExpr, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	expectedType, ok := expectedTypeExpr.(*ast.FuncType)
	if !ok {
		return loweredExpr{}, fmt.Errorf("discarding function coercion target type %d is not a function", expr.Type)
	}

	args := make([]ast.Expr, 0, len(expectedType.Params.List))
	params := make([]*ast.Field, 0, len(expectedType.Params.List))
	variadic := false
	for i, field := range expectedType.Params.List {
		name := fmt.Sprintf("arg%d", i)
		args = append(args, l.ident(name))
		params = append(params, &ast.Field{Names: []*ast.Ident{l.ident(name)}, Type: field.Type})
		if i == len(expectedType.Params.List)-1 {
			_, variadic = field.Type.(*ast.Ellipsis)
		}
	}
	original := l.ident("original")
	call := &ast.CallExpr{Fun: original, Args: args}
	if variadic {
		call.Ellipsis = token.Pos(1)
	}
	wrapper := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: params}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
	}
	adapter := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{original}, Type: actualType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: expectedTypeExpr}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{wrapper}}}},
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: adapter, Args: []ast.Expr{target.expr}}}, nil
}

func (l *lowerer) adaptCallArgWithStmts(_ air.Function, arg air.Expr, argExpr ast.Expr, param air.Param) (ast.Expr, []ast.Stmt, []ast.Stmt, error) {
	if param.ABI == air.ABIParamDescriptorValue {
		return l.valueThroughReference(arg.Type, argExpr), nil, nil, nil
	}
	return argExpr, nil, nil, nil
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
		if len(methodFn.Signature.Params) > 0 && l.isReferenceType(methodFn.Signature.Params[0].Type) {
			return true
		}
	}
	return false
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
		if targetType.Kind == air.TypeReference && validTypeID(l.program, targetType.Elem) {
			targetType = l.program.Types[targetType.Elem-1]
		}
		if targetType.Kind != air.TypeStruct || arg.Field < 0 || arg.Field >= len(targetType.Fields) {
			return nil, nil, false, nil
		}
		field := targetType.Fields[arg.Field]
		fieldTarget := ast.Expr(&ast.SelectorExpr{X: targetPlace, Sel: l.ident(l.goFieldName(targetType, field.Name))})
		if l.isReferenceType(field.Type) {
			fieldTarget = &ast.StarExpr{X: fieldTarget}
		}
		return fieldTarget, setup, true, nil
	default:
		return nil, nil, false, nil
	}
}

// lowerMutRef lowers one of AIR's explicit reference creation modes. Every
// Ard-owned TypeReference is represented by a direct Go pointer, including
// descriptor referents; copying a reference therefore copies only the handle.
func (l *lowerer) lowerMutRef(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("mut ref missing operand")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	if l.isReferenceType(expr.Type) {
		reference := l.program.Types[expr.Type-1]
		if l.isTraitObjectType(reference.Elem) && expr.ReferenceMode != air.ExistingReference {
			stmts := append([]ast.Stmt{}, target.stmts...)
			var place ast.Expr
			switch expr.ReferenceMode {
			case air.AddressablePlace:
				place = addressOfPlace(target.expr)
			case air.FreshValue:
				temp := l.nextTemp()
				stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{target.expr}})
				place = &ast.UnaryExpr{Op: token.AND, X: l.ident(temp)}
			default:
				return loweredExpr{}, fmt.Errorf("trait mut ref has invalid mode %d", expr.ReferenceMode)
			}
			return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{place}}}, nil
		}
	}
	switch expr.ReferenceMode {
	case air.ExistingReference:
		return target, nil
	case air.AddressablePlace:
		return loweredExpr{stmts: target.stmts, expr: addressOfPlace(target.expr)}, nil
	case air.FreshValue:
		// Composite literals can be addressed directly. Every other expression
		// is evaluated once into escaping stable storage first.
		if _, isComposite := target.expr.(*ast.CompositeLit); isComposite {
			return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.AND, X: target.expr}}, nil
		}
		tmp := l.nextTemp()
		stmts := append([]ast.Stmt{}, target.stmts...)
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{target.expr}})
		return loweredExpr{stmts: stmts, expr: &ast.UnaryExpr{Op: token.AND, X: l.ident(tmp)}}, nil
	default:
		return loweredExpr{}, fmt.Errorf("mut ref has invalid mode %d", expr.ReferenceMode)
	}
}

func (l *lowerer) localValueExpr(fn air.Function, local air.LocalID) ast.Expr {
	name := ast.Expr(l.ident(l.localName(fn, local)))
	if l.foreignABIValueReferenceParam(fn, local) {
		return &ast.UnaryExpr{Op: token.AND, X: name}
	}
	if l.captureMode(fn, local) == air.CaptureSlot {
		return &ast.StarExpr{X: name}
	}
	if int(local) >= 0 && int(local) < len(fn.Locals) && l.isReferenceType(fn.Locals[local].Type) {
		// First-class references load their current handle. Only ExprDeref
		// reads the referent (ADR 0057).
		return name
	}
	if l.localIsPointerParam(fn, local) {
		return &ast.StarExpr{X: name}
	}
	return name
}

func (l *lowerer) localAssignExpr(fn air.Function, local air.LocalID) ast.Expr {
	if l.captureMode(fn, local) == air.CaptureSlot {
		return &ast.StarExpr{X: l.ident(l.localName(fn, local))}
	}
	if int(local) >= 0 && int(local) < len(fn.Locals) && l.isReferenceType(fn.Locals[local].Type) {
		return l.ident(l.localName(fn, local))
	}
	return l.localValueExpr(fn, local)
}

func (l *lowerer) foreignABIValueReferenceParam(fn air.Function, local air.LocalID) bool {
	index := int(local)
	if index < 0 || index >= len(fn.Signature.Params) {
		return false
	}
	return fn.Signature.Params[index].ABI == air.ABIParamDescriptorValue
}

func (l *lowerer) captureMode(fn air.Function, local air.LocalID) air.CaptureMode {
	for _, capture := range fn.Captures {
		if capture.Local == local {
			return capture.Mode
		}
	}
	return air.CaptureValue
}

func (l *lowerer) localIsReference(fn air.Function, local air.LocalID) bool {
	idx := int(local)
	return idx >= 0 && idx < len(fn.Locals) && fn.Locals[idx].Reference
}

func (l *lowerer) localIsPointerParam(fn air.Function, local air.LocalID) bool {
	idx := int(local)
	if idx >= 0 && idx < len(fn.Locals) && l.isReferenceType(fn.Locals[idx].Type) {
		return false
	}
	if l.localIsReference(fn, local) {
		// Reference locals hold a Go pointer only when the referent's
		// representation requires one; descriptor-backed referents are
		// stored as their sharing value forms (ADR 0040).
		idx := int(local)
		if idx >= 0 && idx < len(fn.Locals) {
			return l.mutableParamUsesPointer(fn.Locals[idx].Type)
		}
		return true
	}
	if idx >= 0 && idx < len(fn.Signature.Params) {
		return false
	}
	for _, capture := range fn.Captures {
		if capture.Local == local {
			return capture.Mode == air.CaptureSlot
		}
	}
	return false
}

func (l *lowerer) runtimeQualified(name string) ast.Expr {
	return l.qualified("ard", path.Join(l.generatedModulePath, "internal", "ard"), name)
}

func (l *lowerer) qualified(alias string, importPath string, name string) ast.Expr {
	alias = l.registerImport(alias, importPath)
	return &ast.SelectorExpr{X: l.ident(alias), Sel: l.ident(name)}
}

func (l *lowerer) registerImport(alias string, importPath string) string {
	if alias == "" || importPath == "" {
		return alias
	}
	key := importAliasKey{alias: alias, path: importPath}
	if resolved, ok := l.resolvedImportAliases[key]; ok {
		return resolved
	}
	if l.currentImports == nil {
		l.currentImports = map[string]string{}
	}
	if l.resolvedImportAliases == nil {
		l.resolvedImportAliases = map[importAliasKey]string{}
	}
	if existing, ok := l.currentImports[alias]; ok && existing == importPath {
		return alias
	}
	chosen := alias
	for i := 1; ; i++ {
		if l.importAliasAvailable(chosen, importPath) {
			l.currentImports[chosen] = importPath
			l.resolvedImportAliases[key] = chosen
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
	if l.namePlan != nil {
		return l.namePlan.importAliasCollides(l.useModulePackages, l.currentModule, alias)
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
		if l.typeTopLevelNameCollidesWithImportAlias(*typ, alias) {
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
		case air.TypeFloat64:
			return &ast.CallExpr{Fun: l.qualified("strconv", "strconv", "FormatFloat"), Args: []ast.Expr{expr, &ast.BasicLit{Kind: token.CHAR, Value: "'f'"}, &ast.BasicLit{Kind: token.INT, Value: "2"}, &ast.BasicLit{Kind: token.INT, Value: "64"}}}
		case air.TypeRune:
			return &ast.CallExpr{Fun: l.ident("string"), Args: []ast.Expr{expr}}
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
		&ast.KeyValueExpr{Key: l.ident(unionTagFieldName(unionType)), Value: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", expr.Tag)}},
		&ast.KeyValueExpr{Key: l.ident(fieldName), Value: fieldValue},
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
	resultExpr := l.ident("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
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
		bind := &ast.AssignStmt{Lhs: []ast.Expr{l.ident(localName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: target.expr, Sel: l.ident(fieldName)}}}
		body, err := l.lowerValueBlock(fn, unionCase.Body, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		body = append([]ast.Stmt{bind, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(localName)}}}, body...)
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", unionCase.Tag)}}, Body: body})
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		body, err := l.lowerValueBlock(fn, expr.CatchAll, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{Body: body})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: target.expr, Sel: l.ident(unionTagFieldName(unionType))}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

// lowerMatchForeignType lowers a dynamic foreign type test (ADR 0042) to a Go
// type switch over the subject's dynamic type.
func (l *lowerer) lowerMatchForeignType(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("foreign type match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := l.ident("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
	}
	switchLocal := l.nextTemp()
	cases := make([]ast.Stmt, 0, len(expr.ForeignCases)+1)
	for _, foreignCase := range expr.ForeignCases {
		caseType, err := l.goType(foreignCase.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		body, err := l.lowerValueBlock(fn, foreignCase.Body, expr.Type, assignTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		if foreignCase.Bound {
			localName := l.localName(fn, foreignCase.Local)
			l.declaredLocals[foreignCase.Local] = true
			bind := &ast.AssignStmt{Lhs: []ast.Expr{l.ident(localName)}, Tok: token.DEFINE, Rhs: []ast.Expr{l.ident(switchLocal)}}
			discard := &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(localName)}}
			body = append([]ast.Stmt{bind, discard}, body...)
		}
		cases = append(cases, &ast.CaseClause{List: []ast.Expr{caseType}, Body: body})
	}
	catchAll, err := l.lowerValueBlock(fn, expr.CatchAll, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	cases = append(cases, &ast.CaseClause{Body: catchAll})
	anyBound := false
	for _, foreignCase := range expr.ForeignCases {
		if foreignCase.Bound {
			anyBound = true
			break
		}
	}
	typeSwitch := &ast.TypeSwitchStmt{Body: &ast.BlockStmt{List: cases}}
	if anyBound {
		typeSwitch.Assign = &ast.AssignStmt{Lhs: []ast.Expr{l.ident(switchLocal)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: target.expr, Type: nil}}}
	} else {
		typeSwitch.Assign = &ast.ExprStmt{X: &ast.TypeAssertExpr{X: target.expr, Type: nil}}
	}
	stmts = append(stmts, typeSwitch)
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
	resultExpr := l.ident("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
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
	stmts = append(stmts, &ast.SwitchStmt{Tag: l.ident("true"), Body: &ast.BlockStmt{List: cases}})
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
	resultExpr := l.ident("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
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
	resultExpr := l.ident("nil")
	stmts := append([]ast.Stmt{}, target.stmts...)
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
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
	resultExpr := l.ident(resultTemp)
	stmts := append(target.stmts, message.stmts...)
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	if l.isVoidType(expr.Type) {
		stmts = append(stmts, &ast.IfStmt{
			Cond: l.maybeIsSomeExpr(resultExpr),
			Body: &ast.BlockStmt{},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("panic"), Args: []ast.Expr{message.expr}}}}},
		})
		return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareReferenceAwareTemp(expr.Type, temp, expr.Bool)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(resultExpr),
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(resultExpr)}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("panic"), Args: []ast.Expr{message.expr}}}}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(temp)}, nil
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
	targetExpr := l.ident(targetTemp)
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareReferenceAwareTemp(expr.Type, resultTemp, expr.Bool)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := l.ident(resultTemp)
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
	targetExpr := l.ident(targetTemp)
	resultTemp := l.nextTemp()
	resultDecls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := l.ident(resultTemp)
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
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{defaultExpr}}}},
	})
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerMaybeSet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("maybe set expects target and one arg")
	}
	if expr.Target.Kind != air.ExprLoadLocal && expr.Target.Kind != air.ExprGetField && expr.Target.Kind != air.ExprLoadGlobal {
		return loweredExpr{}, fmt.Errorf("maybe set requires an addressable local, field, or global target")
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("maybe set target type is invalid")
	}
	maybeType := expr.Target.Type
	if referent, reference := l.referentType(maybeType); reference {
		maybeType = referent
	}
	maybeInfo := l.program.Types[maybeType-1]
	if maybeInfo.Kind != air.TypeMaybe {
		return loweredExpr{}, fmt.Errorf("maybe set target is not Maybe")
	}
	value, err := l.lowerExprWithExpectedType(fn, expr.Args[0], maybeInfo.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	var target ast.Expr
	var targetStmts []ast.Stmt
	if expr.Target.Kind == air.ExprLoadLocal {
		target = l.localValueExpr(fn, expr.Target.Local)
	} else {
		loweredTarget, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		target = loweredTarget.expr
		targetStmts = loweredTarget.stmts
	}
	target = l.valueThroughReference(expr.Target.Type, target)
	valueExpr := value.expr
	stmts := append(append([]ast.Stmt{}, targetStmts...), value.stmts...)
	if l.isVoidType(maybeInfo.Elem) || isVoidExpr(valueExpr) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	some, err := l.maybeSomeExpr(maybeType, valueExpr)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{some}})
	return loweredExpr{stmts: stmts, expr: l.voidValueExpr()}, nil
}

func (l *lowerer) lowerMaybeClear(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 0 {
		return loweredExpr{}, fmt.Errorf("maybe clear expects target and no args")
	}
	if expr.Target.Kind != air.ExprLoadLocal && expr.Target.Kind != air.ExprGetField && expr.Target.Kind != air.ExprLoadGlobal {
		return loweredExpr{}, fmt.Errorf("maybe clear requires an addressable local, field, or global target")
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("maybe clear target type is invalid")
	}
	maybeType := expr.Target.Type
	if referent, reference := l.referentType(maybeType); reference {
		maybeType = referent
	}
	maybeInfo := l.program.Types[maybeType-1]
	if maybeInfo.Kind != air.TypeMaybe {
		return loweredExpr{}, fmt.Errorf("maybe clear target is not Maybe")
	}
	var target ast.Expr
	var targetStmts []ast.Stmt
	if expr.Target.Kind == air.ExprLoadLocal {
		target = l.localValueExpr(fn, expr.Target.Local)
	} else {
		loweredTarget, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		target = loweredTarget.expr
		targetStmts = loweredTarget.stmts
	}
	target = l.valueThroughReference(expr.Target.Type, target)
	none, err := l.maybeNoneExpr(maybeType)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]ast.Stmt{}, targetStmts...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{none}})
	return loweredExpr{stmts: stmts, expr: l.voidValueExpr()}, nil
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
	resultExpr := l.ident(resultTemp)
	targetExpr := l.ident(targetTemp)
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
	resultExpr := l.ident(resultTemp)
	targetExpr := l.ident(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{l.maybeValueExpr(targetExpr)}}
	callExpr := ast.Expr(call)
	callStmts := []ast.Stmt{}
	if cbInfo, ok := l.functionTypeInfo(expr.Args[0].Type); ok && l.usesABIResultReturn(cbInfo.Return) {
		packed, err := l.packABICallResult(expr.Type, cbInfo.Return, nil, call)
		if err != nil {
			return loweredExpr{}, err
		}
		callStmts = packed.stmts
		callExpr = packed.expr
	}
	noneExpr, err := l.maybeNoneExpr(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: l.maybeIsSomeExpr(targetExpr),
		Body: &ast.BlockStmt{List: append(callStmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{callExpr}})},
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
	return loweredExpr{stmts: target.stmts, expr: &ast.SelectorExpr{X: target.expr, Sel: l.ident("Ok")}}, nil
}

func (l *lowerer) lowerResultIsErr(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("result is_err missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.UnaryExpr{Op: token.NOT, X: &ast.SelectorExpr{X: target.expr, Sel: l.ident("Ok")}}}, nil
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
	resultExpr := l.ident(resultTemp)
	targetExpr := l.ident(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}}}
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
			&ast.KeyValueExpr{Key: l.ident("Value"), Value: valueExpr},
			&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")},
		}}},
	})
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Ok")},
		Body: &ast.BlockStmt{List: okBody},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: l.ident("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")}},
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
	resultExpr := l.ident(resultTemp)
	targetExpr := l.ident(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")}}}
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
			&ast.KeyValueExpr{Key: l.ident("Err"), Value: errExpr},
		}}},
	})
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: l.ident("Value"), Value: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}},
					&ast.KeyValueExpr{Key: l.ident("Ok"), Value: l.ident("true")},
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
	resultExpr := l.ident(resultTemp)
	targetExpr := l.ident(targetTemp)
	stmts := append(target.stmts, callback.stmts...)
	stmts = append(stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	stmts = append(stmts, resultDecls...)
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callback.expr, Args: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}}}
	callExpr := ast.Expr(call)
	callStmts := []ast.Stmt{}
	if cbInfo, ok := l.functionTypeInfo(expr.Args[0].Type); ok && l.usesABIResultReturn(cbInfo.Return) {
		packed, err := l.packABICallResult(expr.Type, cbInfo.Return, nil, call)
		if err != nil {
			return loweredExpr{}, err
		}
		callStmts = packed.stmts
		callExpr = packed.expr
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Ok")},
		Body: &ast.BlockStmt{List: append(callStmts,
			&ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{callExpr}},
		)},
		Else: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{resultExpr},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CompositeLit{Type: resultType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: l.ident("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")}},
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
	targetExpr := l.ident(targetTemp)
	resultExpr := l.ident("nil")
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
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
	}
	okName := l.localName(fn, expr.OkLocal)
	errName := l.localName(fn, expr.ErrLocal)
	l.declaredLocals[expr.OkLocal] = true
	l.declaredLocals[expr.ErrLocal] = true
	okBind := &ast.AssignStmt{Lhs: []ast.Expr{l.ident(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}}}
	errBind := &ast.AssignStmt{Lhs: []ast.Expr{l.ident(errName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")}}}
	okBody, err := l.lowerValueBlock(fn, expr.Ok, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	okBody = append([]ast.Stmt{okBind, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(okName)}}}, okBody...)
	errBody, err := l.lowerValueBlock(fn, expr.Err, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	errBody = append([]ast.Stmt{errBind, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(errName)}}}, errBody...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Ok")},
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
	resultExpr := l.ident(resultTemp)
	panicMsg := &ast.BinaryExpr{X: message.expr, Op: token.ADD, Y: &ast.BinaryExpr{X: &ast.BasicLit{Kind: token.STRING, Value: `": "`}, Op: token.ADD, Y: &ast.CallExpr{Fun: l.qualified("fmt", "fmt", "Sprint"), Args: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: l.ident("Err")}}}}}
	stmts := append(target.stmts, message.stmts...)
	stmts = append(stmts, resultDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{resultExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	if l.isVoidType(expr.Type) {
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.SelectorExpr{X: resultExpr, Sel: l.ident("Ok")},
			Body: &ast.BlockStmt{},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("panic"), Args: []ast.Expr{panicMsg}}}}},
		})
		return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
	}
	temp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, temp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	stmts = append(stmts, &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: resultExpr, Sel: l.ident("Ok")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: resultExpr, Sel: l.ident("Value")}}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("panic"), Args: []ast.Expr{panicMsg}}}}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(temp)}, nil
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
	targetExpr := l.ident(targetTemp)
	stmts := append(target.stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	var resultExpr ast.Expr = l.ident("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		resultExpr = l.ident(temp)
		assignTarget = resultExpr
	}
	okBody := []ast.Stmt{}
	if assignTarget != nil {
		okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{assignTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}}})
		if expr.HasCatch {
			okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignTarget}})
		}
	} else {
		okBody = append(okBody, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Value")}}})
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
			catchTarget = l.ident(catchTargetName)
		}
		errName := l.localName(fn, expr.CatchLocal)
		l.declaredLocals[expr.CatchLocal] = true
		errBind := &ast.AssignStmt{Lhs: []ast.Expr{l.ident(errName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")}}}
		catchBody, err := l.lowerValueBlock(fn, expr.Catch, fn.Signature.Return, catchTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		elseBody = append(catchDecls, errBind, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(errName)}})
		elseBody = append(elseBody, catchBody...)
		if l.usesABIResultReturn(fn.Signature.Return) {
			// The enclosing function uses the (T, error) tuple ABI (ADR 0038),
			// so the caught Result must be unpacked into that shape rather than
			// returned as a Result value. (#282)
			packed, err := l.returnPackedABIValue(fn.Signature.Return, catchTarget)
			if err != nil {
				return loweredExpr{}, err
			}
			elseBody = append(elseBody, packed...)
		} else if !l.isVoidType(fn.Signature.Return) {
			elseBody = append(elseBody, &ast.ReturnStmt{Results: []ast.Expr{catchTarget}})
		} else {
			elseBody = append(elseBody, &ast.ReturnStmt{})
		}
	} else {
		if l.usesABIResultReturn(fn.Signature.Return) {
			retInfo := l.program.Types[fn.Signature.Return-1]
			if retInfo.Kind != air.TypeResult {
				return loweredExpr{}, fmt.Errorf("cannot propagate Result try through non-Result ABI return")
			}
			errExpr := l.goErrorValueExpr(retInfo.Error, &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")})
			if l.isVoidType(retInfo.Value) {
				elseBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{errExpr}}}
			} else {
				zero, err := l.zeroValueExpr(retInfo.Value)
				if err != nil {
					return loweredExpr{}, err
				}
				elseBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{zero, errExpr}}}
			}
		} else {
			returnExpr := ast.Expr(targetExpr)
			if fn.Signature.Return != expr.Target.Type {
				returnType, err := l.goType(fn.Signature.Return)
				if err != nil {
					return loweredExpr{}, err
				}
				returnExpr = &ast.CompositeLit{Type: returnType, Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: l.ident("Err"), Value: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Err")}},
				}}
			}
			elseBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{returnExpr}}}
		}
	}
	stmts = append(stmts, &ast.IfStmt{Cond: &ast.SelectorExpr{X: targetExpr, Sel: l.ident("Ok")}, Body: &ast.BlockStmt{List: okBody}, Else: &ast.BlockStmt{List: elseBody}})
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
	targetTypeID := expr.Target.Type
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(targetTypeID, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := l.ident(targetTemp)
	stmts := append(target.stmts, targetDecls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{targetExpr}, Tok: token.ASSIGN, Rhs: []ast.Expr{target.expr}})
	resultTypeID := expr.Type
	var resultExpr ast.Expr = l.ident("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(resultTypeID) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(resultTypeID, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, decls...)
		resultExpr = l.ident(temp)
		assignTarget = resultExpr
	}
	someBody := []ast.Stmt{}
	if assignTarget != nil {
		someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{assignTarget}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}})
		if expr.HasCatch {
			someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{assignTarget}})
		}
	} else {
		someBody = append(someBody, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}})
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
			catchTarget = l.ident(catchTargetName)
		}
		catchBody, err := l.lowerValueBlock(fn, expr.Catch, fn.Signature.Return, catchTarget)
		if err != nil {
			return loweredExpr{}, err
		}
		noneBody = append(catchDecls, catchBody...)
		if l.usesABIResultReturn(fn.Signature.Return) {
			// Unpack the caught value into the enclosing function's tuple ABI
			// rather than returning a Result/Maybe value directly. (#282)
			packed, err := l.returnPackedABIValue(fn.Signature.Return, catchTarget)
			if err != nil {
				return loweredExpr{}, err
			}
			noneBody = append(noneBody, packed...)
		} else if !l.isVoidType(fn.Signature.Return) {
			noneBody = append(noneBody, &ast.ReturnStmt{Results: []ast.Expr{catchTarget}})
		} else {
			noneBody = append(noneBody, &ast.ReturnStmt{})
		}
	} else {
		if l.usesABIResultReturn(fn.Signature.Return) {
			retInfo := l.program.Types[fn.Signature.Return-1]
			if retInfo.Kind != air.TypeMaybe {
				return loweredExpr{}, fmt.Errorf("cannot propagate Maybe try through non-Maybe ABI return")
			}
			if retInfo.Kind == air.TypeMaybe {
				if l.isVoidType(retInfo.Elem) {
					noneBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{l.ident("false")}}}
				} else {
					zero, err := l.zeroValueExpr(retInfo.Elem)
					if err != nil {
						return loweredExpr{}, err
					}
					noneBody = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{zero, l.ident("false")}}}
				}
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
	targetTypeID := expr.Target.Type
	targetTemp := l.nextTemp()
	targetDecls, err := l.declareTemp(targetTypeID, targetTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	targetExpr := l.ident(targetTemp)
	resultExpr := l.ident("nil")
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
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
	}
	someName := l.localName(fn, expr.SomeLocal)
	l.declaredLocals[expr.SomeLocal] = true
	someDecl := &ast.AssignStmt{Lhs: []ast.Expr{l.ident(someName)}, Tok: token.DEFINE, Rhs: []ast.Expr{l.maybeValueExpr(targetExpr)}}
	someBody, err := l.lowerValueBlock(fn, expr.Some, expr.Type, assignTarget)
	if err != nil {
		return loweredExpr{}, err
	}
	someBody = append([]ast.Stmt{someDecl, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(someName)}}}, someBody...)
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

func (l *lowerer) lowerMakeList(fn air.Function, expr air.Expr) (loweredExpr, error) {
	typ, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	elts := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	elemType := air.NoType
	if validTypeID(l.program, expr.Type) {
		if info := l.program.Types[expr.Type-1]; info.Kind == air.TypeList || info.Kind == air.TypeFixedArray {
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
	captureNames, stmts, err := l.lowerClosureCaptureSnapshots(fn, expr, closureFn)
	if err != nil {
		return loweredExpr{}, err
	}
	for _, name := range captureNames {
		callArgs = append(callArgs, l.ident(name))
	}
	params := []*ast.Field{}
	for i, param := range closureFn.Signature.Params {
		paramType, err := l.goParamType(param)
		if err != nil {
			return loweredExpr{}, err
		}
		name := l.localName(closureFn, air.LocalID(i))
		params = append(params, &ast.Field{Names: []*ast.Ident{l.ident(name)}, Type: paramType})
		callArgs = append(callArgs, l.ident(name))
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
	if funcType.Results == nil || len(funcType.Results.List) == 0 {
		bodyStmts = append(bodyStmts, &ast.ExprStmt{X: call})
	} else if l.usesABIResultReturn(closureFn.Signature.Return) {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: l.unpackABIResultExprs(closureFn.Signature.Return, call)})
	} else {
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: []ast.Expr{call}})
	}
	funcLit := &ast.FuncLit{Type: funcType, Body: &ast.BlockStmt{List: bodyStmts}}
	return loweredExpr{stmts: stmts, expr: funcLit}, nil
}

func (l *lowerer) lowerClosureCaptureSnapshots(parent air.Function, expr air.Expr, closureFn air.Function) ([]string, []ast.Stmt, error) {
	if len(expr.CaptureLocals) != len(closureFn.Captures) {
		return nil, nil, fmt.Errorf("closure %s has %d capture locals, want %d", closureFn.Name, len(expr.CaptureLocals), len(closureFn.Captures))
	}
	names := make([]string, len(expr.CaptureLocals))
	stmts := make([]ast.Stmt, 0, len(expr.CaptureLocals))
	for i, local := range expr.CaptureLocals {
		capture := closureFn.Captures[i]
		captured := l.localValueExpr(parent, local)
		if capture.Mode == air.CaptureSlot {
			captured = addressOfPlace(l.localAssignExpr(parent, local))
		}
		// Ard captures snapshot either the current value/reference handle or the
		// stable binding slot when the closure value is created (ADR 0057). Give
		// the Go literal a fresh lexical local so later outer rebinding cannot
		// change value/reference captures.
		temp := l.nextTemp()
		names[i] = temp
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.DEFINE, Rhs: []ast.Expr{captured}})
	}
	return names, stmts, nil
}

func (l *lowerer) lowerInlineClosure(parent air.Function, expr air.Expr, closureFn air.Function) (loweredExpr, error) {
	captureNames, stmts, err := l.lowerClosureCaptureSnapshots(parent, expr, closureFn)
	if err != nil {
		return loweredExpr{}, err
	}
	inlineFn := closureFn
	inlineFn.Captures = append([]air.Capture(nil), closureFn.Captures...)
	inlineFn.Locals = append([]air.Local(nil), closureFn.Locals...)
	for i, name := range captureNames {
		capture := &inlineFn.Captures[i]
		if int(capture.Local) < 0 || int(capture.Local) >= len(inlineFn.Locals) {
			return loweredExpr{}, fmt.Errorf("closure %s has invalid capture local %d", closureFn.Name, capture.Local)
		}
		capture.Name = name
		inlineFn.Locals[capture.Local].Name = name
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
	allocatedNames := l.allocateLocalNames(inlineFn)
	for i, capture := range inlineFn.Captures {
		// Generated temps start with `_`, while Ard local names are sanitized to
		// omit leading underscores. Pin captures to the exact outer snapshot name
		// after allocating every source local so neither side can shadow the other.
		allocatedNames[capture.Local] = captureNames[i]
	}

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
		params = append(params, &ast.Field{Names: []*ast.Ident{l.ident(name)}, Type: paramType})
	}
	if funcType == nil {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}}
	} else {
		funcType = &ast.FuncType{Params: &ast.FieldList{List: params}, Results: funcType.Results}
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
	body, err := l.lowerBlock(inlineFn, inlineFn.Body, inlineFn.Signature.Return)
	if err != nil {
		return loweredExpr{}, err
	}
	funcLit := &ast.FuncLit{Type: funcType, Body: body}
	if len(stmts) == 0 {
		return loweredExpr{expr: funcLit}, nil
	}
	// Keep snapshot evaluation at the closure expression's exact position. If
	// setup statements escaped to the surrounding expression, a later closure
	// argument could snapshot before an earlier argument's side effects, and
	// statement-restricted contexts such as while conditions could not use it.
	factoryBody := append(stmts, &ast.ReturnStmt{Results: []ast.Expr{funcLit}})
	factory := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: closureType}}},
		},
		Body: &ast.BlockStmt{List: factoryBody},
	}
	return loweredExpr{expr: &ast.CallExpr{Fun: factory}}, nil
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
		// A named Go func type is called through its underlying signature.
		// Value is overloaded for foreign types (named maps store their value
		// type there and also set Key), so require Key to be unset.
		if info.Kind == air.TypeForeignType && validTypeID(l.program, info.Value) && info.Key == air.NoType {
			if underlying := l.program.Types[info.Value-1]; underlying.Kind == air.TypeFunction {
				targetInfo = underlying
				hasFunctionType = true
			}
		}
	}
	params := []air.Param{}
	if hasFunctionType {
		paramCount := len(targetInfo.Params)
		if targetInfo.Variadic && len(expr.Args) > paramCount {
			paramCount = len(expr.Args)
		}
		params = make([]air.Param, paramCount)
		for i := range params {
			paramIndex := i
			if paramIndex >= len(targetInfo.Params) {
				paramIndex = len(targetInfo.Params) - 1
			}
			params[i] = air.Param{Type: targetInfo.Params[paramIndex]}
		}
	}
	args, stmts, writeback, err := l.lowerCallArgs(fn, expr.Args, params)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(append([]ast.Stmt{}, target.stmts...), stmts...)
	call := &ast.CallExpr{Fun: target.expr, Args: args}
	if hasFunctionType && l.abiReturnShapeAvailable(targetInfo.Return) && len(writeback) == 0 {
		return l.packABICallResult(expr.Type, targetInfo.Return, stmts, call)
	}
	return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
}

func (l *lowerer) lowerCheckedSlice(fn air.Function, expr air.Expr, capToLength bool) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return loweredExpr{}, fmt.Errorf("checked slice expects a target and two nullable bounds")
	}
	if !validTypeID(l.program, expr.Type) || l.program.Types[expr.Type-1].Kind != air.TypeMaybe {
		return loweredExpr{}, fmt.Errorf("checked slice lowered with non-Maybe type %d", expr.Type)
	}

	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]ast.Stmt{}, target.stmts...)
	sourceTemp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{
		Lhs: []ast.Expr{l.ident(sourceTemp)}, Tok: token.DEFINE,
		Rhs: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)},
	})

	boundTemps := make([]string, 2)
	argOrder := expr.ArgOrder
	if len(argOrder) == 0 {
		argOrder = []int{0, 1}
	}
	for _, i := range argOrder {
		if i < 0 || i >= len(expr.Args) || boundTemps[i] != "" {
			return loweredExpr{}, fmt.Errorf("checked slice has invalid argument order %v", argOrder)
		}
		lowered, err := l.lowerExpr(fn, expr.Args[i])
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, lowered.stmts...)
		boundTemps[i] = l.nextTemp()
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(boundTemps[i])}, Tok: token.DEFINE,
			Rhs: []ast.Expr{lowered.expr},
		})
	}
	if boundTemps[0] == "" || boundTemps[1] == "" {
		return loweredExpr{}, fmt.Errorf("checked slice argument order omits a bound")
	}

	startTemp := l.nextTemp()
	endTemp := l.nextTemp()
	stmts = append(stmts,
		&ast.AssignStmt{Lhs: []ast.Expr{l.ident(startTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}}},
		&ast.IfStmt{
			Cond: l.maybeIsSomeExpr(l.ident(boundTemps[0])),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{l.ident(startTemp)}, Tok: token.ASSIGN,
				Rhs: []ast.Expr{l.maybeValueExpr(l.ident(boundTemps[0]))},
			}}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(endTemp)}, Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(sourceTemp)}}},
		},
		&ast.IfStmt{
			Cond: l.maybeIsSomeExpr(l.ident(boundTemps[1])),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{l.ident(endTemp)}, Tok: token.ASSIGN,
				Rhs: []ast.Expr{l.maybeValueExpr(l.ident(boundTemps[1]))},
			}}},
		},
	)

	resultTemp := l.nextTemp()
	decls, err := l.declareTemp(expr.Type, resultTemp)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, decls...)
	noneExpr, err := l.maybeNoneExpr(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}

	sliceExpr := &ast.SliceExpr{
		X:    l.ident(sourceTemp),
		Low:  l.ident(startTemp),
		High: l.ident(endTemp),
	}
	if capToLength {
		sliceExpr.Max = l.ident(endTemp)
		sliceExpr.Slice3 = true
	}
	someExpr, err := l.maybeSomeExpr(expr.Type, sliceExpr)
	if err != nil {
		return loweredExpr{}, err
	}

	invalid := &ast.BinaryExpr{
		X: &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: l.ident(startTemp), Op: token.LSS, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
			Op: token.LOR,
			Y:  &ast.BinaryExpr{X: l.ident(startTemp), Op: token.GTR, Y: l.ident(endTemp)},
		},
		Op: token.LOR,
		Y: &ast.BinaryExpr{
			X: l.ident(endTemp), Op: token.GTR,
			Y: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(sourceTemp)}},
		},
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: invalid,
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{noneExpr},
		}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr},
		}}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
}

func (l *lowerer) lowerSliceToList(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 0 {
		return loweredExpr{}, fmt.Errorf("slice to_list expects a target and no args")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	resultType, err := l.goType(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]ast.Stmt{}, target.stmts...)
	sourceTemp := l.nextTemp()
	resultTemp := l.nextTemp()
	stmts = append(stmts,
		&ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(sourceTemp)}, Tok: token.DEFINE,
			Rhs: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  l.ident("make"),
				Args: []ast.Expr{resultType, &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(sourceTemp)}}},
			}},
		},
		&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("copy"), Args: []ast.Expr{l.ident(resultTemp), l.ident(sourceTemp)}}},
	)
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
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
	if info, ok := l.typeInfoThroughReference(expr.Target.Type); ok && (info.Kind == air.TypeList || info.Kind == air.TypeSlice) {
		elemType = info.Elem
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

	stmts := append([]ast.Stmt{}, target.stmts...)
	targetTemp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{
		Lhs: []ast.Expr{l.ident(targetTemp)}, Tok: token.DEFINE,
		Rhs: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr)},
	})
	stmts = append(stmts, index.stmts...)
	indexTemp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(indexTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{index.expr}})
	stmts = append(stmts, value.stmts...)
	valueExpr := value.expr
	if l.isVoidType(elemType) || isVoidExpr(valueExpr) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	valueTemp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{valueExpr}})
	resultTemp := l.nextTemp()
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{l.ident("false")}})

	valid := &ast.BinaryExpr{
		X:  &ast.BinaryExpr{X: l.ident(indexTemp), Op: token.GEQ, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
		Op: token.LAND,
		Y:  &ast.BinaryExpr{X: l.ident(indexTemp), Op: token.LSS, Y: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident(targetTemp)}}},
	}
	stmts = append(stmts, &ast.IfStmt{
		Cond: valid,
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: l.ident(targetTemp), Index: l.ident(indexTemp)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(valueTemp)}},
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident("true")}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
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
	targetExpr := l.valueThroughReference(expr.Target.Type, target.expr)
	stmts := append(target.stmts, left.stmts...)
	stmts = append(stmts, right.stmts...)
	stmts = append(stmts,
		&ast.AssignStmt{Lhs: []ast.Expr{l.ident(leftName)}, Tok: token.DEFINE, Rhs: []ast.Expr{left.expr}},
		&ast.AssignStmt{Lhs: []ast.Expr{l.ident(rightName)}, Tok: token.DEFINE, Rhs: []ast.Expr{right.expr}},
		&ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: targetExpr, Index: l.ident(leftName)}, &ast.IndexExpr{X: targetExpr, Index: l.ident(rightName)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.IndexExpr{X: targetExpr, Index: l.ident(rightName)}, &ast.IndexExpr{X: targetExpr, Index: l.ident(leftName)}}},
	)
	return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
}

func (l *lowerer) lowerListPrepend(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list prepend expects target and value")
	}
	if expr.Target.Kind != air.ExprLoadLocal && expr.Target.Kind != air.ExprLoadGlobal {
		return loweredExpr{}, fmt.Errorf("list prepend requires an addressable local or global target")
	}
	if !validTypeID(l.program, expr.Target.Type) {
		return loweredExpr{}, fmt.Errorf("invalid list prepend target type")
	}
	listInfo := l.program.Types[expr.Target.Type-1]
	if listInfo.Kind == air.TypeReference && validTypeID(l.program, listInfo.Elem) {
		listInfo = l.program.Types[listInfo.Elem-1]
	}
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
	var target ast.Expr
	if expr.Target.Kind == air.ExprLoadLocal {
		target = l.localValueExpr(fn, expr.Target.Local)
	} else {
		if expr.Target.Global < 0 || int(expr.Target.Global) >= len(l.program.Globals) {
			return loweredExpr{}, fmt.Errorf("list prepend references invalid global %d", expr.Target.Global)
		}
		target = l.globalExpr(l.program.Globals[expr.Target.Global])
	}
	target = l.valueThroughReference(expr.Target.Type, target)
	valueExpr := value.expr
	stmts := append([]ast.Stmt{}, value.stmts...)
	if l.isVoidType(listInfo.Elem) || isVoidExpr(valueExpr) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	assign := &ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("append"), Args: []ast.Expr{&ast.CompositeLit{Type: &ast.ArrayType{Elt: elemType}, Elts: []ast.Expr{valueExpr}}, target}, Ellipsis: 2}}}
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{target}}}, nil
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
	targetExpr := l.valueThroughReference(expr.Target.Type, target.expr)
	lessFunc := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{l.ident("i")}, Type: l.ident("int")},
				{Names: []*ast.Ident{l.ident("j")}, Type: l.ident("int")},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: l.ident("bool")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: cmp.expr, Args: []ast.Expr{
				&ast.IndexExpr{X: targetExpr, Index: l.ident("i")},
				&ast.IndexExpr{X: targetExpr, Index: l.ident("j")},
			}}}},
		}},
	}
	stmts := append(target.stmts, cmp.stmts...)
	stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: l.ident(sortAlias), Sel: l.ident("SliceStable")}, Args: []ast.Expr{targetExpr, lessFunc}}})
	return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
}

func (l *lowerer) lowerListPush(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("list push missing target")
	}
	if expr.Target.Kind != air.ExprLoadLocal && expr.Target.Kind != air.ExprGetField && expr.Target.Kind != air.ExprLoadGlobal {
		return loweredExpr{}, fmt.Errorf("list push requires an addressable local, field, or global target")
	}
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("list push expects one arg")
	}
	var value loweredExpr
	var err error
	if info, ok := l.typeInfoThroughReference(expr.Target.Type); ok && info.Kind == air.TypeList {
		value, err = l.lowerExprWithExpectedType(fn, expr.Args[0], info.Elem)
	} else {
		value, err = l.lowerExpr(fn, expr.Args[0])
	}
	if err != nil {
		return loweredExpr{}, err
	}
	var target ast.Expr
	var targetStmts []ast.Stmt
	if expr.Target.Kind == air.ExprLoadLocal {
		target = l.localValueExpr(fn, expr.Target.Local)
	} else {
		loweredTarget, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		target = loweredTarget.expr
		targetStmts = loweredTarget.stmts
	}
	target = l.valueThroughReference(expr.Target.Type, target)
	valueExpr := value.expr
	stmts := append(append([]ast.Stmt{}, targetStmts...), value.stmts...)
	if info, ok := l.typeInfoThroughReference(expr.Target.Type); ok && info.Kind == air.TypeList && l.isVoidType(info.Elem) {
		stmts = l.appendVoidValueEval(stmts, valueExpr)
		valueExpr = l.voidValueExpr()
	}
	if isVoidExpr(valueExpr) {
		valueExpr = l.voidValueExpr()
	}
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{target},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("append"), Args: []ast.Expr{target, valueExpr}}},
	}
	stmts = append(stmts, assign)
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{target}}}, nil
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
	lookup := &ast.IndexExpr{X: l.valueThroughReference(expr.Target.Type, target.expr), Index: key.expr}
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident("_"), l.ident(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{lookup}})
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(okName)}})
	return loweredExpr{stmts: stmts, expr: l.ident(temp)}, nil
}

func (l *lowerer) lowerAsyncStart(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("async start expects one arg")
	}
	task, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(task.stmts, &ast.GoStmt{Call: &ast.CallExpr{Fun: task.expr}})
	return loweredExpr{stmts: stmts, expr: l.voidValueExpr()}, nil
}

// lowerMakeChannel lowers Chan::new to `make(chan T)` or `make(chan T, capacity)`.
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
	call := &ast.CallExpr{Fun: l.ident("make"), Args: []ast.Expr{chanType, &ast.CallExpr{Fun: &ast.SelectorExpr{X: capacity.expr, Sel: l.ident("Value")}}}}
	return loweredExpr{stmts: capacity.stmts, expr: call}, nil
}

// lowerChannelSend lowers Chan.send/Sender.send to `ch <- value` and yields Void.
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

// lowerChannelRecv lowers Chan.recv/Receiver.recv to `v, ok := <-ch` wrapped into a
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
	someExpr, err := l.maybeSomeExpr(expr.Type, l.ident(valueTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueTemp), l.ident(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{recv}},
		Cond: l.ident(okName),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(temp)}, nil
}

// lowerChannelClose lowers Chan.close/Sender.close to `close(ch)` and yields Void.
func (l *lowerer) lowerChannelClose(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("channel close expects one arg")
	}
	ch, err := l.lowerExpr(fn, expr.Args[0])
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append(ch.stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("close"), Args: []ast.Expr{ch.expr}}})
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
	var resultExpr ast.Expr = l.ident("nil")
	var assignTarget ast.Expr
	if !l.isVoidType(expr.Type) {
		temp := l.nextTemp()
		decls, err := l.declareTemp(expr.Type, temp)
		if err != nil {
			return loweredExpr{}, err
		}
		preStmts = append(preStmts, decls...)
		assignTarget = l.ident(temp)
		resultExpr = l.ident(temp)
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
					Lhs: []ast.Expr{l.ident(valueTemp), l.ident(okTemp)},
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
				someExpr, err := l.maybeSomeExpr(maybeTypeID, l.ident(valueTemp))
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
						Cond: l.ident(okTemp),
						Body: &ast.BlockStmt{List: []ast.Stmt{
							&ast.AssignStmt{Lhs: []ast.Expr{l.ident(bindName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
						}},
					},
					&ast.AssignStmt{Lhs: []ast.Expr{l.ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident(bindName)}},
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
	lookup := ast.Expr(&ast.IndexExpr{X: l.valueThroughReference(expr.Target.Type, target.expr), Index: key.expr})
	stmts := append(target.stmts, key.stmts...)
	stmts = append(stmts, decls...)
	someExpr, err := l.maybeSomeExpr(expr.Type, l.ident(valueTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueTemp), l.ident(okName)}, Tok: token.DEFINE, Rhs: []ast.Expr{lookup}},
		Cond: l.ident(okName),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{someExpr}},
		}},
	})
	return loweredExpr{stmts: stmts, expr: l.ident(temp)}, nil
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
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: l.valueThroughReference(expr.Target.Type, target.expr), Index: keyExpr}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}})
	return loweredExpr{stmts: stmts, expr: l.voidValueExpr()}, nil
}

func (l *lowerer) lowerMapDelete(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return loweredExpr{}, fmt.Errorf("map delete expects target and one arg")
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
	stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("delete"), Args: []ast.Expr{l.valueThroughReference(expr.Target.Type, target.expr), key.expr}}})
	return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
}

func (l *lowerer) lowerMapKeys(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("map keys missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	keys, err := l.mapKeysExpr(expr.Target.Type, target.expr)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: keys}, nil
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
	keys, err := l.mapKeysExpr(expr.Target.Type, target.expr)
	if err != nil {
		return loweredExpr{}, err
	}
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
	keys, err := l.mapKeysExpr(expr.Target.Type, target.expr)
	if err != nil {
		return loweredExpr{}, err
	}
	keyExpr := &ast.IndexExpr{X: keys, Index: index.expr}
	return loweredExpr{stmts: stmts, expr: &ast.IndexExpr{X: l.valueThroughReference(expr.Target.Type, target.expr), Index: keyExpr}}, nil
}

func (l *lowerer) mapKeysExpr(typeID air.TypeID, mapExpr ast.Expr) (ast.Expr, error) {
	info, ok := l.typeInfoThroughReference(typeID)
	if !ok {
		return nil, fmt.Errorf("invalid map type %d", typeID)
	}
	mapExpr = l.valueThroughReference(typeID, mapExpr)
	if info.Kind != air.TypeMap && !(info.Kind == air.TypeForeignType && validTypeID(l.program, info.Key) && validTypeID(l.program, info.Value)) {
		return nil, fmt.Errorf("type %s is not a map", info.Name)
	}
	mapValueType := typeID
	if referent, reference := l.referentType(typeID); reference {
		mapValueType = referent
	}
	mapType, err := l.goType(mapValueType)
	if err != nil {
		return nil, err
	}
	keyType, err := l.goType(info.Key)
	if err != nil {
		return nil, err
	}
	keysType := &ast.ArrayType{Elt: keyType}
	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{l.ident("m")}, Type: mapType}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: keysType}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{l.ident("keys")},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("make"), Args: []ast.Expr{keysType, &ast.BasicLit{Kind: token.INT, Value: "0"}, &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{l.ident("m")}}}}},
				},
				&ast.RangeStmt{
					Key: l.ident("k"),
					Tok: token.DEFINE,
					X:   l.ident("m"),
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
						Lhs: []ast.Expr{l.ident("keys")},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("append"), Args: []ast.Expr{l.ident("keys"), l.ident("k")}}},
					}}},
				},
				&ast.ReturnStmt{Results: []ast.Expr{l.ident("keys")}},
			}},
		},
		Args: []ast.Expr{mapExpr},
	}, nil
}

func mustTypeExpr(l *lowerer, typeID air.TypeID) ast.Expr {
	typ, err := l.goType(typeID)
	if err != nil {
		panic(err)
	}
	return typ
}

func (l *lowerer) lowerTraitReferenceProjection(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Target == nil || !l.isReferenceType(expr.Type) || !validImplID(l.program, expr.Impl) {
		return loweredExpr{}, fmt.Errorf("invalid mutable trait reference projection")
	}
	reference := l.program.Types[expr.Type-1]
	if !l.isTraitObjectType(reference.Elem) {
		return loweredExpr{}, fmt.Errorf("mutable trait projection destination is not a trait reference")
	}
	traitID := l.program.Types[reference.Elem-1].Trait
	if !validTraitID(l.program, traitID) {
		return loweredExpr{}, fmt.Errorf("invalid mutable trait projection trait %d", traitID)
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{target.expr}}}, nil
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
	if l.isReferenceType(expr.Target.Type) {
		targetIsTraitObject = l.isTraitObjectType(l.program.Types[expr.Target.Type-1].Elem)
	}
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
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: l.ident(methodName)}, Args: args}
	if l.abiReturnShapeAvailable(method.Signature.Return) && len(writeback) == 0 {
		return l.packABICallResult(expr.Type, method.Signature.Return, stmts, call)
	}
	return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
}

func (l *lowerer) exprIsMutableReference(fn air.Function, expr air.Expr) bool {
	if l.isReferenceType(expr.Type) {
		return true
	}
	switch expr.Kind {
	case air.ExprLoadLocal:
		return l.localIsPointerParam(fn, expr.Local)
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
	current := target
	current.expr = l.mutableTraitCurrentExpr(trait, target.expr)
	if l.usesNativeTraitInterface(l.traitObjectTypeID(trait.ID)) {
		return l.lowerNativeTraitInterfaceCall(fn, current, expr)
	}
	return l.lowerFallbackTraitDispatchCall(fn, current, expr)
}

func (l *lowerer) lowerFallbackTraitDispatchCall(fn air.Function, target loweredExpr, expr air.Expr) (loweredExpr, error) {
	trait := l.program.Traits[expr.Trait]
	method := trait.Methods[expr.Method]
	args, argStmts, writeback, err := l.lowerCallArgs(fn, expr.Args, method.Signature.Params)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]ast.Stmt{}, target.stmts...)
	stmts = append(stmts, argStmts...)
	dispatch := &ast.TypeAssertExpr{X: target.expr, Type: l.traitOwnedTypeExpr(trait, mutableTraitDispatchTypeName(trait))}
	call := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: dispatch, Sel: l.ident(mutableTraitDispatchMethodName(trait.ID, expr.Method))},
		Args: args,
	}
	if l.abiReturnShapeAvailable(method.Signature.Return) && len(writeback) == 0 {
		return l.packABICallResult(expr.Type, method.Signature.Return, stmts, call)
	}
	return l.finishCallWithWriteback(expr.Type, stmts, call, writeback)
}

func (l *lowerer) lowerTraitObjectCall(fn air.Function, target loweredExpr, expr air.Expr) (loweredExpr, error) {
	if l.traitHasReferenceTypeUse(expr.Trait) {
		return l.lowerFallbackTraitDispatchCall(fn, target, expr)
	}
	isVoid := l.isVoidType(expr.Type)
	stmts := append([]ast.Stmt{}, target.stmts...)
	traitMethod := l.program.Traits[expr.Trait].Methods[expr.Method]

	var resultTemp string
	if !isVoid {
		resultTemp = l.nextTemp()
		resultType, err := l.goType(expr.Type)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(resultTemp)}, Type: resultType}}}})
	}

	loweredArgs := make([]loweredExpr, len(expr.Args))
	for i, arg := range expr.Args {
		var loweredArg loweredExpr
		var err error
		if i < len(traitMethod.Signature.Params) && !l.isReferenceType(traitMethod.Signature.Params[i].Type) {
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
	switchVarExpr := l.ident(switchVar)
	cases := []ast.Stmt{}
	for _, impl := range l.program.Impls {
		if impl.Trait != expr.Trait || expr.Method >= len(impl.Methods) || !validTypeID(l.program, impl.ForType) {
			continue
		}
		methodFn := l.program.Functions[impl.Methods[expr.Method]]
		buildBody := func(receiver ast.Expr) ([]ast.Stmt, error) {
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
						return nil, adaptErr
					}
					body = append(body, setup...)
					writeback = append(writeback, post...)
				}
				args = append(args, argExpr)
			}
			callResult := ast.Expr(l.functionCallExpr(methodFn, args, nil))
			if l.isBuiltinToStringTraitCall(expr, impl.ForType) && len(args) == 1 {
				callResult = l.toStringExpr(impl.ForType, args[0])
			}
			if isVoid {
				body = append(body, &ast.ExprStmt{X: callResult})
			} else {
				body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{callResult}})
			}
			body = append(body, writeback...)
			return body, nil
		}
		concreteType := mustTypeExpr(l, impl.ForType)
		if !l.implRequiresPointerReceiver(impl.ID) {
			body, err := buildBody(switchVarExpr)
			if err != nil {
				return loweredExpr{}, err
			}
			cases = append(cases, &ast.CaseClause{List: []ast.Expr{concreteType}, Body: body})
		}
		pointerReceiver := ast.Expr(&ast.StarExpr{X: switchVarExpr})
		if len(methodFn.Signature.Params) > 0 && l.isReferenceType(methodFn.Signature.Params[0].Type) {
			pointerReceiver = switchVarExpr
		}
		pointerBody, err := buildBody(pointerReceiver)
		if err != nil {
			return loweredExpr{}, err
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.StarExpr{X: concreteType}},
			Body: pointerBody,
		})
	}
	cases = append(cases, &ast.CaseClause{Body: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident("panic"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: "\"unsupported trait object dispatch\""}}}}}})
	stmts = append(stmts, &ast.TypeSwitchStmt{Assign: &ast.AssignStmt{Lhs: []ast.Expr{switchVarExpr}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.TypeAssertExpr{X: target.expr}}}, Body: &ast.BlockStmt{List: cases}})
	if isVoid {
		return loweredExpr{stmts: stmts, expr: l.ident("nil")}, nil
	}
	return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
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
	case air.TypeInt, air.TypeScalar, air.TypeFloat64, air.TypeBool, air.TypeByte, air.TypeRune, air.TypeStr:
		return true
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
		elts = append(elts, &ast.KeyValueExpr{Key: l.ident(l.goFieldName(info, field.Name)), Value: &ast.SelectorExpr{X: expr, Sel: l.ident(exportedFieldName(field.Name))}})
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
	valueExpr := ast.Expr(l.ident(valueTemp))
	if nativeTraitValue {
		valueDeclType = l.ident("any")
		valueExpr, err = l.nativeTraitInterfaceAssertion(resultType.Value, l.ident(valueTemp))
		if err != nil {
			return loweredExpr{}, err
		}
	}
	decls := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(valueTemp)}, Type: valueDeclType}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(errTemp)}, Type: l.ident("error")}}}},
	}
	stmts := append([]ast.Stmt{}, decls...)
	stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{l.ident(valueTemp), l.ident(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
	errExpr, err := l.convertStdlibError(resultType.Error, l.ident(errTemp))
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
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(resultTemp)}, Type: resultTypeExpr}}}},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: l.ident(errTemp), Op: token.EQL, Y: l.ident("nil")},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: l.ident(resultTemp), Sel: l.ident("Value")}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}},
					&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: l.ident(resultTemp), Sel: l.ident("Ok")}}, Tok: token.ASSIGN, Rhs: []ast.Expr{l.ident("true")}},
				}},
				Else: &ast.BlockStmt{List: []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: l.ident(resultTemp), Sel: l.ident("Err")}}, Tok: token.ASSIGN, Rhs: []ast.Expr{errExpr}},
				}},
			},
		)
		return loweredExpr{stmts: stmts, expr: l.ident(resultTemp)}, nil
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Value"), Value: valueExpr},
		&ast.KeyValueExpr{Key: l.ident("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: l.ident("Ok"), Value: &ast.BinaryExpr{X: l.ident(errTemp), Op: token.EQL, Y: l.ident("nil")}},
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
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(errTemp)}, Type: l.ident("error")}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{l.ident(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	errExpr, err := l.convertStdlibError(resultType.Error, l.ident(errTemp))
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: l.ident("Value"), Value: l.voidValueExpr()},
		&ast.KeyValueExpr{Key: l.ident("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: l.ident("Ok"), Value: &ast.BinaryExpr{X: l.ident(errTemp), Op: token.EQL, Y: l.ident("nil")}},
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
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(temp)}, Type: l.ident("any")}}}},
	}
	cases := make([]ast.Stmt, 0, len(info.Members))
	for _, member := range info.Members {
		fieldName := unionMemberFieldName(info, member)
		valueExpr := ast.Expr(&ast.SelectorExpr{X: wrappedExpr, Sel: l.ident(fieldName)})
		if validTypeID(l.program, member.Type) && l.program.Types[member.Type-1].Kind == air.TypeVoid {
			valueExpr = l.ident("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{l.ident(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}}},
		})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: wrappedExpr, Sel: l.ident(unionTagFieldName(info))}, Body: &ast.BlockStmt{List: cases}})
	return loweredExpr{stmts: stmts, expr: l.ident(temp)}, nil
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
		return loweredExpr{expr: &ast.CallExpr{Fun: l.ident("ardListToAnySlice"), Args: []ast.Expr{expr}}}, nil
	}
	valueTemp := l.nextTemp()
	indexTemp := l.nextTemp()
	outTemp := l.nextTemp()
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{l.ident(outTemp)}, Type: &ast.ArrayType{Elt: l.ident("any")}}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{l.ident(outTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.ident("make"), Args: []ast.Expr{&ast.ArrayType{Elt: l.ident("any")}, &ast.CallExpr{Fun: l.ident("len"), Args: []ast.Expr{expr}}}}}},
		&ast.RangeStmt{Key: l.ident(indexTemp), Value: l.ident(valueTemp), Tok: token.DEFINE, X: expr, Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.SwitchStmt{Tag: &ast.SelectorExpr{X: l.ident(valueTemp), Sel: l.ident(unionTagFieldName(elemInfo))}, Body: &ast.BlockStmt{List: unionSliceCaseClauses(l.program, elemInfo, outTemp, indexTemp, valueTemp)}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: l.ident(outTemp)}, nil
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
	return &ast.TypeAssertExpr{X: &ast.CallExpr{Fun: l.ident("any"), Args: []ast.Expr{value}}, Type: traitType}, nil
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
	elem, err := l.goType(info.Elem)
	if err != nil {
		return nil, err
	}
	return elem, nil
}

func (l *lowerer) maybeSomeExpr(maybeTypeID air.TypeID, value ast.Expr) (ast.Expr, error) {
	elemType, err := l.maybeElemTypeExpr(maybeTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("Some"), Index: elemType}, Args: []ast.Expr{value}}, nil
}

func (l *lowerer) maybeNoneExpr(maybeTypeID air.TypeID) (ast.Expr, error) {
	elemType, err := l.maybeElemTypeExpr(maybeTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: l.runtimeQualified("None"), Index: elemType}}, nil
}

func (l *lowerer) maybeIsSomeExpr(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: selectorExpr(expr, "IsSome")}
}

func (l *lowerer) maybeIsNoneExpr(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: selectorExpr(expr, "IsNone")}
}

func (l *lowerer) maybeValueExpr(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: selectorExpr(expr, "Value")}
}

func (l *lowerer) collectInlineClosureFunctions() map[air.FunctionID]bool {
	uses := map[air.FunctionID]int{}
	directRefs := map[air.FunctionID]bool{}
	for _, fn := range l.program.Functions {
		walkBlockExprs(fn.Body, func(expr air.Expr) {
			switch expr.Kind {
			case air.ExprMakeClosure:
				uses[expr.Function]++
			case air.ExprCall, air.ExprFunctionRef:
				directRefs[expr.Function] = true
			}
		})
	}
	inline := map[air.FunctionID]bool{}
	for _, fn := range l.program.Functions {
		if uses[fn.ID] == 0 || directRefs[fn.ID] || !l.canInlineClosureFunction(fn) {
			continue
		}
		inline[fn.ID] = true
	}
	return inline
}

func (l *lowerer) canInlineClosureFunction(fn air.Function) bool {
	return strings.HasPrefix(fn.Name, "anon_func_") && !functionDirectlyReferences(fn.Body, fn.ID)
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
