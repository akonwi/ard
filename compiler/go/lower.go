package gotarget

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
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
	directGoAliases         map[string]string
	reservedGoIdentifiers   map[string]bool
	declaredLocals          map[air.LocalID]bool
	runtimeHelpers          map[string]bool
	jsonParseTypes          map[air.TypeID]bool
	jsonEncodeTypes         map[air.TypeID]bool
	ffiImports              map[string]string
	projectInfo             *checker.ProjectInfo
	directGoResolver        *checker.GoPackagesResolver
	inlineClosures          map[air.FunctionID]bool
	goMethodCollisions      map[string]bool
	emittedGoMethods        map[string]bool
	ffiNativeTraitFallbacks map[air.TraitID]bool
	suppressMain            bool
	includeTests            bool
	useModulePackages       bool
}

func lowerProgram(program *air.Program, options Options) (map[string]*ast.File, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	directGoResolverDir := "."
	if options.ProjectInfo != nil && options.ProjectInfo.RootPath != "" {
		directGoResolverDir = options.ProjectInfo.RootPath
	}
	l := &lowerer{program: program, packageName: defaultPackageName(options.PackageName), runtimeHelpers: map[string]bool{}, jsonParseTypes: map[air.TypeID]bool{}, jsonEncodeTypes: map[air.TypeID]bool{}, ffiImports: collectFFIGoImports(options.ProjectInfo), projectInfo: options.ProjectInfo, directGoResolver: checker.NewGoPackagesResolver(directGoResolverDir), directGoAliases: map[string]string{}, reservedGoIdentifiers: collectReservedGoIdentifiers(program), suppressMain: options.SuppressMain, includeTests: options.IncludeTests}
	l.inlineClosures = l.collectInlineClosureFunctions()
	l.goMethodCollisions = l.collectGoMethodCollisions()
	l.emittedGoMethods = map[string]bool{}
	l.ffiNativeTraitFallbacks = collectFFINativeTraitFallbacks(program)
	files := map[string]*ast.File{}
	rootID, hasRoot := findRootFunction(program)
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
		files[moduleFileName(program, module)] = file
	}
	return files, nil
}

func collectFFIGoImports(projectInfo *checker.ProjectInfo) map[string]string {
	imports := collectGoImportsFromEmbeddedArdModule()
	for alias, path := range collectGoImportsFromPaths(stdlibFFIGoPaths()) {
		imports[alias] = path
	}
	for alias, path := range collectGoImportsFromPaths(projectFFIGoPaths(projectInfo)) {
		imports[alias] = path
	}
	if projectHasFFICompanions(projectInfo) {
		registerProjectFFIImports(imports, projectInfo)
	}
	for alias, path := range collectGoImportsFromPaths(dependencyFFIGoPaths(projectInfo)) {
		imports[alias] = path
	}
	return imports
}

func stdlibFFIGoPaths() []string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil
	}
	stdlibFFIDir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "std_lib", "ffi"))
	matches, err := filepath.Glob(filepath.Join(stdlibFFIDir, "*.go"))
	if err != nil {
		return nil
	}
	return matches
}

func dependencyFFIGoPaths(projectInfo *checker.ProjectInfo) []string {
	if projectInfo == nil {
		return nil
	}
	paths := []string{}
	seenRoots := map[string]bool{}
	addRoot := func(root string) {
		if root == "" || seenRoots[root] {
			return
		}
		seenRoots[root] = true
		matches, err := filepath.Glob(filepath.Join(root, "ffi", "*.go"))
		if err == nil {
			paths = append(paths, matches...)
		}
	}
	for _, dep := range projectInfo.Dependencies {
		addRoot(dependencyRootPath(dep))
	}
	for packageID, pkg := range projectInfo.Packages {
		if packageID == projectInfo.RootPackageID {
			continue
		}
		addRoot(pkg.RootPath)
	}
	return paths
}

func projectFFIGoPaths(projectInfo *checker.ProjectInfo) []string {
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return nil
	}
	paths := []string{filepath.Join(projectInfo.RootPath, "ffi.go")}
	matches, err := filepath.Glob(filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
	if err == nil {
		paths = append(paths, matches...)
	}
	return paths
}

func collectGoImportsFromPaths(paths []string) map[string]string {
	imports := map[string]string{}
	for _, path := range paths {
		if skipFFIImportSource(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		collectGoImportsFromSource(imports, path, data)
	}
	return imports
}

func collectGoImportsFromEmbeddedArdModule() map[string]string {
	imports := map[string]string{}
	for rel, content := range embeddedArdModuleFiles {
		if !strings.HasPrefix(rel, "std_lib/ffi/") || skipFFIImportSource(rel) {
			continue
		}
		collectGoImportsFromSource(imports, rel, []byte(content))
	}
	return imports
}

func skipFFIImportSource(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, ".gen.go") || strings.HasSuffix(base, "_test.go") || base == "generate.go"
}

func collectGoImportsFromSource(imports map[string]string, name string, data []byte) {
	file, err := parser.ParseFile(token.NewFileSet(), name, data, parser.ImportsOnly)
	if err != nil {
		return
	}
	for _, spec := range file.Imports {
		if spec.Path == nil {
			continue
		}
		importPath := strings.Trim(spec.Path.Value, "\"")
		if importPath == "" || importPath == "C" {
			continue
		}
		alias := ""
		if spec.Name != nil {
			if spec.Name.Name == "." || spec.Name.Name == "_" {
				continue
			}
			alias = spec.Name.Name
		} else {
			alias = filepath.Base(importPath)
		}
		imports[alias] = importPath
	}
}

func (l *lowerer) lowerModule(module air.Module) (*ast.File, error) {
	previousModule := l.currentModule
	l.currentModule = module.ID
	defer func() { l.currentModule = previousModule }()
	l.currentImports = map[string]string{}
	l.importErr = nil
	decls := []ast.Decl{}
	rootID, hasRoot := findRootFunction(l.program)
	mainModuleID := air.ModuleID(0)
	if hasRoot {
		mainModuleID = l.program.Functions[rootID].Module
	} else if len(l.program.Modules) > 0 {
		mainModuleID = l.program.Modules[len(l.program.Modules)-1].ID
	}
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
	functionIDs := append([]air.FunctionID(nil), module.Functions...)
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
	if module.ID == mainModuleID {
		if l.suppressMain {
			decls = append(l.runtimePreludeDecls(), decls...)
		} else if !hasRoot {
			decls = append(l.runtimePreludeDecls(), decls...)
			decls = append(decls, &ast.FuncDecl{Name: ast.NewIdent("main"), Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{}})
		} else {
			mainDecl, err := l.lowerMainWrapper(rootID)
			if err != nil {
				return nil, err
			}
			decls = append(decls, mainDecl)
			decls = append(l.runtimePreludeDecls(), decls...)
		}
	}
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
	return &ast.File{Name: ast.NewIdent(l.packageName), Decls: decls}, nil
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

func rewritePreludeSourceImports(source string, aliases map[string]string) string {
	for original, alias := range aliases {
		if alias == "" || alias == original {
			continue
		}
		source = strings.ReplaceAll(source, original+".", alias+".")
	}
	return source
}

func (l *lowerer) registerFFIImportsForGoType(expr ast.Expr) {
	l.registerImportsForGoType(expr, l.ffiImports)
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

func (l *lowerer) validateProjectFFIExternTypeBinding(binding string, expr ast.Expr) error {
	if !projectHasFFICompanions(l.projectInfo) {
		return nil
	}
	if name, ok := unqualifiedExternTypeIdent(expr); ok {
		return fmt.Errorf("project go extern type binding %q must qualify %s with package %s", binding, name, projectFFIPackageAlias(l.projectInfo))
	}
	return nil
}

func unqualifiedExternTypeIdent(expr ast.Expr) (string, bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name, ast.IsExported(node.Name) && !isPredeclaredGoTypeName(node.Name)
	case *ast.StarExpr:
		return unqualifiedExternTypeIdent(node.X)
	case *ast.ArrayType:
		return unqualifiedExternTypeIdent(node.Elt)
	case *ast.MapType:
		if name, ok := unqualifiedExternTypeIdent(node.Key); ok {
			return name, true
		}
		return unqualifiedExternTypeIdent(node.Value)
	case *ast.IndexExpr:
		if name, ok := unqualifiedExternTypeIdent(node.X); ok {
			return name, true
		}
		return unqualifiedExternTypeIdent(node.Index)
	case *ast.IndexListExpr:
		if name, ok := unqualifiedExternTypeIdent(node.X); ok {
			return name, true
		}
		for _, index := range node.Indices {
			if name, ok := unqualifiedExternTypeIdent(index); ok {
				return name, true
			}
		}
		return "", false
	case *ast.ParenExpr:
		return unqualifiedExternTypeIdent(node.X)
	case *ast.SelectorExpr:
		return "", false
	default:
		return "", false
	}
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
	if l.runtimeHelpers["fiber"] {
		parts = append(parts, `
	type ardFiberState[T any] struct {
		ch    chan T
		value T
		done  bool
	}

	type ardFiber[T any] struct {
		state *ardFiberState[T]
	}

	func ardSpawnFiber[T any](do func() T) ardFiber[T] {
		state := &ardFiberState[T]{ch: make(chan T, 1)}
		go func() {
			state.ch <- do()
		}()
		return ardFiber[T]{state: state}
	}

	func ardJoinFiber[T any](fiber ardFiber[T]) {
		if !fiber.state.done {
			fiber.state.value = <-fiber.state.ch
			fiber.state.done = true
		}
	}

	func ardGetFiber[T any](fiber ardFiber[T]) T {
		ardJoinFiber(fiber)
		return fiber.state.value
	}
`)
	}
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
	if l.runtimeHelpers["direct_go_signed_int_range"] {
		parts = append(parts, `
	func ardDirectGoCheckSignedIntRange(value int, min int64, max int64, target string) int {
		v := int64(value)
		if v < min || v > max {
			panic("Ard direct Go FFI: int value out of range for " + target)
		}
		return value
	}
`)
	}
	if l.runtimeHelpers["direct_go_uint_int_range"] {
		parts = append(parts, `
	func ardDirectGoCheckUintIntRange(value int, max uint64, target string) int {
		if value < 0 || uint64(value) > max {
			panic("Ard direct Go FFI: int value out of range for " + target)
		}
		return value
	}
`)
	}
	if l.runtimeHelpers["direct_go_nonnegative_int"] {
		parts = append(parts, `
	func ardDirectGoCheckNonNegativeInt(value int, target string) int {
		if value < 0 {
			panic("Ard direct Go FFI: negative int value out of range for " + target)
		}
		return value
	}
`)
	}
	if l.runtimeHelpers["direct_go_signed_to_int"] {
		parts = append(parts, `
	func ardDirectGoIntFromSigned(value int64, target string) int {
		max := int64(^uint(0) >> 1)
		min := -max - 1
		if value < min || value > max {
			panic("Ard direct Go FFI: signed integer value out of range for Int from " + target)
		}
		return int(value)
	}
`)
	}
	if l.runtimeHelpers["direct_go_unsigned_to_int"] {
		parts = append(parts, `
	func ardDirectGoIntFromUnsigned(value uint64, target string) int {
		max := uint64(^uint(0) >> 1)
		if value > max {
			panic("Ard direct Go FFI: unsigned integer value out of range for Int from " + target)
		}
		return int(value)
	}
`)
	}
	if l.runtimeHelpers["direct_go_float32_range"] {
		mathAlias := l.registerImport("ardmath", "math")
		parts = append(parts, fmt.Sprintf(`
	func ardDirectGoCheckFloat32Range(value float64, target string) float64 {
		if value > %s.MaxFloat32 || value < -%s.MaxFloat32 {
			panic("Ard direct Go FFI: float value out of range for " + target)
		}
		return value
	}
`, mathAlias, mathAlias))
	}
	if l.runtimeHelpers["direct_go_valid_rune"] {
		utf8Alias := l.registerImport("ardutf8", "unicode/utf8")
		parts = append(parts, fmt.Sprintf(`
	func ardDirectGoCheckRune(value rune) rune {
		if !%s.ValidRune(value) {
			panic("Ard direct Go FFI: Go returned invalid Rune")
		}
		return value
	}
`, utf8Alias))
	}
	if l.runtimeHelpers["json_parse"] {
		aliases := map[string]string{
			"bytes":      l.registerImport("bytes", "bytes"),
			"fmt":        l.registerImport("fmt", "fmt"),
			"json":       l.registerImport("json", "encoding/json/v2"),
			"jsontext":   l.registerImport("jsontext", "encoding/json/jsontext"),
			"ardruntime": l.registerImport("ardruntime", "github.com/akonwi/ard/runtime"),
			"strconv":    l.registerImport("strconv", "strconv"),
		}
		parts = append(parts, rewritePreludeSourceImports(l.jsonParsePreludeSource(), aliases))
	}
	if l.runtimeHelpers["json_encode"] {
		aliases := map[string]string{
			"bytes":      l.registerImport("bytes", "bytes"),
			"fmt":        l.registerImport("fmt", "fmt"),
			"json":       l.registerImport("json", "encoding/json/v2"),
			"jsontext":   l.registerImport("jsontext", "encoding/json/jsontext"),
			"ardruntime": l.registerImport("ardruntime", "github.com/akonwi/ard/runtime"),
		}
		parts = append(parts, rewritePreludeSourceImports(l.jsonEncodePreludeSource(), aliases))
	}
	src := strings.Join(parts, "\n")
	file, err := parser.ParseFile(token.NewFileSet(), "prelude.go", src, 0)
	if err != nil {
		panic(err)
	}
	return file.Decls
}

func (l *lowerer) lowerTypeDecls(typ air.TypeInfo) ([]ast.Decl, error) {
	switch typ.Kind {
	case air.TypeStruct:
		if l.isStdlibFFIBackedType(typ) {
			decl := &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Assign: token.Pos(1), Type: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", typ.Name)},
			}}
			return []ast.Decl{decl}, nil
		}
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
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(l.goFieldName(typ, field.Name))}, Type: fieldType})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeUnion:
		fields := []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("tag")}, Type: ast.NewIdent("uint32")}}
		for _, member := range typ.Members {
			memberType, err := l.goType(member.Type)
			if err != nil {
				return nil, err
			}
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ast.NewIdent(unionMemberFieldName(member))}, Type: memberType})
		}
		return []ast.Decl{&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: &ast.StructType{Fields: &ast.FieldList{List: fields}}}}}}, nil
	case air.TypeTraitObject:
		return l.lowerTraitObjectDecls(typ)
	case air.TypeEnum:
		typeSpec := &ast.TypeSpec{Name: ast.NewIdent(typeName(l.program, typ)), Type: ast.NewIdent("int")}
		directGoEnum := false
		if strings.TrimSpace(typ.ExternBinding) != "" {
			if typeExpr, ok, err := l.directGoExternTypeExpr(typ.ExternBinding); err != nil || ok {
				if err != nil {
					return nil, err
				}
				directGoEnum = true
				typeSpec.Assign = token.Pos(1)
				typeSpec.Type = typeExpr
			}
		} else if l.isStdlibFFIBackedType(typ) {
			typeSpec.Assign = token.Pos(1)
			typeSpec.Type = l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", typ.Name)
		}
		specs := []ast.Spec{typeSpec}
		for _, variant := range typ.Variants {
			value := ast.Expr(&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", variant.Discriminant)})
			if directGoEnum {
				constant, ok, err := l.directGoEnumConstantExpr(typ.ExternBinding, variant.Name)
				if err != nil {
					return nil, err
				}
				if ok {
					value = constant
				}
			}
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
	if !l.traitInterfaceAvailable(traitID) || l.ffiNativeTraitFallbacks[traitID] || l.traitHasMutableTraitUse(traitID) {
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

func (l *lowerer) namedTypeExpr(info air.TypeInfo) ast.Expr {
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

func (l *lowerer) functionExpr(fn air.Function) ast.Expr {
	name := functionName(l.program, fn)
	if !l.useModulePackages || fn.Module == l.currentModule {
		return ast.NewIdent(name)
	}
	return l.moduleQualified(fn.Module, name)
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

func (l *lowerer) lowerMainWrapper(root air.FunctionID) (ast.Decl, error) {
	fn := l.program.Functions[root]
	call := &ast.CallExpr{Fun: l.functionExpr(fn)}
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
			Names: []*ast.Ident{ast.NewIdent(localName(fn, capture.Local))},
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
			Names: []*ast.Ident{ast.NewIdent(localName(fn, air.LocalID(i)))},
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
	funcType := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if !l.isVoidType(returnTypeID) {
		returnType, err := l.goType(returnTypeID)
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
	receiverType, err := l.goType(receiverTypeID)
	if err != nil {
		return nil, false, err
	}
	if fn.Signature.Params[0].Mutable {
		receiverType = &ast.StarExpr{X: receiverType}
	}

	params := make([]*ast.Field, 0, len(fn.Signature.Params)-1)
	callArgs := []ast.Expr{ast.NewIdent(localName(fn, 0))}
	for i, param := range fn.Signature.Params[1:] {
		paramType, err := l.goParamType(param)
		if err != nil {
			return nil, false, err
		}
		localID := air.LocalID(i + 1)
		name := localName(fn, localID)
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: paramType})
		callArgs = append(callArgs, ast.NewIdent(name))
	}

	call := &ast.CallExpr{Fun: l.functionExpr(fn), Args: callArgs}
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
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(localName(fn, 0))}, Type: receiverType}}},
		Name: ast.NewIdent(methodName),
		Type: funcType,
		Body: &ast.BlockStmt{List: body},
	}, true, nil
}

func collectFFINativeTraitFallbacks(program *air.Program) map[air.TraitID]bool {
	// Project/dependency FFI companions historically see Ard trait objects as any.
	// Keep that ABI for container-shaped signatures we do not adapt at the
	// boundary; top-level return Trait, Trait?, and Trait!E are adapted below.
	fallbacks := map[air.TraitID]bool{}
	if program == nil {
		return fallbacks
	}
	var scan func(air.TypeID, bool, int, bool)
	scan = func(typeID air.TypeID, unsupportedContainer bool, wrapperDepth int, allowDirectWrapper bool) {
		if !validTypeID(program, typeID) {
			return
		}
		info := program.Types[typeID-1]
		switch info.Kind {
		case air.TypeTraitObject:
			if validTraitID(program, info.Trait) && (unsupportedContainer || wrapperDepth > 1 || wrapperDepth == 1 && !allowDirectWrapper) {
				fallbacks[info.Trait] = true
			}
		case air.TypeList:
			scan(info.Elem, true, wrapperDepth, allowDirectWrapper)
		case air.TypeMap:
			scan(info.Key, true, wrapperDepth, allowDirectWrapper)
			scan(info.Value, true, wrapperDepth, allowDirectWrapper)
		case air.TypeFunction:
			for _, param := range info.Params {
				scan(param, true, wrapperDepth, allowDirectWrapper)
			}
			scan(info.Return, true, wrapperDepth, allowDirectWrapper)
		case air.TypeMaybe:
			scan(info.Elem, unsupportedContainer, wrapperDepth+1, allowDirectWrapper)
		case air.TypeResult:
			scan(info.Value, unsupportedContainer, wrapperDepth+1, allowDirectWrapper)
			scan(info.Error, unsupportedContainer, wrapperDepth+1, allowDirectWrapper)
		}
	}
	for _, ext := range program.Externs {
		for _, param := range ext.Signature.Params {
			scan(param.Type, false, 0, false)
		}
		scan(ext.Signature.Return, false, 0, true)
	}
	return fallbacks
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
	if l.isStdlibFFIBackedType(info) || strings.TrimSpace(info.ExternBinding) != "" {
		return false
	}
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
		if methodName == "tag" {
			return true
		}
		for _, member := range info.Members {
			if unionMemberFieldName(member) == methodName {
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
	name := sanitizeName(raw)
	if name == "" || name == "_" {
		return "", false
	}
	if token.Lookup(name).IsKeyword() {
		name += "_"
	}
	if !token.IsIdentifier(name) {
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
		name := localName(fn, stmt.Local)
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
			name := localName(fn, stmt.Local)
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
			out = append(out, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(localName(fn, stmt.Local)), Sel: ast.NewIdent(l.mutableTraitAssignFieldNameForType(localType))}, Args: []ast.Expr{assignValue}}})
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
	case air.StmtSetDirectGoField:
		if stmt.Target == nil {
			return nil, fmt.Errorf("direct Go field set statement missing target")
		}
		if stmt.Value == nil {
			return nil, fmt.Errorf("direct Go field set statement missing value")
		}
		if strings.TrimSpace(stmt.FieldName) == "" {
			return nil, fmt.Errorf("direct Go field set statement missing field name")
		}
		target, err := l.lowerExpr(fn, *stmt.Target)
		if err != nil {
			return nil, err
		}
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return nil, err
		}
		valueExpr, err := l.coerceDirectGoArg(stmt.Value.Type, value.expr, stmt.DirectGoFieldType, directGoExternBinding{})
		if err != nil {
			return nil, err
		}
		out := append([]ast.Stmt{}, target.stmts...)
		out = append(out, value.stmts...)
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent(stmt.FieldName)}},
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
	case air.ExprCallExtern:
		return l.lowerExternCall(fn, expr)
	case air.ExprDirectGoPackageValue:
		return l.lowerDirectGoPackageValue(expr.Str, expr.Type)
	case air.ExprDirectGoFieldAccess:
		return l.lowerDirectGoFieldAccess(fn, expr)
	case air.ExprDirectGoStructLiteral:
		return l.lowerDirectGoStructLiteral(fn, expr)
	case air.ExprSpawnFiber:
		return l.lowerSpawnFiber(fn, expr)
	case air.ExprFiberGet:
		return l.lowerFiberGet(fn, expr)
	case air.ExprFiberJoin:
		return l.lowerFiberJoin(fn, expr)
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
	case air.ExprToDynamic:
		if expr.Target == nil {
			return loweredExpr{}, fmt.Errorf("to dynamic missing target")
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
		if l.mapUsesStructuralKeys(expr.Target.Type) {
			return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Len")}}}, nil
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
	case air.ExprCall:
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid function id %d", expr.Function)
		}
		target := l.program.Functions[expr.Function]
		args, stmts, writeback, err := l.lowerCallArgs(fn, expr.Args, target.Signature.Params)
		if err != nil {
			return loweredExpr{}, err
		}
		call := &ast.CallExpr{Fun: l.functionExpr(target), Args: args}
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
		air.ExprTryResult, air.ExprTryMaybe:
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
	if a.Kind != b.Kind || a.Type != b.Type || a.Field != b.Field || a.Local != b.Local || a.Function != b.Function || a.Extern != b.Extern {
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
		right.expr = castGoExprToType(right.expr, typeName(l.program, leftInfo))
	}
	if leftInfo.Kind == air.TypeInt && rightInfo.Kind == air.TypeEnum {
		left.expr = castGoExprToType(left.expr, typeName(l.program, rightInfo))
	}
}

func castGoExprToType(expr ast.Expr, typ string) ast.Expr {
	return &ast.CallExpr{Fun: ast.NewIdent(typ), Args: []ast.Expr{expr}}
}

func (l *lowerer) typeInfo(id air.TypeID) (air.TypeInfo, bool) {
	if id <= 0 || int(id) > len(l.program.Types) {
		return air.TypeInfo{}, false
	}
	return l.program.Types[id-1], true
}

func (l *lowerer) voidTypeExpr() ast.Expr {
	return l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "Void")
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
	case air.TypeDynamic, air.TypeExtern, air.TypeFunction, air.TypeTraitObject:
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

func (l *lowerer) typeContainsMaybe(typeID air.TypeID, seen map[air.TypeID]bool) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	if seen[typeID] {
		return false
	}
	seen[typeID] = true
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeMaybe:
		return true
	case air.TypeList, air.TypeFiber:
		return l.typeContainsMaybe(info.Elem, seen)
	case air.TypeMap:
		return l.typeContainsMaybe(info.Key, seen) || l.typeContainsMaybe(info.Value, seen)
	case air.TypeStruct:
		for _, field := range info.Fields {
			if l.typeContainsMaybe(field.Type, seen) {
				return true
			}
		}
	case air.TypeResult:
		return l.typeContainsMaybe(info.Value, seen) || l.typeContainsMaybe(info.Error, seen)
	case air.TypeUnion:
		for _, member := range info.Members {
			if l.typeContainsMaybe(member.Type, seen) {
				return true
			}
		}
	case air.TypeFunction:
		for _, param := range info.Params {
			if l.typeContainsMaybe(param, seen) {
				return true
			}
		}
		return l.typeContainsMaybe(info.Return, seen)
	}
	return false
}

func (l *lowerer) mapUsesStructuralKeys(mapTypeID air.TypeID) bool {
	if !validTypeID(l.program, mapTypeID) {
		return false
	}
	info := l.program.Types[mapTypeID-1]
	return info.Kind == air.TypeMap && l.typeContainsMaybe(info.Key, map[air.TypeID]bool{})
}

func (l *lowerer) structuralMapTypes(mapTypeID air.TypeID) (ast.Expr, ast.Expr, error) {
	if !validTypeID(l.program, mapTypeID) {
		return nil, nil, fmt.Errorf("invalid map type id %d", mapTypeID)
	}
	info := l.program.Types[mapTypeID-1]
	if info.Kind != air.TypeMap {
		return nil, nil, fmt.Errorf("type %s is not a map", info.Name)
	}
	keyType, err := l.goType(info.Key)
	if err != nil {
		return nil, nil, err
	}
	valueType, err := l.goType(info.Value)
	if err != nil {
		return nil, nil, err
	}
	return keyType, valueType, nil
}

func (l *lowerer) structuralMapEntryTypeExpr(mapTypeID air.TypeID) (ast.Expr, error) {
	keyType, valueType, err := l.structuralMapTypes(mapTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.IndexListExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "StructuralMapEntry"), Indices: []ast.Expr{keyType, valueType}}, nil
}

func (l *lowerer) structuralMapWithEntriesExpr(mapTypeID air.TypeID, entries []ast.Expr) (ast.Expr, error) {
	keyType, valueType, err := l.structuralMapTypes(mapTypeID)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.IndexListExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "NewStructuralMapWithEntries"), Indices: []ast.Expr{keyType, valueType}}, Args: entries}, nil
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

func (l *lowerer) isStdlibFFIBackedType(info air.TypeInfo) bool {
	if info.ID == 0 {
		return false
	}
	if info.Kind != air.TypeStruct && info.Kind != air.TypeEnum {
		return false
	}
	path := l.modulePathForType(info.ID)
	return path == "ard/http" && (info.Name == "Method" || info.Name == "Request" || info.Name == "Response")
}

func (l *lowerer) isChannelExternType(info air.TypeInfo) bool {
	if info.Kind != air.TypeExtern || info.Elem == air.NoType {
		return false
	}
	path := l.modulePathForType(info.ID)
	return path == "ard/async/channel" && (info.Name == "Chan" || strings.HasPrefix(info.Name, "Chan<"))
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
	case air.TypeFiber:
		l.markRuntimeHelper("fiber")
		elem, err := l.goType(info.Elem)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: ast.NewIdent("ardFiber"), Index: elem}, nil
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
	case air.TypeMap:
		key, err := l.goType(info.Key)
		if err != nil {
			return nil, err
		}
		value, err := l.goType(info.Value)
		if err != nil {
			return nil, err
		}
		if l.typeContainsMaybe(info.Key, map[air.TypeID]bool{}) {
			return &ast.IndexListExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "StructuralMap"), Indices: []ast.Expr{key, value}}, nil
		}
		return &ast.MapType{Key: key, Value: value}, nil
	case air.TypeStruct, air.TypeEnum:
		if l.isStdlibFFIBackedType(info) {
			return l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", info.Name), nil
		}
		return l.namedTypeExpr(info), nil
	case air.TypeUnion:
		return l.namedTypeExpr(info), nil
	case air.TypeExtern:
		if l.isChannelExternType(info) {
			elem, err := l.goType(info.Elem)
			if err != nil {
				return nil, err
			}
			return &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: elem}, nil
		}
		if strings.TrimSpace(info.ExternBinding) != "" {
			if typ, ok, err := l.directGoExternTypeExpr(info.ExternBinding); err != nil || ok {
				return typ, err
			}
			if alias, ok := dependencyAliasForModulePath(info.ModulePath, l.projectInfo); ok {
				return l.dependencyFFITypeExpr(alias, info.ExternBinding)
			}
			typ, err := parser.ParseExpr(info.ExternBinding)
			if err != nil {
				return nil, fmt.Errorf("invalid go extern type binding %q for %s: %w", info.ExternBinding, info.Name, err)
			}
			if err := l.validateProjectFFIExternTypeBinding(info.ExternBinding, typ); err != nil {
				return nil, err
			}
			l.registerFFIImportsForGoType(typ)
			return typ, nil
		}
		return ast.NewIdent("any"), nil
	case air.TypeDynamic:
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
		ref, err := l.mutableTraitDynamicForwarderExpr(place, param.Type)
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
			return ast.NewIdent(localName(fn, arg.Local)), nil, true, nil
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

func (l *lowerer) mutableTraitDynamicForwarderExpr(place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
	if !l.isTraitObjectType(traitTypeID) {
		return nil, fmt.Errorf("type %d is not a trait object", traitTypeID)
	}
	traitID := l.program.Types[traitTypeID-1].Trait
	if !validTraitID(l.program, traitID) {
		return nil, fmt.Errorf("invalid trait id %d", traitID)
	}
	trait := l.program.Traits[traitID]
	assignFunc, err := l.mutableTraitDynamicAssignFuncLit(place, traitTypeID)
	if err != nil {
		return nil, err
	}
	elts := []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitLoadFieldName(trait)), Value: mutableTraitLoadFuncLit(place)},
		&ast.KeyValueExpr{Key: ast.NewIdent(mutableTraitAssignFieldName(trait)), Value: assignFunc},
	}
	for i, method := range trait.Methods {
		fieldValue, err := l.mutableTraitDynamicForwarderMethodExpr(trait, i, method, place, traitTypeID)
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

func (l *lowerer) mutableTraitDynamicForwarderMethodExpr(trait air.Trait, methodIndex int, traitMethod air.TraitMethod, place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
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

func (l *lowerer) mutableTraitDynamicAssignFuncLit(place ast.Expr, traitTypeID air.TypeID) (ast.Expr, error) {
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
		return ast.NewIdent(localName(fn, arg.Local))
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
	name := ast.Expr(ast.NewIdent(localName(fn, local)))
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
			if chosen != alias {
				l.updateDirectGoAliasReservation(alias, chosen, importPath)
			}
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
		l.reservedGoIdentifiers = collectReservedGoIdentifiers(l.program)
	}
	if l.reservedGoIdentifiers[alias] && !l.aliasReservedForImport(alias, importPath) {
		return false
	}
	return !l.importAliasCollidesWithTopLevel(alias)
}

func (l *lowerer) updateDirectGoAliasReservation(oldAlias string, newAlias string, importPath string) {
	updated := false
	for key, reservedAlias := range l.directGoAliases {
		if reservedAlias == oldAlias && strings.HasPrefix(key, importPath+"\x00") {
			l.directGoAliases[key] = newAlias
			updated = true
		}
	}
	if updated && l.reservedGoIdentifiers != nil {
		delete(l.reservedGoIdentifiers, oldAlias)
	}
}

func (l *lowerer) aliasReservedForImport(alias string, importPath string) bool {
	for key, reservedAlias := range l.directGoAliases {
		if reservedAlias == alias && strings.HasPrefix(key, importPath+"\x00") {
			return true
		}
	}
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
	for _, functionID := range l.program.Modules[moduleID].Functions {
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
			fieldName = unionMemberFieldName(member)
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
	return loweredExpr{stmts: target.stmts, expr: &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, unionType)), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("tag"), Value: &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", expr.Tag)}},
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
				fieldName = unionMemberFieldName(member)
				break
			}
		}
		if fieldName == "" {
			return loweredExpr{}, fmt.Errorf("invalid union case tag %d", unionCase.Tag)
		}
		localName := localName(fn, unionCase.Local)
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
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("tag")}, Body: &ast.BlockStmt{List: cases}})
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
	okName := localName(fn, expr.OkLocal)
	errName := localName(fn, expr.ErrLocal)
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
		errName := localName(fn, expr.CatchLocal)
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
	someName := localName(fn, expr.SomeLocal)
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

func (l *lowerer) lowerSpawnFiber(fn air.Function, expr air.Expr) (loweredExpr, error) {
	l.markRuntimeHelper("fiber")
	var targetExpr ast.Expr
	stmts := []ast.Stmt{}
	if expr.Target != nil {
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, target.stmts...)
		targetExpr = target.expr
		if validTypeID(l.program, expr.Type) {
			fiberType := l.program.Types[expr.Type-1]
			if validTypeID(l.program, fiberType.Elem) && l.program.Types[fiberType.Elem-1].Kind == air.TypeVoid {
				targetExpr = &ast.FuncLit{
					Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: l.voidTypeExpr()}}}},
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ExprStmt{X: &ast.CallExpr{Fun: target.expr}},
						&ast.ReturnStmt{Results: []ast.Expr{l.voidValueExpr()}},
					}},
				}
			}
		}
	} else {
		if !validFunctionID(l.program, expr.Function) {
			return loweredExpr{}, fmt.Errorf("invalid fiber function %d", expr.Function)
		}
		targetFn := l.program.Functions[expr.Function]
		if l.isVoidType(targetFn.Signature.Return) {
			targetExpr = &ast.FuncLit{
				Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: l.voidTypeExpr()}}}},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ExprStmt{X: &ast.CallExpr{Fun: l.functionExpr(targetFn)}},
					&ast.ReturnStmt{Results: []ast.Expr{l.voidValueExpr()}},
				}},
			}
		} else {
			targetExpr = &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: mustTypeExpr(l, targetFn.Signature.Return)}}}}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: l.functionExpr(targetFn)}}}}}}
		}
	}
	return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: &ast.IndexExpr{X: ast.NewIdent("ardSpawnFiber"), Index: mustTypeExpr(l, l.program.Types[expr.Type-1].Elem)}, Args: []ast.Expr{targetExpr}}}, nil
}

func (l *lowerer) lowerFiberGet(fn air.Function, expr air.Expr) (loweredExpr, error) {
	l.markRuntimeHelper("fiber")
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("fiber get missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.IndexExpr{X: ast.NewIdent("ardGetFiber"), Index: mustTypeExpr(l, expr.Type)}, Args: []ast.Expr{target.expr}}}, nil
}

func (l *lowerer) lowerFiberJoin(fn air.Function, expr air.Expr) (loweredExpr, error) {
	l.markRuntimeHelper("fiber")
	if expr.Target == nil {
		return loweredExpr{}, fmt.Errorf("fiber join missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return loweredExpr{}, err
	}
	fiberType := l.program.Types[expr.Target.Type-1]
	return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.IndexExpr{X: ast.NewIdent("ardJoinFiber"), Index: mustTypeExpr(l, fiberType.Elem)}, Args: []ast.Expr{target.expr}}}, nil
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
		argExpr := ast.Expr(ast.NewIdent(localName(fn, local)))
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
		name := localName(closureFn, air.LocalID(i))
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(name)}, Type: paramType})
		callArgs = append(callArgs, ast.NewIdent(name))
	}
	bodyStmts := []ast.Stmt{}
	call := &ast.CallExpr{Fun: l.functionExpr(closureFn), Args: callArgs}
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
		outerName := localName(parent, expr.CaptureLocals[i])
		capture.Name = outerName
		inlineFn.Locals[capture.Local].Name = outerName
		// Inline closures directly close over the outer Go local. Do not treat
		// captures as pointer parameters; mutable argument lowering can still take
		// the address of the outer local when a callee requires it.
		inlineFn.Locals[capture.Local].Mutable = false
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
		name := localName(inlineFn, air.LocalID(i))
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
	if l.mapUsesStructuralKeys(expr.Type) {
		return l.lowerMakeStructuralMap(fn, expr, keyType, valueType)
	}
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

func (l *lowerer) lowerMakeStructuralMap(fn air.Function, expr air.Expr, keyType air.TypeID, valueType air.TypeID) (loweredExpr, error) {
	entryType, err := l.structuralMapEntryTypeExpr(expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	entries := make([]ast.Expr, 0, len(expr.Entries))
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
		entries = append(entries, &ast.CompositeLit{Type: entryType, Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ast.NewIdent("Key"), Value: keyExpr},
			&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: valueExpr},
		}})
	}
	mapExpr, err := l.structuralMapWithEntriesExpr(expr.Type, entries)
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: stmts, expr: mapExpr}, nil
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		return loweredExpr{stmts: append(target.stmts, key.stmts...), expr: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Has")}, Args: []ast.Expr{key.expr}}}, nil
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		lookup = &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Get")}, Args: []ast.Expr{key.expr}}
	}
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Set")}, Args: []ast.Expr{keyExpr, valueExpr}}})
	} else {
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{&ast.IndexExpr{X: target.expr, Index: keyExpr}}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}})
	}
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Drop")}, Args: []ast.Expr{key.expr}}})
	} else {
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent("delete"), Args: []ast.Expr{target.expr, key.expr}}})
	}
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		return loweredExpr{stmts: target.stmts, expr: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("Keys")}}}, nil
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("KeyAt")}, Args: []ast.Expr{index.expr}}}, nil
	}
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
	if l.mapUsesStructuralKeys(expr.Target.Type) {
		return loweredExpr{stmts: stmts, expr: &ast.CallExpr{Fun: &ast.SelectorExpr{X: target.expr, Sel: ast.NewIdent("ValueAt")}, Args: []ast.Expr{index.expr}}}, nil
	}
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
	case air.TypeDynamic, air.TypeExtern:
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
		call := &ast.CallExpr{Fun: l.functionExpr(methodFn), Args: args}
		if isVoid {
			body = append(body, &ast.ExprStmt{X: call})
		} else {
			body = append(body, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}})
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
	if l.isStdlibFFIBackedType(typ) {
		return exportedFieldName(fieldName)
	}
	if strings.HasPrefix(typ.ModulePath, "ard/") {
		name := sanitizeName(fieldName)
		if name == "" {
			return "field"
		}
		if token.Lookup(name).IsKeyword() {
			return name + "_"
		}
		return name
	}
	return naturalGoIdentifier(fieldName, !typ.Private)
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
		elts = append(elts, &ast.KeyValueExpr{Key: ast.NewIdent(field.Name), Value: &ast.SelectorExpr{X: expr, Sel: ast.NewIdent(exportedFieldName(field.Name))}})
	}
	return &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, info)), Elts: elts}, nil
}

func (l *lowerer) wrapMaybeNativeTraitExternCall(maybeTypeID air.TypeID, call ast.Expr, stmts []ast.Stmt) (loweredExpr, bool, error) {
	if !validTypeID(l.program, maybeTypeID) {
		return loweredExpr{}, false, fmt.Errorf("invalid maybe type id %d", maybeTypeID)
	}
	maybeType := l.program.Types[maybeTypeID-1]
	if maybeType.Kind != air.TypeMaybe || !l.usesNativeTraitInterface(maybeType.Elem) {
		return loweredExpr{}, false, nil
	}
	maybeGoType, err := l.goType(maybeTypeID)
	if err != nil {
		return loweredExpr{}, false, err
	}
	elemGoType, err := l.goType(maybeType.Elem)
	if err != nil {
		return loweredExpr{}, false, err
	}
	rawTemp := l.nextTemp()
	resultTemp := l.nextTemp()
	coercedValue, err := l.nativeTraitInterfaceAssertion(maybeType.Elem, &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(rawTemp), Sel: ast.NewIdent("Value")}})
	if err != nil {
		return loweredExpr{}, false, err
	}
	stmts = append(stmts,
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(rawTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: maybeGoType}}}},
		&ast.IfStmt{
			Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(rawTemp), Sel: ast.NewIdent("IsSome")}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "Some"), Index: elemGoType}, Args: []ast.Expr{coercedValue}}}}}},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.IndexExpr{X: l.qualified("ardruntime", "github.com/akonwi/ard/runtime", "None"), Index: elemGoType}}}}}},
		},
	)
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(resultTemp)}, true, nil
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
		fieldName := unionMemberFieldName(member)
		valueExpr := ast.Expr(&ast.SelectorExpr{X: wrappedExpr, Sel: ast.NewIdent(fieldName)})
		if validTypeID(l.program, member.Type) && l.program.Types[member.Type-1].Kind == air.TypeVoid {
			valueExpr = ast.NewIdent("nil")
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", member.Tag)}},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(temp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{valueExpr}}},
		})
	}
	stmts = append(stmts, &ast.SwitchStmt{Tag: &ast.SelectorExpr{X: wrappedExpr, Sel: ast.NewIdent("tag")}, Body: &ast.BlockStmt{List: cases}})
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
			&ast.SwitchStmt{Tag: &ast.SelectorExpr{X: ast.NewIdent(valueTemp), Sel: ast.NewIdent("tag")}, Body: &ast.BlockStmt{List: unionSliceCaseClauses(l.program, elemInfo, outTemp, indexTemp, valueTemp)}},
		}}},
	}
	return loweredExpr{stmts: stmts, expr: ast.NewIdent(outTemp)}, nil
}

func unionSliceCaseClauses(program *air.Program, unionInfo air.TypeInfo, outTemp string, indexTemp string, valueTemp string) []ast.Stmt {
	cases := make([]ast.Stmt, 0, len(unionInfo.Members))
	for _, member := range unionInfo.Members {
		valueExpr := ast.Expr(&ast.SelectorExpr{X: ast.NewIdent(valueTemp), Sel: ast.NewIdent(unionMemberFieldName(member))})
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

func (l *lowerer) wrapStdlibResultCall(resultTypeID air.TypeID, call ast.Expr) (loweredExpr, error) {
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
	resultTemp := l.nextTemp()
	stdlibResultType := &ast.IndexListExpr{X: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Result"), Indices: []ast.Expr{valueType, l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "Error")}}
	stmts := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(resultTemp)}, Type: stdlibResultType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(resultTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
	}
	errExpr, err := l.convertStdlibError(resultType.Error, &ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Err")})
	if err != nil {
		return loweredExpr{}, err
	}
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, resultTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: &ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Value")}},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: errExpr},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.SelectorExpr{X: ast.NewIdent(resultTemp), Sel: ast.NewIdent("Ok")}},
	}}
	return loweredExpr{stmts: stmts, expr: resultExpr}, nil
}

func (l *lowerer) lowerExternCall(fn air.Function, expr air.Expr) (loweredExpr, error) {
	if expr.Extern < 0 || int(expr.Extern) >= len(l.program.Externs) {
		return loweredExpr{}, fmt.Errorf("invalid extern id %d", expr.Extern)
	}
	ext := l.program.Externs[expr.Extern]
	args := make([]ast.Expr, 0, len(expr.Args))
	stmts := []ast.Stmt{}
	for i, arg := range expr.Args {
		var loweredArg loweredExpr
		var err error
		if i < len(ext.Signature.Params) && !ext.Signature.Params[i].Mutable {
			loweredArg, err = l.lowerExprWithExpectedType(fn, arg, ext.Signature.Params[i].Type)
		} else {
			loweredArg, err = l.lowerExpr(fn, arg)
		}
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, loweredArg.stmts...)
		argExpr := loweredArg.expr
		if i < len(ext.Signature.Params) && l.isVoidType(ext.Signature.Params[i].Type) {
			stmts = l.appendVoidValueEval(stmts, argExpr)
			argExpr = l.voidValueExpr()
		}
		args = append(args, argExpr)
	}
	binding := ext.Name
	if goBinding, ok := ext.Bindings["go"]; ok && goBinding != "" {
		binding = goBinding
	}
	if direct, ok, err := l.lowerDirectGoExternCall(ext, binding, args, stmts); err != nil || ok {
		return direct, err
	}
	if !externModuleIsStdlib(l.program, ext) {
		if alias, ok := dependencyAliasForModulePath(modulePathForExtern(l.program, ext), l.projectInfo); ok {
			return l.lowerDependencyExternCall(ext, alias, binding, args, stmts, expr.Type)
		}
		return l.lowerProjectExternCall(ext, binding, args, stmts, expr.Type)
	}
	channel, ok, err := l.lowerChannelStdlibExtern(binding, args, stmts, expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	if ok {
		return channel, nil
	}
	generated, ok, err := l.lowerGeneratedStdlibExtern(binding, ext.Signature, args, stmts, expr.Type)
	if err != nil {
		return loweredExpr{}, err
	}
	if ok {
		return generated, nil
	}
	return loweredExpr{}, fmt.Errorf("unsupported go extern binding %q", binding)
}

func (l *lowerer) lowerChannelStdlibExtern(binding string, args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, bool, error) {
	switch binding {
	case "ChannelNew":
		out, err := l.lowerChannelNew(args, stmts, returnTypeID)
		return out, true, err
	case "ChannelSend":
		call := &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "ChannelSend"), Args: args}
		return loweredExpr{stmts: stmts, expr: call}, true, nil
	case "ChannelRecv":
		call := &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "ChannelRecv"), Args: args}
		return loweredExpr{stmts: stmts, expr: call}, true, nil
	case "ChannelClose":
		call := &ast.CallExpr{Fun: l.qualified("stdlibffi", "github.com/akonwi/ard/std_lib/ffi", "ChannelClose"), Args: args}
		return loweredExpr{stmts: stmts, expr: call}, true, nil
	default:
		return loweredExpr{}, false, nil
	}
}

func (l *lowerer) lowerChannelNew(args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, error) {
	if !validTypeID(l.program, returnTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid channel type id %d", returnTypeID)
	}
	info := l.program.Types[returnTypeID-1]
	if info.Kind != air.TypeStruct || info.Name != "Channel" && !strings.HasPrefix(info.Name, "Channel<") {
		return loweredExpr{}, fmt.Errorf("ChannelNew returned non-Channel type %s", info.Name)
	}
	var rawField air.FieldInfo
	foundRawField := false
	for _, field := range info.Fields {
		if field.Name == "chan" {
			rawField = field
			foundRawField = true
			break
		}
	}
	if !foundRawField || !validTypeID(l.program, rawField.Type) {
		return loweredExpr{}, fmt.Errorf("ChannelNew result type %s has no raw channel field", info.Name)
	}
	rawInfo := l.program.Types[rawField.Type-1]
	if !l.isChannelExternType(rawInfo) {
		return loweredExpr{}, fmt.Errorf("ChannelNew raw field has non-channel type %s", rawInfo.Name)
	}
	elemType, err := l.goType(rawInfo.Elem)
	if err != nil {
		return loweredExpr{}, err
	}
	channelType := func() ast.Expr {
		return &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: elemType}
	}
	makeCall := func(capacity ast.Expr) ast.Expr {
		call := &ast.CallExpr{Fun: ast.NewIdent("make"), Args: []ast.Expr{channelType()}}
		if capacity != nil {
			call.Args = append(call.Args, capacity)
		}
		return call
	}
	channelTemp := l.nextTemp()
	stmts = append(stmts, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names:  []*ast.Ident{ast.NewIdent(channelTemp)},
		Type:   channelType(),
		Values: []ast.Expr{makeCall(nil)},
	}}}})
	if len(args) > 0 {
		sizeTemp := l.nextTemp()
		stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sizeTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{args[0]}})
		sizeExpr := ast.NewIdent(sizeTemp)
		cond := &ast.BinaryExpr{
			X:  l.maybeIsSomeExpr(sizeExpr),
			Op: token.LAND,
			Y: &ast.BinaryExpr{
				X:  l.maybeValueExpr(sizeExpr),
				Op: token.GTR,
				Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
			},
		}
		stmts = append(stmts, &ast.IfStmt{Cond: cond, Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(channelTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{makeCall(l.maybeValueExpr(sizeExpr))}},
		}}})
	}
	result := &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, info)), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(l.goFieldName(info, rawField.Name)), Value: ast.NewIdent(channelTemp)},
	}}
	return loweredExpr{stmts: stmts, expr: result}, nil
}

func (l *lowerer) adaptProjectExternArgs(signature air.Signature, args []ast.Expr) ([]ast.Expr, []ast.Stmt, error) {
	if len(signature.Params) != len(args) {
		return nil, nil, fmt.Errorf("project extern argument count mismatch: signature has %d params, call has %d args", len(signature.Params), len(args))
	}
	return append([]ast.Expr(nil), args...), nil, nil
}

func (l *lowerer) lowerDependencyExternCall(ext air.Extern, alias string, binding string, args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, error) {
	return l.lowerDirectExternCall(ext, l.dependencyFFIBindingExpr, alias, binding, args, stmts, returnTypeID)
}

func (l *lowerer) dependencyFFITypeExpr(alias string, binding string) (ast.Expr, error) {
	expr, err := parser.ParseExpr(binding)
	if err != nil {
		return nil, fmt.Errorf("invalid go extern type binding %q: %w", binding, err)
	}
	return l.rewriteDependencyFFITypeExpr(alias, binding, expr)
}

func (l *lowerer) rewriteDependencyFFITypeExpr(alias string, binding string, expr ast.Expr) (ast.Expr, error) {
	switch node := expr.(type) {
	case *ast.StarExpr:
		x, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.X)
		if err != nil {
			return nil, err
		}
		return &ast.StarExpr{X: x}, nil
	case *ast.ArrayType:
		elt, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.Elt)
		if err != nil {
			return nil, err
		}
		return &ast.ArrayType{Len: node.Len, Elt: elt}, nil
	case *ast.MapType:
		key, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.Key)
		if err != nil {
			return nil, err
		}
		value, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.Value)
		if err != nil {
			return nil, err
		}
		return &ast.MapType{Key: key, Value: value}, nil
	case *ast.ChanType:
		value, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.Value)
		if err != nil {
			return nil, err
		}
		return &ast.ChanType{Dir: node.Dir, Value: value}, nil
	case *ast.Ellipsis:
		elt, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.Elt)
		if err != nil {
			return nil, err
		}
		return &ast.Ellipsis{Elt: elt}, nil
	case *ast.FuncType:
		params, err := l.rewriteDependencyFFIFieldList(alias, binding, node.Params)
		if err != nil {
			return nil, err
		}
		results, err := l.rewriteDependencyFFIFieldList(alias, binding, node.Results)
		if err != nil {
			return nil, err
		}
		return &ast.FuncType{Params: params, Results: results}, nil
	case *ast.StructType:
		fields, err := l.rewriteDependencyFFIFieldList(alias, binding, node.Fields)
		if err != nil {
			return nil, err
		}
		return &ast.StructType{Fields: fields}, nil
	case *ast.InterfaceType:
		methods, err := l.rewriteDependencyFFIFieldList(alias, binding, node.Methods)
		if err != nil {
			return nil, err
		}
		return &ast.InterfaceType{Methods: methods}, nil
	case *ast.IndexExpr:
		x, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.X)
		if err != nil {
			return nil, err
		}
		index, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.Index)
		if err != nil {
			return nil, err
		}
		return &ast.IndexExpr{X: x, Index: index}, nil
	case *ast.IndexListExpr:
		x, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.X)
		if err != nil {
			return nil, err
		}
		indices := make([]ast.Expr, len(node.Indices))
		for i := range node.Indices {
			indices[i], err = l.rewriteDependencyFFITypeExpr(alias, binding, node.Indices[i])
			if err != nil {
				return nil, err
			}
		}
		return &ast.IndexListExpr{X: x, Indices: indices}, nil
	case *ast.ParenExpr:
		x, err := l.rewriteDependencyFFITypeExpr(alias, binding, node.X)
		if err != nil {
			return nil, err
		}
		return &ast.ParenExpr{X: x}, nil
	case *ast.SelectorExpr:
		if ident, ok := node.X.(*ast.Ident); ok && ident.Name == "ffi" {
			packageAlias := sanitizeName(alias) + "ffi"
			return l.qualified(packageAlias, "generated/depffi/"+sanitizeName(alias), node.Sel.Name), nil
		}
		l.registerFFIImportsForGoType(node)
		return node, nil
	case *ast.Ident:
		if isPredeclaredGoTypeName(node.Name) {
			return node, nil
		}
		return nil, fmt.Errorf("dependency go extern type binding %q must qualify %s with package ffi", binding, node.Name)
	default:
		l.registerFFIImportsForGoType(node)
		return node, nil
	}
}

func (l *lowerer) rewriteDependencyFFIFieldList(alias string, binding string, fields *ast.FieldList) (*ast.FieldList, error) {
	if fields == nil {
		return nil, nil
	}
	rewritten := &ast.FieldList{List: make([]*ast.Field, len(fields.List))}
	for i, field := range fields.List {
		copyField := *field
		if field.Type != nil {
			typ, err := l.rewriteDependencyFFITypeExpr(alias, binding, field.Type)
			if err != nil {
				return nil, err
			}
			copyField.Type = typ
		}
		rewritten.List[i] = &copyField
	}
	return rewritten, nil
}

func (l *lowerer) lowerProjectExternCall(ext air.Extern, binding string, args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, error) {
	return l.lowerDirectExternCall(ext, func(_ string, binding string) (ast.Expr, error) { return l.projectFFIBindingExpr(binding) }, "", binding, args, stmts, returnTypeID)
}

func (l *lowerer) applyExplicitExternTypeArgs(callee ast.Expr, typeArgs []air.TypeID) (ast.Expr, error) {
	if len(typeArgs) == 0 {
		return callee, nil
	}
	indices := make([]ast.Expr, len(typeArgs))
	for i, typeArg := range typeArgs {
		goType, err := l.goType(typeArg)
		if err != nil {
			return nil, err
		}
		indices[i] = goType
	}
	if len(indices) == 1 {
		return &ast.IndexExpr{X: callee, Index: indices[0]}, nil
	}
	return &ast.IndexListExpr{X: callee, Indices: indices}, nil
}

func (l *lowerer) lowerDirectExternCall(ext air.Extern, bindingExpr func(string, string) (ast.Expr, error), alias string, binding string, args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, error) {
	adaptedArgs, argStmts, err := l.adaptProjectExternArgs(ext.Signature, args)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts = append(stmts, argStmts...)
	callee, err := bindingExpr(alias, binding)
	if err != nil {
		return loweredExpr{}, err
	}
	callee, err = l.applyExplicitExternTypeArgs(callee, ext.TypeArgs)
	if err != nil {
		return loweredExpr{}, err
	}
	call := &ast.CallExpr{Fun: callee, Args: adaptedArgs}
	if !validTypeID(l.program, returnTypeID) {
		return loweredExpr{stmts: stmts, expr: call}, nil
	}
	returnType := l.program.Types[returnTypeID-1]
	switch returnType.Kind {
	case air.TypeVoid:
		return loweredExpr{stmts: stmts, expr: call}, nil
	case air.TypeMaybe:
		if wrapped, ok, err := l.wrapMaybeNativeTraitExternCall(returnTypeID, call, stmts); ok || err != nil {
			return wrapped, err
		}
		return loweredExpr{stmts: stmts, expr: call}, nil
	case air.TypeStruct:
		if rawField, ok := l.channelStructRawField(returnType); ok {
			wrapped := &ast.CompositeLit{Type: ast.NewIdent(typeName(l.program, returnType)), Elts: []ast.Expr{
				&ast.KeyValueExpr{Key: ast.NewIdent(l.goFieldName(returnType, rawField.Name)), Value: call},
			}}
			return loweredExpr{stmts: stmts, expr: wrapped}, nil
		}
		return loweredExpr{stmts: stmts, expr: call}, nil
	case air.TypeResult:
		if validTypeID(l.program, returnType.Value) && l.program.Types[returnType.Value-1].Kind == air.TypeVoid {
			wrapped, err := l.wrapErrorCall(returnTypeID, call)
			if err != nil {
				return loweredExpr{}, err
			}
			wrapped.stmts = append(stmts, wrapped.stmts...)
			return wrapped, nil
		}
		wrapped, err := l.wrapValueErrorCall(returnTypeID, call)
		if err != nil {
			return loweredExpr{}, err
		}
		wrapped.stmts = append(stmts, wrapped.stmts...)
		return wrapped, nil
	default:
		if l.usesNativeTraitInterface(returnTypeID) {
			coerced, err := l.nativeTraitInterfaceAssertion(returnTypeID, call)
			if err != nil {
				return loweredExpr{}, err
			}
			return loweredExpr{stmts: stmts, expr: coerced}, nil
		}
		return loweredExpr{stmts: stmts, expr: call}, nil
	}
}

func (l *lowerer) nativeTraitInterfaceAssertion(typeID air.TypeID, value ast.Expr) (ast.Expr, error) {
	traitType, err := l.goType(typeID)
	if err != nil {
		return nil, err
	}
	return &ast.TypeAssertExpr{X: &ast.CallExpr{Fun: ast.NewIdent("any"), Args: []ast.Expr{value}}, Type: traitType}, nil
}

func (l *lowerer) channelStructRawField(info air.TypeInfo) (air.FieldInfo, bool) {
	if info.Kind != air.TypeStruct || info.Name != "Channel" && !strings.HasPrefix(info.Name, "Channel<") {
		return air.FieldInfo{}, false
	}
	for _, field := range info.Fields {
		if field.Name != "chan" || !validTypeID(l.program, field.Type) {
			continue
		}
		if l.isChannelExternType(l.program.Types[field.Type-1]) {
			return field, true
		}
	}
	return air.FieldInfo{}, false
}

func (l *lowerer) dependencyFFIBindingExpr(alias string, binding string) (ast.Expr, error) {
	if strings.TrimSpace(alias) == "" {
		return nil, fmt.Errorf("empty dependency alias for go extern binding %q", binding)
	}
	if strings.TrimSpace(binding) == "" {
		return nil, fmt.Errorf("empty go extern binding")
	}
	if !token.IsIdentifier(binding) {
		return nil, fmt.Errorf("dependency go extern binding %q must be an unqualified function name in package ffi", binding)
	}
	packageAlias := sanitizeName(alias) + "ffi"
	return l.qualified(packageAlias, "generated/depffi/"+sanitizeName(alias), binding), nil
}

func (l *lowerer) projectFFIBindingExpr(binding string) (ast.Expr, error) {
	if strings.TrimSpace(binding) == "" {
		return nil, fmt.Errorf("empty go extern binding")
	}
	expr, err := parser.ParseExpr(binding)
	if err != nil {
		return nil, fmt.Errorf("invalid go extern binding %q: %w", binding, err)
	}
	if name, ok := unqualifiedExternFunctionIdent(expr); ok {
		return nil, fmt.Errorf("project go extern binding %q must qualify %s with package %s", binding, name, projectFFIPackageAlias(l.projectInfo))
	}
	if !isQualifiedExternFunctionExpr(expr) {
		return nil, fmt.Errorf("project go extern binding %q must be a qualified function reference", binding)
	}
	l.registerFFIImportsForGoType(expr)
	return expr, nil
}

func unqualifiedExternFunctionIdent(expr ast.Expr) (string, bool) {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name, true
	case *ast.IndexExpr:
		return unqualifiedExternFunctionIdent(node.X)
	case *ast.IndexListExpr:
		return unqualifiedExternFunctionIdent(node.X)
	case *ast.ParenExpr:
		return unqualifiedExternFunctionIdent(node.X)
	default:
		return "", false
	}
}

func isQualifiedExternFunctionExpr(expr ast.Expr) bool {
	switch node := expr.(type) {
	case *ast.SelectorExpr:
		_, ok := node.X.(*ast.Ident)
		return ok && node.Sel != nil
	case *ast.IndexExpr:
		return isQualifiedExternFunctionExpr(node.X)
	case *ast.IndexListExpr:
		return isQualifiedExternFunctionExpr(node.X)
	case *ast.ParenExpr:
		return isQualifiedExternFunctionExpr(node.X)
	default:
		return false
	}
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
	case air.ExprSpawnFiber:
		if expr.Target == nil {
			directRefs[expr.Function] = true
		}
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
		case air.ExprSpawnFiber:
			found = expr.Target == nil && expr.Function == function
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

func (l *lowerer) markJSONParseType(typeID air.TypeID) {
	if !validTypeID(l.program, typeID) || l.jsonParseTypes[typeID] {
		return
	}
	l.jsonParseTypes[typeID] = true
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeMaybe, air.TypeList:
		l.markJSONParseType(info.Elem)
	case air.TypeMap:
		l.markJSONParseType(info.Key)
		l.markJSONParseType(info.Value)
	case air.TypeStruct:
		for _, field := range info.Fields {
			l.markJSONParseType(field.Type)
		}
	case air.TypeUnion:
		for _, member := range info.Members {
			l.markJSONParseType(member.Type)
		}
	case air.TypeResult:
		l.markJSONParseType(info.Value)
		l.markJSONParseType(info.Error)
	}
}

func (l *lowerer) markJSONEncodeType(typeID air.TypeID) {
	if !validTypeID(l.program, typeID) || l.jsonEncodeTypes[typeID] {
		return
	}
	l.jsonEncodeTypes[typeID] = true
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeMaybe, air.TypeList, air.TypeFiber:
		l.markJSONEncodeType(info.Elem)
	case air.TypeMap:
		l.markJSONEncodeType(info.Key)
		l.markJSONEncodeType(info.Value)
	case air.TypeStruct:
		for _, field := range info.Fields {
			l.markJSONEncodeType(field.Type)
		}
	case air.TypeUnion:
		for _, member := range info.Members {
			l.markJSONEncodeType(member.Type)
		}
	case air.TypeResult:
		l.markJSONEncodeType(info.Value)
		l.markJSONEncodeType(info.Error)
	case air.TypeFunction:
		for _, param := range info.Params {
			l.markJSONEncodeType(param)
		}
		l.markJSONEncodeType(info.Return)
	case air.TypeExtern:
		if info.Elem != air.NoType {
			l.markJSONEncodeType(info.Elem)
		}
	}
}

func (l *lowerer) lowerJSONEncodeStdlibExtern(signature air.Signature, args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, error) {
	if len(args) != 1 || len(signature.Params) != 1 {
		return loweredExpr{}, fmt.Errorf("JsonEncode expects 1 arg")
	}
	valueTypeID := signature.Params[0].Type
	l.markRuntimeHelper("json_encode")
	l.markJSONEncodeType(valueTypeID)
	helper := l.jsonEncodeTopHelperName(valueTypeID)
	// JsonEncode is lowered with type-specific helpers instead of the generic
	// stdlib FFI call so the Go target can preserve Ard JSON semantics for
	// maybe/union/dynamic-heavy values. When the static type is simple enough
	// for native Go JSON encoding to match those semantics, prefer json.Marshal:
	// it is noticeably faster on JSON-heavy benchmarks while keeping the
	// Ard-aware streaming encoder as the universal fallback.
	if l.jsonNativeCodecSafe(valueTypeID, map[air.TypeID]bool{}) {
		helper = l.jsonEncodeMarshalTopHelperName(valueTypeID)
	}
	wrapped, err := l.wrapValueErrorCall(returnTypeID, &ast.CallExpr{Fun: ast.NewIdent(helper), Args: args})
	if err != nil {
		return loweredExpr{}, err
	}
	wrapped.stmts = append(stmts, wrapped.stmts...)
	return wrapped, nil
}

func (l *lowerer) lowerJSONParseStdlibExtern(args []ast.Expr, stmts []ast.Stmt, returnTypeID air.TypeID) (loweredExpr, error) {
	if len(args) != 1 {
		return loweredExpr{}, fmt.Errorf("JsonParse expects 1 arg")
	}
	if !validTypeID(l.program, returnTypeID) {
		return loweredExpr{}, fmt.Errorf("invalid JsonParse result type %d", returnTypeID)
	}
	resultInfo := l.program.Types[returnTypeID-1]
	if resultInfo.Kind != air.TypeResult {
		return loweredExpr{}, fmt.Errorf("JsonParse expected result return, got %s", resultInfo.Name)
	}
	l.markRuntimeHelper("json_parse")
	l.markJSONParseType(resultInfo.Value)
	valueType, err := l.goType(resultInfo.Value)
	if err != nil {
		return loweredExpr{}, err
	}
	errTemp := l.nextTemp()
	outTemp := l.nextTemp()
	decoderTemp := l.nextTemp()

	allStmts := append([]ast.Stmt{}, stmts...)
	allStmts = append(allStmts,
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errTemp)}, Type: ast.NewIdent("string")}}}},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(outTemp)}, Type: valueType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(decoderTemp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{Fun: l.qualified("jsontext", "encoding/json/jsontext", "NewDecoder"), Args: []ast.Expr{&ast.CallExpr{Fun: l.qualified("bytes", "bytes", "NewReader"), Args: []ast.Expr{&ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Args: []ast.Expr{args[0]}}}}}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(outTemp), ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent(l.jsonDecodeTextHelperName(resultInfo.Value)), Args: []ast.Expr{ast.NewIdent(decoderTemp), &ast.BasicLit{Kind: token.STRING, Value: `""`}}}}},
		&ast.IfStmt{Cond: &ast.BinaryExpr{X: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: &ast.BasicLit{Kind: token.STRING, Value: `""`}}, Op: token.LAND, Y: &ast.BinaryExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(decoderTemp), Sel: ast.NewIdent("PeekKind")}}, Op: token.NEQ, Y: l.qualified("jsontext", "encoding/json/jsontext", "KindInvalid")}}, Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errTemp)}, Tok: token.ASSIGN, Rhs: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"trailing JSON after value"`}}}}}},
	)
	resultExpr := &ast.CompositeLit{Type: mustTypeExpr(l, returnTypeID), Elts: []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent("Value"), Value: ast.NewIdent(outTemp)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Err"), Value: ast.NewIdent(errTemp)},
		&ast.KeyValueExpr{Key: ast.NewIdent("Ok"), Value: &ast.BinaryExpr{X: ast.NewIdent(errTemp), Op: token.EQL, Y: &ast.BasicLit{Kind: token.STRING, Value: `""`}}},
	}}
	return loweredExpr{stmts: allStmts, expr: resultExpr}, nil
}

func (l *lowerer) jsonParsePreludeSource() string {
	var b strings.Builder
	b.WriteString(`
func ardJSONPath(path string, segment string) string {
	if path == "" {
		return segment
	}
	if len(segment) > 0 && segment[0] == '[' {
		return path + segment
	}
	return path + "." + segment
}

func ardJSONFound(raw any) string {
	switch raw.(type) {
	case nil:
		return "null"
	case string:
		return "Str"
	case bool:
		return "Bool"
	case float64, float32, int, int64, uint64:
		return "Number"
	case []any:
		return "List"
	case map[string]any:
		return "Map"
	default:
		return fmt.Sprintf("%T", raw)
	}
}

func ardJSONErr(path string, expected string, raw any) string {
	message := "got " + ardJSONFound(raw) + ", expected " + expected
	if path == "" {
		return message
	}
	return path + ": " + message
}

func ardJSONMissing(path string, expected string) string {
	if path == "" {
		return "missing, expected " + expected
	}
	return path + ": missing, expected " + expected
}

func ardJSONDecodeInt(dec *jsontext.Decoder, path string) (int, string) {
	var out int
	tok, err := dec.ReadToken()
	if err != nil { return out, err.Error() }
	if tok.Kind() != jsontext.KindNumber { return out, path + ": got " + tok.Kind().String() + ", expected Int" }
	parsed, err := strconv.Atoi(tok.String())
	if err != nil { return out, path + ": got Number, expected Int" }
	return parsed, ""
}

func ardJSONDecodeFloat(dec *jsontext.Decoder, path string) (float64, string) {
	var out float64
	tok, err := dec.ReadToken()
	if err != nil { return out, err.Error() }
	if tok.Kind() != jsontext.KindNumber { return out, path + ": got " + tok.Kind().String() + ", expected Float" }
	parsed, err := strconv.ParseFloat(tok.String(), 64)
	if err != nil { return out, path + ": got Number, expected Float" }
	return parsed, ""
}

func ardJSONDecodeBool(dec *jsontext.Decoder, path string) (bool, string) {
	var out bool
	tok, err := dec.ReadToken()
	if err != nil { return out, err.Error() }
	if tok.Kind() == jsontext.KindTrue { return true, "" }
	if tok.Kind() == jsontext.KindFalse { return false, "" }
	return out, path + ": got " + tok.Kind().String() + ", expected Bool"
}

func ardJSONDecodeString(dec *jsontext.Decoder, path string) (string, string) {
	var out string
	tok, err := dec.ReadToken()
	if err != nil { return out, err.Error() }
	if tok.Kind() != jsontext.KindString { return out, path + ": got " + tok.Kind().String() + ", expected Str" }
	return tok.String(), ""
}

func ardJSONDecodeDynamic(dec *jsontext.Decoder, path string) (any, string) {
	value, err := dec.ReadValue()
	if err != nil { return nil, err.Error() }
	var raw any
	if err := json.Unmarshal(value, &raw); err != nil { return nil, err.Error() }
	return raw, ""
}

func ardJSONDecodeByteList(dec *jsontext.Decoder, path string, expected string) ([]byte, string) {
	var out []byte
	if dec.PeekKind() != jsontext.KindString {
		tok, err := dec.ReadToken()
		if err != nil { return out, err.Error() }
		return out, path + ": got " + tok.Kind().String() + ", expected " + expected
	}
	value, err := dec.ReadValue()
	if err != nil { return out, err.Error() }
	if err := json.Unmarshal(value, &out); err != nil { return out, path + ": got Str, expected " + expected }
	return out, ""
}

func ardJSONDecodeMaybe[T any](dec *jsontext.Decoder, path string, decode func(*jsontext.Decoder, string) (T, string)) (ardruntime.Maybe[T], string) {
	var out ardruntime.Maybe[T]
	if dec.PeekKind() == jsontext.KindNull { _, _ = dec.ReadToken(); return out, "" }
	value, message := decode(dec, path)
	if message != "" { return out, message }
	return ardruntime.Some(value), ""
}

func ardJSONDecodeList[T any](dec *jsontext.Decoder, path string, expected string, decode func(*jsontext.Decoder, string) (T, string)) ([]T, string) {
	var out []T
	tok, err := dec.ReadToken()
	if err != nil { return out, err.Error() }
	if tok.Kind() != jsontext.KindBeginArray { return out, path + ": got " + tok.Kind().String() + ", expected " + expected }
	out = make([]T, 0)
	for i := 0; dec.PeekKind() != jsontext.KindEndArray; i++ {
		value, message := decode(dec, ardJSONPath(path, fmt.Sprintf("[%d]", i)))
		if message != "" { return out, message }
		out = append(out, value)
	}
	_, err = dec.ReadToken()
	if err != nil { return out, err.Error() }
	return out, ""
}

func ardJSONDecodeStringMap[V any](dec *jsontext.Decoder, path string, expected string, decode func(*jsontext.Decoder, string) (V, string)) (map[string]V, string) {
	var out map[string]V
	tok, err := dec.ReadToken()
	if err != nil { return out, err.Error() }
	if tok.Kind() != jsontext.KindBeginObject { return out, path + ": got " + tok.Kind().String() + ", expected " + expected }
	out = make(map[string]V)
	for dec.PeekKind() != jsontext.KindEndObject {
		keyTok, err := dec.ReadToken()
		if err != nil { return out, err.Error() }
		key := keyTok.String()
		value, message := decode(dec, ardJSONPath(path, key))
		if message != "" { return out, message }
		out[key] = value
	}
	_, err = dec.ReadToken()
	if err != nil { return out, err.Error() }
	return out, ""
}
`)
	for _, typ := range l.program.Types {
		if typ.ID == air.NoType || !l.jsonParseTypes[typ.ID] {
			continue
		}
		l.writeJSONDecodeTextHelper(&b, typ.ID)
	}
	return b.String()
}

func (l *lowerer) jsonParseGoTypeName(typeID air.TypeID) string {
	if !validTypeID(l.program, typeID) {
		return "any"
	}
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeVoid:
		return "ardruntime.Void"
	case air.TypeInt:
		return "int"
	case air.TypeByte:
		return "byte"
	case air.TypeRune:
		return "rune"
	case air.TypeFloat:
		return "float64"
	case air.TypeBool:
		return "bool"
	case air.TypeStr:
		return "string"
	case air.TypeDynamic, air.TypeExtern, air.TypeTraitObject:
		return "any"
	case air.TypeMaybe:
		return "ardruntime.Maybe[" + l.jsonParseGoTypeName(info.Elem) + "]"
	case air.TypeList:
		return "[]" + l.jsonParseGoTypeName(info.Elem)
	case air.TypeMap:
		return "map[" + l.jsonParseGoTypeName(info.Key) + "]" + l.jsonParseGoTypeName(info.Value)
	case air.TypeStruct, air.TypeEnum, air.TypeUnion:
		return typeName(l.program, info)
	default:
		return "any"
	}
}

func (l *lowerer) jsonEncodeGoTypeName(typeID air.TypeID) string {
	typ, err := l.goType(typeID)
	if err != nil {
		return l.jsonParseGoTypeName(typeID)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), typ); err != nil {
		return l.jsonParseGoTypeName(typeID)
	}
	return buf.String()
}

func (l *lowerer) jsonEncodePreludeSource() string {
	var b strings.Builder
	needsMaybeHelper := false
	needsStructuralMapHelper := false
	for _, typ := range l.program.Types {
		if typ.ID == air.NoType || !l.jsonEncodeTypes[typ.ID] {
			continue
		}
		if typ.Kind == air.TypeMaybe {
			needsMaybeHelper = true
		}
		if typ.Kind == air.TypeMap && l.mapUsesStructuralKeys(typ.ID) {
			needsStructuralMapHelper = true
		}
	}
	if needsMaybeHelper || needsStructuralMapHelper {
		l.registerImport("ardruntime", "github.com/akonwi/ard/runtime")
	}
	b.WriteString(`
func ardJSONEncodeInt(enc *jsontext.Encoder, value int) error {
	return enc.WriteToken(jsontext.Int(int64(value)))
}

func ardJSONEncodeFloat(enc *jsontext.Encoder, value float64) error {
	return enc.WriteToken(jsontext.Float(value))
}

func ardJSONEncodeBool(enc *jsontext.Encoder, value bool) error {
	if value { return enc.WriteToken(jsontext.True) }
	return enc.WriteToken(jsontext.False)
}

func ardJSONEncodeString(enc *jsontext.Encoder, value string) error {
	return enc.WriteToken(jsontext.String(value))
}

func ardJSONEncodeDynamic(enc *jsontext.Encoder, value any) error {
	data, err := json.Marshal(value)
	if err != nil { return err }
	return enc.WriteValue(jsontext.Value(data))
}
`)
	if needsMaybeHelper {
		b.WriteString(`
func ardJSONEncodeMaybe[T any](enc *jsontext.Encoder, value ardruntime.Maybe[T], encode func(*jsontext.Encoder, T) error) error {
	if value.IsNone() { return enc.WriteToken(jsontext.Null) }
	return encode(enc, value.Value())
}
`)
	}
	b.WriteString(`
func ardJSONEncodeList[T any](enc *jsontext.Encoder, values []T, encode func(*jsontext.Encoder, T) error) error {
	if err := enc.WriteToken(jsontext.BeginArray); err != nil { return err }
	for _, item := range values {
		if err := encode(enc, item); err != nil { return err }
	}
	return enc.WriteToken(jsontext.EndArray)
}

func ardJSONEncodeMap[K comparable, V any](enc *jsontext.Encoder, values map[K]V, encode func(*jsontext.Encoder, V) error) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil { return err }
	for key, item := range values {
		if err := enc.WriteToken(jsontext.String(fmt.Sprint(key))); err != nil { return err }
		if err := encode(enc, item); err != nil { return err }
	}
	return enc.WriteToken(jsontext.EndObject)
}
`)
	if needsStructuralMapHelper {
		b.WriteString(`
func ardJSONEncodeStructuralMap[K any, V any](enc *jsontext.Encoder, values ardruntime.StructuralMap[K, V], encode func(*jsontext.Encoder, V) error) error {
	if err := enc.WriteToken(jsontext.BeginObject); err != nil { return err }
	for _, key := range values.Keys() {
		item, _ := values.Get(key)
		if err := enc.WriteToken(jsontext.String(fmt.Sprint(key))); err != nil { return err }
		if err := encode(enc, item); err != nil { return err }
	}
	return enc.WriteToken(jsontext.EndObject)
}
`)
	}
	for _, typ := range l.program.Types {
		if typ.ID == air.NoType || !l.jsonEncodeTypes[typ.ID] {
			continue
		}
		l.writeJSONEncodeHelper(&b, typ.ID)
	}
	return b.String()
}

func (l *lowerer) writeJSONEncodeHelper(b *strings.Builder, typeID air.TypeID) {
	if !validTypeID(l.program, typeID) {
		return
	}
	info := l.program.Types[typeID-1]
	typeName := l.jsonEncodeGoTypeName(typeID)
	helper := l.jsonEncodeHelperName(typeID)
	fmt.Fprintf(b, "\nfunc %s(enc *jsontext.Encoder, value %s) error {\n", helper, typeName)
	switch info.Kind {
	case air.TypeVoid:
		fmt.Fprintf(b, "\treturn enc.WriteToken(jsontext.Null)\n")
	case air.TypeInt, air.TypeByte, air.TypeRune:
		fmt.Fprintf(b, "\treturn ardJSONEncodeInt(enc, int(value))\n")
	case air.TypeFloat:
		fmt.Fprintf(b, "\treturn ardJSONEncodeFloat(enc, value)\n")
	case air.TypeBool:
		fmt.Fprintf(b, "\treturn ardJSONEncodeBool(enc, value)\n")
	case air.TypeStr:
		fmt.Fprintf(b, "\treturn ardJSONEncodeString(enc, value)\n")
	case air.TypeDynamic, air.TypeExtern, air.TypeTraitObject:
		fmt.Fprintf(b, "\treturn ardJSONEncodeDynamic(enc, value)\n")
	case air.TypeMaybe:
		fmt.Fprintf(b, "\treturn ardJSONEncodeMaybe(enc, value, %s)\n", l.jsonEncodeHelperName(info.Elem))
	case air.TypeResult:
		fmt.Fprintf(b, "\tif value.Ok { return %s(enc, value.Value) }\n\treturn %s(enc, value.Err)\n", l.jsonEncodeHelperName(info.Value), l.jsonEncodeHelperName(info.Error))
	case air.TypeList:
		if l.typeKind(info.Elem) == air.TypeByte {
			fmt.Fprintf(b, "\tdata, err := json.Marshal(value)\n\tif err != nil { return err }\n\treturn enc.WriteValue(jsontext.Value(data))\n")
		} else {
			fmt.Fprintf(b, "\treturn ardJSONEncodeList(enc, value, %s)\n", l.jsonEncodeHelperName(info.Elem))
		}
	case air.TypeMap:
		if l.mapUsesStructuralKeys(typeID) {
			fmt.Fprintf(b, "\treturn ardJSONEncodeStructuralMap(enc, value, %s)\n", l.jsonEncodeHelperName(info.Value))
		} else {
			fmt.Fprintf(b, "\treturn ardJSONEncodeMap(enc, value, %s)\n", l.jsonEncodeHelperName(info.Value))
		}
	case air.TypeStruct:
		fmt.Fprintf(b, "\tif err := enc.WriteToken(jsontext.BeginObject); err != nil { return err }\n")
		for _, field := range info.Fields {
			fieldValue := "value." + l.goFieldName(info, field.Name)
			if field.Mutable {
				fieldValue = "*" + fieldValue
			}
			fmt.Fprintf(b, "\tif err := enc.WriteToken(jsontext.String(%q)); err != nil { return err }\n", field.Name)
			fmt.Fprintf(b, "\tif err := %s(enc, %s); err != nil { return err }\n", l.jsonEncodeHelperName(field.Type), fieldValue)
		}
		fmt.Fprintf(b, "\treturn enc.WriteToken(jsontext.EndObject)\n")
	case air.TypeEnum:
		fmt.Fprintf(b, "\treturn enc.WriteToken(jsontext.Int(int64(value)))\n")
	case air.TypeUnion:
		fmt.Fprintf(b, "\tswitch value.tag {\n")
		for _, member := range info.Members {
			fieldName := unionMemberFieldName(member)
			fmt.Fprintf(b, "\tcase %d:\n\t\treturn %s(enc, value.%s)\n", member.Tag, l.jsonEncodeHelperName(member.Type), fieldName)
		}
		fmt.Fprintf(b, "\t}\n\treturn enc.WriteToken(jsontext.Null)\n")
	case air.TypeFunction, air.TypeFiber:
		fmt.Fprintf(b, "\tdata, err := json.Marshal(value)\n\tif err != nil { return err }\n\treturn enc.WriteValue(jsontext.Value(data))\n")
	default:
		fmt.Fprintf(b, "\tdata, err := json.Marshal(value)\n\tif err != nil { return err }\n\treturn enc.WriteValue(jsontext.Value(data))\n")
	}
	fmt.Fprintf(b, "}\n")
	if info.Kind == air.TypeStruct && !l.isStdlibFFIBackedType(info) {
		fmt.Fprintf(b, "\nfunc (value %s) MarshalJSONTo(enc *jsontext.Encoder) error {\n\treturn %s(enc, value)\n}\n", typeName, helper)
	}
	fmt.Fprintf(b, "\nfunc %s(value %s) (string, error) {\n\tvar buf bytes.Buffer\n\tenc := jsontext.NewEncoder(&buf)\n\tif err := %s(enc, value); err != nil { return \"\", err }\n\treturn string(bytes.TrimSuffix(buf.Bytes(), []byte(\"\\n\"))), nil\n}\n", l.jsonEncodeTopHelperName(typeID), typeName, helper)
	fmt.Fprintf(b, "\nfunc %s(value %s) (string, error) {\n\tdata, err := json.Marshal(value)\n\tif err != nil { return \"\", err }\n\treturn string(data), nil\n}\n", l.jsonEncodeMarshalTopHelperName(typeID), typeName)
}

func (l *lowerer) jsonEncodeHelperName(typeID air.TypeID) string {
	return fmt.Sprintf("ardJSONEncode_%d", typeID)
}

func (l *lowerer) jsonEncodeTopHelperName(typeID air.TypeID) string {
	return fmt.Sprintf("ardJSONEncodeTop_%d", typeID)
}

func (l *lowerer) jsonEncodeMarshalTopHelperName(typeID air.TypeID) string {
	return fmt.Sprintf("ardJSONMarshalTop_%d", typeID)
}

func (l *lowerer) jsonNativeCodecSafe(typeID air.TypeID, seen map[air.TypeID]bool) bool {
	if !validTypeID(l.program, typeID) {
		return false
	}
	if seen[typeID] {
		return true
	}
	seen[typeID] = true
	info := l.program.Types[typeID-1]
	switch info.Kind {
	case air.TypeVoid, air.TypeInt, air.TypeByte, air.TypeRune, air.TypeFloat, air.TypeBool, air.TypeStr, air.TypeEnum:
		return true
	case air.TypeList:
		return l.jsonNativeCodecSafe(info.Elem, seen)
	case air.TypeMap:
		keyInfo := l.program.Types[info.Key-1]
		return keyInfo.Kind == air.TypeStr && l.jsonNativeCodecSafe(info.Value, seen)
	case air.TypeStruct:
		for _, field := range info.Fields {
			if !l.jsonNativeCodecSafe(field.Type, seen) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (l *lowerer) writeJSONDecodeTextHelper(b *strings.Builder, typeID air.TypeID) {
	if !validTypeID(l.program, typeID) {
		return
	}
	info := l.program.Types[typeID-1]
	typeName := l.jsonParseGoTypeName(typeID)
	helper := l.jsonDecodeTextHelperName(typeID)
	fmt.Fprintf(b, "\nfunc %s(dec *jsontext.Decoder, path string) (%s, string) {\n", helper, typeName)
	fmt.Fprintf(b, "\tvar out %s\n\t_ = out\n", typeName)
	switch info.Kind {
	case air.TypeInt:
		fmt.Fprintf(b, "\treturn ardJSONDecodeInt(dec, path)\n")
	case air.TypeByte:
		fmt.Fprintf(b, "\tparsed, message := ardJSONDecodeInt(dec, path)\n\tif message != \"\" { return out, message }\n\tif parsed < 0 || parsed > 255 { return out, path + \": got Number, expected Byte\" }\n\treturn byte(parsed), \"\"\n")
	case air.TypeRune:
		fmt.Fprintf(b, "\tparsed, message := ardJSONDecodeInt(dec, path)\n\tif message != \"\" { return out, message }\n\tif parsed < 0 || parsed > 0x10FFFF || (parsed >= 0xD800 && parsed <= 0xDFFF) { return out, path + \": got Number, expected Rune\" }\n\treturn rune(parsed), \"\"\n")
	case air.TypeFloat:
		fmt.Fprintf(b, "\treturn ardJSONDecodeFloat(dec, path)\n")
	case air.TypeBool:
		fmt.Fprintf(b, "\treturn ardJSONDecodeBool(dec, path)\n")
	case air.TypeStr:
		fmt.Fprintf(b, "\treturn ardJSONDecodeString(dec, path)\n")
	case air.TypeDynamic:
		fmt.Fprintf(b, "\treturn ardJSONDecodeDynamic(dec, path)\n")
	case air.TypeMaybe:
		fmt.Fprintf(b, "\treturn ardJSONDecodeMaybe(dec, path, %s)\n", l.jsonDecodeTextHelperName(info.Elem))
	case air.TypeList:
		if l.typeKind(info.Elem) == air.TypeByte {
			fmt.Fprintf(b, "\treturn ardJSONDecodeByteList(dec, path, %q)\n", info.Name)
		} else {
			fmt.Fprintf(b, "\treturn ardJSONDecodeList(dec, path, %q, %s)\n", info.Name, l.jsonDecodeTextHelperName(info.Elem))
		}
	case air.TypeMap:
		fmt.Fprintf(b, "\treturn ardJSONDecodeStringMap(dec, path, %q, %s)\n", info.Name, l.jsonDecodeTextHelperName(info.Value))
	case air.TypeStruct:
		fmt.Fprintf(b, "\ttok, err := dec.ReadToken()\n\tif err != nil { return out, err.Error() }\n\tif tok.Kind() != jsontext.KindBeginObject { return out, path + \": got \" + tok.Kind().String() + \", expected %s\" }\n", info.Name)
		for _, field := range info.Fields {
			fieldInfo := l.program.Types[field.Type-1]
			if fieldInfo.Kind != air.TypeMaybe {
				fmt.Fprintf(b, "\tseen%s := false\n", exportedFieldName(field.Name))
			}
		}
		fmt.Fprintf(b, "\tfor dec.PeekKind() != jsontext.KindEndObject {\n\t\tkeyTok, err := dec.ReadToken()\n\t\tif err != nil { return out, err.Error() }\n\t\tkey := keyTok.String()\n\t\tswitch key {\n")
		for _, field := range info.Fields {
			fieldInfo := l.program.Types[field.Type-1]
			fmt.Fprintf(b, "\t\tcase %q:\n\t\t\tfieldPath := key\n\t\t\tif path != \"\" { fieldPath = ardJSONPath(path, key) }\n\t\t\tvalue, message := %s(dec, fieldPath)\n\t\t\tif message != \"\" { return out, message }\n\t\t\tout.%s = value\n", field.Name, l.jsonDecodeTextHelperName(field.Type), l.goFieldName(info, field.Name))
			if fieldInfo.Kind != air.TypeMaybe {
				fmt.Fprintf(b, "\t\t\tseen%s = true\n", exportedFieldName(field.Name))
			}
		}
		fmt.Fprintf(b, "\t\tdefault:\n\t\t\tif err := dec.SkipValue(); err != nil { return out, err.Error() }\n\t\t}\n\t}\n\t_, err = dec.ReadToken()\n\tif err != nil { return out, err.Error() }\n")
		for _, field := range info.Fields {
			fieldInfo := l.program.Types[field.Type-1]
			if fieldInfo.Kind != air.TypeMaybe {
				fmt.Fprintf(b, "\tif !seen%s { return out, ardJSONMissing(ardJSONPath(path, %q), %q) }\n", exportedFieldName(field.Name), field.Name, fieldInfo.Name)
			}
		}
		fmt.Fprintf(b, "\treturn out, \"\"\n")
	default:
		fmt.Fprintf(b, "\treturn out, ardJSONErr(path, %q, nil)\n", info.Name)
	}
	fmt.Fprintf(b, "}\n")
	if info.Kind == air.TypeStruct {
		fmt.Fprintf(b, "\nfunc (out *%s) UnmarshalJSONFrom(dec *jsontext.Decoder) error {\n\tparsed, message := %s(dec, \"\")\n\tif message != \"\" { return fmt.Errorf(\"%%s\", message) }\n\t*out = parsed\n\treturn nil\n}\n", typeName, helper)
	}
}

// writeJSONDecodePrimitiveListLoop emits specialized loops for primitive JSON arrays.
// The generic element-helper path eagerly constructs item paths with fmt.Sprintf
// for every successful element. Primitive lists are common and can validate tokens
// directly, keeping detailed item paths only on error. A small default capacity
// avoids repeated growth for typical short JSON arrays while preserving [] for
// empty arrays instead of a nil slice.
func (l *lowerer) writeJSONDecodePrimitiveListLoop(b *strings.Builder, elemTypeID air.TypeID, listTypeName string) bool {
	if !validTypeID(l.program, elemTypeID) {
		return false
	}
	elemInfo := l.program.Types[elemTypeID-1]
	switch elemInfo.Kind {
	case air.TypeInt:
		fmt.Fprintf(b, "\tout = make(%s, 0, 8)\n", listTypeName)
		fmt.Fprintf(b, "\tfor i := 0; dec.PeekKind() != jsontext.KindEndArray; i++ {\n\t\ttok, err := dec.ReadToken()\n\t\tif err != nil { return out, err.Error() }\n\t\tif tok.Kind() != jsontext.KindNumber { itemPath := ardJSONPath(path, fmt.Sprintf(\"[%%d]\", i)); return out, itemPath + \": got \" + tok.Kind().String() + \", expected Int\" }\n\t\tparsed, err := strconv.Atoi(tok.String())\n\t\tif err != nil { itemPath := ardJSONPath(path, fmt.Sprintf(\"[%%d]\", i)); return out, itemPath + \": got Number, expected Int\" }\n\t\tout = append(out, parsed)\n\t}\n")
		return true
	case air.TypeFloat:
		fmt.Fprintf(b, "\tout = make(%s, 0, 8)\n", listTypeName)
		fmt.Fprintf(b, "\tfor i := 0; dec.PeekKind() != jsontext.KindEndArray; i++ {\n\t\ttok, err := dec.ReadToken()\n\t\tif err != nil { return out, err.Error() }\n\t\tif tok.Kind() != jsontext.KindNumber { itemPath := ardJSONPath(path, fmt.Sprintf(\"[%%d]\", i)); return out, itemPath + \": got \" + tok.Kind().String() + \", expected Float\" }\n\t\tparsed, err := strconv.ParseFloat(tok.String(), 64)\n\t\tif err != nil { itemPath := ardJSONPath(path, fmt.Sprintf(\"[%%d]\", i)); return out, itemPath + \": got Number, expected Float\" }\n\t\tout = append(out, parsed)\n\t}\n")
		return true
	case air.TypeBool:
		fmt.Fprintf(b, "\tout = make(%s, 0, 8)\n", listTypeName)
		fmt.Fprintf(b, "\tfor i := 0; dec.PeekKind() != jsontext.KindEndArray; i++ {\n\t\ttok, err := dec.ReadToken()\n\t\tif err != nil { return out, err.Error() }\n\t\tif tok.Kind() == jsontext.KindTrue { out = append(out, true); continue }\n\t\tif tok.Kind() == jsontext.KindFalse { out = append(out, false); continue }\n\t\titemPath := ardJSONPath(path, fmt.Sprintf(\"[%%d]\", i)); return out, itemPath + \": got \" + tok.Kind().String() + \", expected Bool\"\n\t}\n")
		return true
	case air.TypeStr:
		fmt.Fprintf(b, "\tout = make(%s, 0, 8)\n", listTypeName)
		fmt.Fprintf(b, "\tfor i := 0; dec.PeekKind() != jsontext.KindEndArray; i++ {\n\t\ttok, err := dec.ReadToken()\n\t\tif err != nil { return out, err.Error() }\n\t\tif tok.Kind() != jsontext.KindString { itemPath := ardJSONPath(path, fmt.Sprintf(\"[%%d]\", i)); return out, itemPath + \": got \" + tok.Kind().String() + \", expected Str\" }\n\t\tout = append(out, tok.String())\n\t}\n")
		return true
	default:
		return false
	}
}

func (l *lowerer) jsonDecodeTextHelperName(typeID air.TypeID) string {
	return fmt.Sprintf("ardJSONDecodeText_%d", typeID)
}
