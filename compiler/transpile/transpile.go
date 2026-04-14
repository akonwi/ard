package transpile

import (
	"fmt"
	gofmt "go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const (
	generatedGoVersion = "1.26.0"
	ardModulePath      = "github.com/akonwi/ard"
	helperImportPath   = ardModulePath + "/go"
	helperImportAlias  = "ardgo"
	stringsImportPath  = "strings"
	strconvImportPath  = "strconv"
	sortImportPath     = "sort"
)

type emitter struct {
	module        checker.Module
	packageName   string
	entrypoint    bool
	imports       map[string]string
	functionNames map[string]string
	emittedTypes  map[string]struct{}
	builder       strings.Builder
	indent        int
	tempCounter   int
	fnReturnType  checker.Type
}

func (e *emitter) nextTemp(prefix string) string {
	name := fmt.Sprintf("__ard%s%d", prefix, e.tempCounter)
	e.tempCounter++
	return name
}

func EmitEntrypoint(module checker.Module) ([]byte, error) {
	return emitModuleSource(module, "main", true)
}

func emitPackageSource(module checker.Module) ([]byte, error) {
	return emitModuleSource(module, packageNameForModulePath(module.Path()), false)
}

func emitModuleSource(module checker.Module, packageName string, entrypoint bool) ([]byte, error) {
	if module == nil || module.Program() == nil {
		return nil, fmt.Errorf("module has no program")
	}

	e := &emitter{
		module:        module,
		packageName:   packageName,
		entrypoint:    entrypoint,
		imports:       collectModuleImports(module.Program().Statements),
		functionNames: make(map[string]string),
		emittedTypes:  make(map[string]struct{}),
	}
	e.indexFunctions()
	if entrypoint {
		e.line("package main")
	} else {
		e.line("package " + packageName)
	}
	if len(e.imports) > 0 {
		e.line("")
		e.line("import (")
		e.indent++
		paths := sortedImportPaths(e.imports)
		for _, path := range paths {
			e.line(fmt.Sprintf("%s %q", e.imports[path], path))
		}
		e.indent--
		e.line(")")
	}
	e.line("")

	for _, stmt := range module.Program().Statements {
		if stmt.Stmt == nil {
			continue
		}
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			if err := e.emitStructDef(def); err != nil {
				return nil, err
			}
			e.line("")
		case *checker.Enum:
			if err := e.emitEnumDef(def); err != nil {
				return nil, err
			}
			e.line("")
		case *checker.VariableDef:
			if entrypoint {
				continue
			}
			if err := e.emitPackageVariable(def); err != nil {
				return nil, err
			}
			e.line("")
		default:
			if !entrypoint {
				return nil, fmt.Errorf("unsupported top-level statement in imported module: %T", stmt.Stmt)
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
			if err := e.emitFunction(def); err != nil {
				return nil, err
			}
			e.line("")
		case *checker.ExternalFunctionDef:
			if err := e.emitExternFunction(def); err != nil {
				return nil, err
			}
			e.line("")
		}
	}

	if entrypoint {
		e.line("func main() {")
		e.indent++
		if err := e.emitStatements(topLevelExecutableStatements(module.Program().Statements), nil); err != nil {
			return nil, err
		}
		e.indent--
		e.line("}")
	}

	formatted, err := gofmt.Source([]byte(e.builder.String()))
	if err != nil {
		return nil, fmt.Errorf("format generated go: %w\n%s", err, e.builder.String())
	}
	return formatted, nil
}

func BuildBinary(inputPath, outputPath string) (string, error) {
	module, project, err := loadModule(inputPath)
	if err != nil {
		return "", err
	}

	generatedDir := filepath.Join(project.RootPath, "generated")
	if err := writeGeneratedProject(generatedDir, project, module); err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-o", outputPath, ".")
	cmd.Dir = generatedDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return outputPath, nil
}

func Run(inputPath string, args []string) error {
	module, project, err := loadModule(inputPath)
	if err != nil {
		return err
	}
	_ = args

	generatedDir := filepath.Join(project.RootPath, "generated")
	if err := writeGeneratedProject(generatedDir, project, module); err != nil {
		return err
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = generatedDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func writeGeneratedProject(generatedDir string, project *checker.ProjectInfo, entrypoint checker.Module) error {
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return err
	}
	moduleRoot, err := compilerModuleRoot()
	if err != nil {
		return err
	}
	goMod := fmt.Sprintf("module %s\n\ngo %s\n\nrequire %s v0.0.0\n\nreplace %s => %s\n", project.ProjectName, generatedGoVersion, ardModulePath, ardModulePath, filepath.Clean(moduleRoot))
	if err := os.WriteFile(filepath.Join(generatedDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}

	source, err := EmitEntrypoint(entrypoint)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "main.go"), source, 0o644); err != nil {
		return err
	}

	written := map[string]struct{}{}
	for _, mod := range sortedModules(entrypoint.Program().Imports) {
		if err := writeImportedModule(generatedDir, project.ProjectName, mod, written); err != nil {
			return err
		}
	}
	return nil
}

func writeImportedModule(generatedDir, projectName string, module checker.Module, written map[string]struct{}) error {
	if module == nil {
		return nil
	}
	if strings.HasPrefix(module.Path(), "ard/") {
		return nil
	}
	if _, ok := written[module.Path()]; ok {
		return nil
	}
	written[module.Path()] = struct{}{}

	source, err := emitPackageSource(module)
	if err != nil {
		return err
	}
	outputPath, err := generatedPathForModule(generatedDir, projectName, module.Path())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, source, 0o644); err != nil {
		return err
	}
	for _, mod := range sortedModules(module.Program().Imports) {
		if err := writeImportedModule(generatedDir, projectName, mod, written); err != nil {
			return err
		}
	}
	return nil
}

func loadModule(inputPath string) (checker.Module, *checker.ProjectInfo, error) {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading file %s - %v", inputPath, err)
	}

	result := parse.Parse(sourceCode, inputPath)
	if len(result.Errors) > 0 {
		result.PrintErrors()
		return nil, nil, fmt.Errorf("parse errors")
	}
	program := result.Program

	workingDir := filepath.Dir(inputPath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil, nil, fmt.Errorf("error initializing module resolver: %w", err)
	}

	relPath, err := filepath.Rel(workingDir, inputPath)
	if err != nil {
		relPath = inputPath
	}

	c := checker.New(relPath, program, moduleResolver)
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			fmt.Println(diagnostic)
		}
		return nil, nil, fmt.Errorf("type errors")
	}

	return c.Module(), moduleResolver.GetProjectInfo(), nil
}

func topLevelExecutableStatements(stmts []checker.Statement) []checker.Statement {
	filtered := make([]checker.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		switch stmt.Expr.(type) {
		case *checker.FunctionDef, *checker.ExternalFunctionDef:
			continue
		}
		switch stmt.Stmt.(type) {
		case *checker.StructDef, *checker.Enum:
			continue
		}
		filtered = append(filtered, stmt)
	}
	return filtered
}

func collectModuleImports(stmts []checker.Statement) map[string]string {
	imports := make(map[string]string)
	for _, stmt := range stmts {
		collectImportsFromStatement(stmt, imports)
	}
	return imports
}

func collectImportsFromStatement(stmt checker.Statement, imports map[string]string) {
	if extern, ok := stmt.Expr.(*checker.ExternalFunctionDef); ok {
		imports[helperImportPath] = helperImportAlias
		for _, param := range extern.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(extern.ReturnType, imports)
	}
	if stmt.Expr != nil {
		collectImportsFromExpr(stmt.Expr, imports)
		collectImportsFromExprTypes(stmt.Expr, imports)
	}
	if stmt.Stmt != nil {
		collectImportsFromNonProducing(stmt.Stmt, imports)
		collectImportsFromStmtTypes(stmt.Stmt, imports)
	}
}

func collectImportsFromExprTypes(expr checker.Expression, imports map[string]string) {
	collectImportsFromType(expr.Type(), imports)
	if fn, ok := expr.(*checker.FunctionDef); ok {
		for _, param := range fn.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(fn.ReturnType, imports)
	}
}

func collectImportsFromStmtTypes(stmt checker.NonProducing, imports map[string]string) {
	switch s := stmt.(type) {
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(s.Fields) {
			collectImportsFromType(s.Fields[fieldName], imports)
		}
	case *checker.VariableDef:
		collectImportsFromType(s.Type(), imports)
	}
}

func collectImportsFromType(t checker.Type, imports map[string]string) {
	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			collectImportsFromType(actual, imports)
		}
	case *checker.Result:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromType(typed.Val(), imports)
		collectImportsFromType(typed.Err(), imports)
	case *checker.Maybe:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromType(typed.Of(), imports)
	case *checker.List:
		collectImportsFromType(typed.Of(), imports)
	case *checker.Map:
		collectImportsFromType(typed.Key(), imports)
		collectImportsFromType(typed.Value(), imports)
	case *checker.StructDef:
		for _, fieldName := range sortedStringKeys(typed.Fields) {
			collectImportsFromType(typed.Fields[fieldName], imports)
		}
	}
}

func collectImportsFromNonProducing(stmt checker.NonProducing, imports map[string]string) {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		collectImportsFromExpr(s.Value, imports)
	case *checker.Reassignment:
		collectImportsFromExpr(s.Target, imports)
		collectImportsFromExpr(s.Value, imports)
	case *checker.WhileLoop:
		collectImportsFromExpr(s.Condition, imports)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.ForLoop:
		collectImportsFromExpr(s.Init.Value, imports)
		collectImportsFromExpr(s.Condition, imports)
		collectImportsFromExpr(s.Update.Value, imports)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.ForIntRange:
		collectImportsFromExpr(s.Start, imports)
		collectImportsFromExpr(s.End, imports)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.ForInStr:
		collectImportsFromExpr(s.Value, imports)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.ForInList:
		collectImportsFromExpr(s.List, imports)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.ForInMap:
		collectImportsFromExpr(s.Map, imports)
		for _, stmt := range s.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	}
}

func collectImportsFromExpr(expr checker.Expression, imports map[string]string) {
	switch v := expr.(type) {
	case *checker.IntAddition:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntSubtraction:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntMultiplication:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntDivision:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntModulo:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatAddition:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatSubtraction:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatMultiplication:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatDivision:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.StrAddition:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntGreater:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntGreaterEqual:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntLess:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.IntLessEqual:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatGreater:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatGreaterEqual:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatLess:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.FloatLessEqual:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.Equality:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.And:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.Or:
		collectImportsFromExpr(v.Left, imports)
		collectImportsFromExpr(v.Right, imports)
	case *checker.Negation:
		collectImportsFromExpr(v.Value, imports)
	case *checker.Not:
		collectImportsFromExpr(v.Value, imports)
	case *checker.FunctionCall:
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports)
		}
	case *checker.StructInstance:
		for _, fieldName := range sortedStringKeys(v.Fields) {
			collectImportsFromExpr(v.Fields[fieldName], imports)
		}
	case *checker.InstanceProperty:
		collectImportsFromExpr(v.Subject, imports)
	case *checker.InstanceMethod:
		collectImportsFromExpr(v.Subject, imports)
		for _, arg := range v.Method.Args {
			collectImportsFromExpr(arg, imports)
		}
	case *checker.CopyExpression:
		collectImportsFromExpr(v.Expr, imports)
	case *checker.ListLiteral:
		for _, element := range v.Elements {
			collectImportsFromExpr(element, imports)
		}
	case *checker.ListMethod:
		collectImportsFromExpr(v.Subject, imports)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports)
		}
		if v.Kind == checker.ListSort {
			imports[sortImportPath] = "sort"
		}
	case *checker.MapLiteral:
		for i := range v.Keys {
			collectImportsFromExpr(v.Keys[i], imports)
			collectImportsFromExpr(v.Values[i], imports)
		}
	case *checker.MapMethod:
		collectImportsFromExpr(v.Subject, imports)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports)
		}
	case *checker.ResultMethod:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Subject, imports)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports)
		}
	case *checker.TryOp:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Expr(), imports)
		collectImportsFromType(v.OkType, imports)
		collectImportsFromType(v.ErrType, imports)
		if v.CatchBlock != nil {
			for _, stmt := range v.CatchBlock.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
	case *checker.MaybeMethod:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Subject, imports)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports)
		}
	case *checker.TemplateStr:
		for _, chunk := range v.Chunks {
			collectImportsFromExpr(chunk, imports)
		}
	case *checker.StrMethod:
		collectImportsFromExpr(v.Subject, imports)
		for _, arg := range v.Args {
			collectImportsFromExpr(arg, imports)
		}
		switch v.Kind {
		case checker.StrContains, checker.StrReplace, checker.StrReplaceAll, checker.StrSplit, checker.StrStartsWith, checker.StrTrim:
			imports[stringsImportPath] = "strings"
		}
	case *checker.IntMethod:
		collectImportsFromExpr(v.Subject, imports)
		if v.Kind == checker.IntToStr {
			imports[strconvImportPath] = "strconv"
		}
	case *checker.FloatMethod:
		collectImportsFromExpr(v.Subject, imports)
		if v.Kind == checker.FloatToStr {
			imports[strconvImportPath] = "strconv"
		}
	case *checker.BoolMethod:
		collectImportsFromExpr(v.Subject, imports)
		if v.Kind == checker.BoolToStr {
			imports[strconvImportPath] = "strconv"
		}
	case *checker.ModuleStructInstance:
		if !strings.HasPrefix(v.Module, "ard/") {
			imports[v.Module] = packageNameForModulePath(v.Module)
		}
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			collectImportsFromExpr(v.Property.Fields[fieldName], imports)
		}
	case *checker.ModuleFunctionCall:
		if v.Module == "ard/maybe" || v.Module == "ard/result" {
			imports[helperImportPath] = helperImportAlias
		} else if !strings.HasPrefix(v.Module, "ard/") {
			imports[v.Module] = packageNameForModulePath(v.Module)
		}
		for _, arg := range v.Call.Args {
			collectImportsFromExpr(arg, imports)
		}
	case *checker.ModuleSymbol:
		if !strings.HasPrefix(v.Module, "ard/") {
			imports[v.Module] = packageNameForModulePath(v.Module)
		}
	case *checker.If:
		collectImportsFromExpr(v.Condition, imports)
		for _, stmt := range v.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
		if v.ElseIf != nil {
			collectImportsFromExpr(v.ElseIf, imports)
		}
		if v.Else != nil {
			for _, stmt := range v.Else.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
	case *checker.BoolMatch:
		collectImportsFromExpr(v.Subject, imports)
		for _, stmt := range v.True.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
		for _, stmt := range v.False.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.IntMatch:
		collectImportsFromExpr(v.Subject, imports)
		for _, block := range v.IntCases {
			for _, stmt := range block.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
		for _, block := range v.RangeCases {
			for _, stmt := range block.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
		if v.CatchAll != nil {
			for _, stmt := range v.CatchAll.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
	case *checker.EnumMatch:
		collectImportsFromExpr(v.Subject, imports)
		for _, block := range v.Cases {
			if block == nil {
				continue
			}
			for _, stmt := range block.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
		if v.CatchAll != nil {
			for _, stmt := range v.CatchAll.Stmts {
				collectImportsFromStatement(stmt, imports)
			}
		}
	case *checker.OptionMatch:
		collectImportsFromExpr(v.Subject, imports)
		for _, stmt := range v.Some.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
		for _, stmt := range v.None.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.ResultMatch:
		imports[helperImportPath] = helperImportAlias
		collectImportsFromExpr(v.Subject, imports)
		for _, stmt := range v.Ok.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
		for _, stmt := range v.Err.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	case *checker.FunctionDef:
		for _, param := range v.Parameters {
			collectImportsFromType(param.Type, imports)
		}
		collectImportsFromType(effectiveFunctionReturnType(v), imports)
		for _, stmt := range v.Body.Stmts {
			collectImportsFromStatement(stmt, imports)
		}
	}
}

func sortedImportPaths(imports map[string]string) []string {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func sortedModules(imports map[string]checker.Module) []checker.Module {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	modules := make([]checker.Module, 0, len(paths))
	for _, path := range paths {
		modules = append(modules, imports[path])
	}
	return modules
}

func sortedIntKeys[T any](values map[int]T) []int {
	keys := make([]int, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func sortedIntRanges(values map[checker.IntRange]*checker.Block) []checker.IntRange {
	ranges := make([]checker.IntRange, 0, len(values))
	for key := range values {
		ranges = append(ranges, key)
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start != ranges[j].Start {
			return ranges[i].Start < ranges[j].Start
		}
		return ranges[i].End < ranges[j].End
	})
	return ranges
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func packageNameForModulePath(modulePath string) string {
	base := filepath.Base(strings.TrimSuffix(modulePath, ".ard"))
	return goName(base, false)
}

func generatedPathForModule(generatedDir, projectName, modulePath string) (string, error) {
	prefix := projectName + "/"
	if !strings.HasPrefix(modulePath, prefix) {
		return "", fmt.Errorf("module path %q does not match project %q", modulePath, projectName)
	}
	relative := strings.TrimPrefix(modulePath, prefix)
	dir := filepath.Join(generatedDir, filepath.FromSlash(relative))
	base := filepath.Base(relative)
	return filepath.Join(dir, base+".go"), nil
}

func (e *emitter) indexFunctions() {
	for _, stmt := range e.module.Program().Statements {
		switch def := stmt.Expr.(type) {
		case *checker.FunctionDef:
			name := goName(def.Name, !def.Private)
			if e.packageName == "main" && name == "main" {
				name = "ardMain"
			}
			e.functionNames[def.Name] = name
		case *checker.ExternalFunctionDef:
			name := goName(def.Name, !def.Private)
			if e.packageName == "main" && name == "main" {
				name = "ardMain"
			}
			e.functionNames[def.Name] = name
		}
	}
}

func (e *emitter) emitStructDef(def *checker.StructDef) error {
	if _, ok := e.emittedTypes["struct:"+def.Name]; ok {
		return nil
	}
	e.emittedTypes["struct:"+def.Name] = struct{}{}
	e.line("type " + goName(def.Name, true) + " struct {")
	e.indent++
	fieldNames := sortedStringKeys(def.Fields)
	for _, fieldName := range fieldNames {
		typeName, err := emitType(def.Fields[fieldName])
		if err != nil {
			return fmt.Errorf("struct %s: %w", def.Name, err)
		}
		e.line(fmt.Sprintf("%s %s", goName(fieldName, true), typeName))
	}
	e.indent--
	e.line("}")
	methodNames := sortedStringKeys(def.Methods)
	for _, methodName := range methodNames {
		e.line("")
		if err := e.emitStructMethod(def, def.Methods[methodName]); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitStructMethod(def *checker.StructDef, method *checker.FunctionDef) error {
	params := make([]string, 0, len(method.Parameters))
	for _, param := range method.Parameters {
		typeName, err := emitType(param.Type)
		if err != nil {
			return fmt.Errorf("method %s.%s: %w", def.Name, method.Name, err)
		}
		params = append(params, fmt.Sprintf("%s %s", goName(param.Name, false), typeName))
	}
	receiverType := goName(def.Name, true)
	if method.Mutates {
		receiverType = "*" + receiverType
	}
	receiverName := goName(method.Receiver, false)
	name := goName(method.Name, !method.Private)
	signature := fmt.Sprintf("func (%s %s) %s(%s)", receiverName, receiverType, name, strings.Join(params, ", "))
	if method.ReturnType != checker.Void {
		returnType, err := emitType(method.ReturnType)
		if err != nil {
			return fmt.Errorf("method %s.%s: %w", def.Name, method.Name, err)
		}
		signature += " " + returnType
	}
	e.line(signature + " {")
	e.indent++
	if err := e.emitStatements(method.Body.Stmts, method.ReturnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitValueForType(expr checker.Expression, expectedType checker.Type) (string, error) {
	if call, ok := expr.(*checker.ModuleFunctionCall); ok {
		switch call.Module {
		case "ard/maybe":
			return e.emitMaybeModuleCall(call, expectedType)
		case "ard/result":
			return e.emitResultModuleCall(call, expectedType)
		}
	}
	return e.emitExpr(expr)
}

func typeNeedsExplicitVarAnnotation(t checker.Type) bool {
	switch t.(type) {
	case *checker.Maybe:
		return true
	default:
		return false
	}
}

func emitCopiedValue(value string, t checker.Type) (string, error) {
	switch typed := t.(type) {
	case *checker.List:
		typeName, err := emitType(typed)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("append(%s(nil), %s...)", typeName, value), nil
	default:
		return value, nil
	}
}

func (e *emitter) emitEnumDef(def *checker.Enum) error {
	if _, ok := e.emittedTypes["enum:"+def.Name]; ok {
		return nil
	}
	e.emittedTypes["enum:"+def.Name] = struct{}{}
	e.line("type " + goName(def.Name, true) + " struct {")
	e.indent++
	e.line("Tag int")
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitPackageVariable(def *checker.VariableDef) error {
	value, err := e.emitValueForType(def.Value, def.Type())
	if err != nil {
		return err
	}
	name := goName(def.Name, !def.Mutable)
	if !typeNeedsExplicitVarAnnotation(def.Type()) {
		e.line(fmt.Sprintf("var %s = %s", name, value))
		return nil
	}
	typeName, err := emitType(def.Type())
	if err != nil || typeName == "" {
		e.line(fmt.Sprintf("var %s = %s", name, value))
		return nil
	}
	e.line(fmt.Sprintf("var %s %s = %s", name, typeName, value))
	return nil
}

func isFunctionLiteralDef(def *checker.FunctionDef) bool {
	if def == nil {
		return false
	}
	return strings.HasPrefix(def.Name, "anon_func_") || strings.HasPrefix(def.Name, "start_func_") || strings.HasPrefix(def.Name, "eval_func_")
}

func effectiveFunctionReturnType(def *checker.FunctionDef) checker.Type {
	if def == nil {
		return checker.Void
	}
	if isFunctionLiteralDef(def) && def.ReturnType == checker.Void && def.Body != nil && def.Body.Type() != checker.Void {
		return def.Body.Type()
	}
	return def.ReturnType
}

func emitFunctionParams(params []checker.Parameter, includeNames bool) ([]string, error) {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		typeName, err := emitType(param.Type)
		if err != nil {
			return nil, err
		}
		if includeNames {
			parts = append(parts, fmt.Sprintf("%s %s", goName(param.Name, false), typeName))
		} else {
			parts = append(parts, typeName)
		}
	}
	return parts, nil
}

func emitFunctionType(def *checker.FunctionDef) (string, error) {
	params, err := emitFunctionParams(def.Parameters, false)
	if err != nil {
		return "", err
	}
	typeName := fmt.Sprintf("func(%s)", strings.Join(params, ", "))
	returnType := effectiveFunctionReturnType(def)
	if returnType != checker.Void {
		emittedReturnType, err := emitType(returnType)
		if err != nil {
			return "", err
		}
		typeName += " " + emittedReturnType
	}
	return typeName, nil
}

func (e *emitter) emitFunctionLiteral(def *checker.FunctionDef) (string, error) {
	params, err := emitFunctionParams(def.Parameters, true)
	if err != nil {
		return "", err
	}
	returnType := effectiveFunctionReturnType(def)
	inner := &emitter{
		module:        e.module,
		packageName:   e.packageName,
		entrypoint:    e.entrypoint,
		imports:       e.imports,
		functionNames: e.functionNames,
		emittedTypes:  e.emittedTypes,
		indent:        1,
		tempCounter:   e.tempCounter,
		fnReturnType:  returnType,
	}
	if err := inner.emitStatements(def.Body.Stmts, returnType); err != nil {
		return "", err
	}
	e.tempCounter = inner.tempCounter
	var builder strings.Builder
	builder.WriteString("func(")
	builder.WriteString(strings.Join(params, ", "))
	builder.WriteString(")")
	if returnType != checker.Void {
		emittedReturnType, err := emitType(returnType)
		if err != nil {
			return "", err
		}
		builder.WriteString(" ")
		builder.WriteString(emittedReturnType)
	}
	builder.WriteString(" {\n")
	builder.WriteString(inner.builder.String())
	builder.WriteString("}")
	return builder.String(), nil
}

func (e *emitter) emitExternFunction(def *checker.ExternalFunctionDef) error {
	params := make([]string, 0, len(def.Parameters))
	args := make([]string, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		typeName, err := emitType(param.Type)
		if err != nil {
			return fmt.Errorf("extern function %s: %w", def.Name, err)
		}
		paramName := goName(param.Name, false)
		params = append(params, fmt.Sprintf("%s %s", paramName, typeName))
		args = append(args, paramName)
	}

	name := e.functionNames[def.Name]
	signature := fmt.Sprintf("func %s(%s)", name, strings.Join(params, ", "))
	returnType := ""
	if def.ReturnType != checker.Void {
		var err error
		returnType, err = emitType(def.ReturnType)
		if err != nil {
			return fmt.Errorf("extern function %s: %w", def.Name, err)
		}
		signature += " " + returnType
	}

	e.line(signature + " {")
	e.indent++
	call := fmt.Sprintf("%s.CallExtern(%q", helperImportAlias, def.ExternalBinding)
	if len(args) > 0 {
		call += ", " + strings.Join(args, ", ")
	}
	call += ")"
	e.line("result, err := " + call)
	e.line("if err != nil {")
	e.indent++
	e.line("panic(err)")
	e.indent--
	e.line("}")
	if def.ReturnType != checker.Void {
		e.line(fmt.Sprintf("return result.(%s)", returnType))
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitFunction(def *checker.FunctionDef) error {
	params, err := emitFunctionParams(def.Parameters, true)
	if err != nil {
		return fmt.Errorf("function %s: %w", def.Name, err)
	}

	returnType := effectiveFunctionReturnType(def)
	signature := fmt.Sprintf("func %s(%s)", e.functionNames[def.Name], strings.Join(params, ", "))
	if returnType != checker.Void {
		emittedReturnType, err := emitType(returnType)
		if err != nil {
			return fmt.Errorf("function %s: %w", def.Name, err)
		}
		signature += " " + emittedReturnType
	}
	e.line(signature + " {")
	e.indent++
	prevReturnType := e.fnReturnType
	e.fnReturnType = returnType
	defer func() {
		e.fnReturnType = prevReturnType
	}()
	if err := e.emitStatements(def.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitStatements(stmts []checker.Statement, returnType checker.Type) error {
	lastMeaningful := lastMeaningfulStatementIndex(stmts)
	for i, stmt := range stmts {
		if stmt.Break {
			e.line("break")
			continue
		}
		isLastExpr := i == lastMeaningful && stmt.Expr != nil
		remaining := stmts[i+1:]
		if stmt.Stmt != nil {
			if err := e.emitNonProducing(stmt.Stmt, remaining, returnType); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if err := e.emitExpressionStatement(stmt.Expr, returnType, isLastExpr); err != nil {
			return err
		}
	}
	return nil
}

func lastMeaningfulStatementIndex(stmts []checker.Statement) int {
	for i := len(stmts) - 1; i >= 0; i-- {
		if stmts[i].Break || stmts[i].Expr != nil || stmts[i].Stmt != nil {
			return i
		}
	}
	return -1
}

func (e *emitter) emitNonProducing(stmt checker.NonProducing, remaining []checker.Statement, returnType checker.Type) error {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		name := goName(s.Name, false)
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			if err := e.emitTryOp(tryOp, returnType, func(successValue string) error {
				if typeNeedsExplicitVarAnnotation(s.Type()) {
					typeName, err := emitType(s.Type())
					if err != nil || typeName == "" {
						e.line(fmt.Sprintf("%s := %s", name, successValue))
					} else {
						e.line(fmt.Sprintf("var %s %s = %s", name, typeName, successValue))
					}
				} else {
					e.line(fmt.Sprintf("%s := %s", name, successValue))
				}
				return nil
			}, nil); err != nil {
				return err
			}
			if !usesNameInStatements(remaining, s.Name) {
				e.line(fmt.Sprintf("_ = %s", name))
			}
			return nil
		}
		value, err := e.emitValueForType(s.Value, s.Type())
		if err != nil {
			return err
		}
		if typeNeedsExplicitVarAnnotation(s.Type()) {
			typeName, err := emitType(s.Type())
			if err != nil || typeName == "" {
				e.line(fmt.Sprintf("%s := %s", name, value))
			} else {
				e.line(fmt.Sprintf("var %s %s = %s", name, typeName, value))
			}
		} else {
			e.line(fmt.Sprintf("%s := %s", name, value))
		}
		if !usesNameInStatements(remaining, s.Name) {
			e.line(fmt.Sprintf("_ = %s", name))
		}
		return nil
	case *checker.Reassignment:
		targetName, err := emitAssignmentTarget(s.Target)
		if err != nil {
			return err
		}
		if tryOp, ok := s.Value.(*checker.TryOp); ok {
			return e.emitTryOp(tryOp, returnType, func(successValue string) error {
				e.line(fmt.Sprintf("%s = %s", targetName, successValue))
				return nil
			}, nil)
		}
		value, err := e.emitExpr(s.Value)
		if err != nil {
			return err
		}
		e.line(fmt.Sprintf("%s = %s", targetName, value))
		return nil
	case *checker.WhileLoop:
		return e.emitWhileLoop(s, returnType)
	case *checker.ForLoop:
		return e.emitForLoop(s, returnType)
	case *checker.ForIntRange:
		return e.emitForIntRange(s, returnType)
	case *checker.ForInStr:
		return e.emitForInStr(s, returnType)
	case *checker.ForInList:
		return e.emitForInList(s, returnType)
	case *checker.ForInMap:
		return e.emitForInMap(s, returnType)
	default:
		return fmt.Errorf("unsupported statement: %T", stmt)
	}
}

func (e *emitter) emitForIntRange(loop *checker.ForIntRange, returnType checker.Type) error {
	start, err := e.emitExpr(loop.Start)
	if err != nil {
		return err
	}
	end, err := e.emitExpr(loop.End)
	if err != nil {
		return err
	}
	cursor := goName(loop.Cursor, false)
	if loop.Index == "" {
		e.line(fmt.Sprintf("for %s := %s; %s <= %s; %s++ {", cursor, start, cursor, end, cursor))
	} else {
		index := goName(loop.Index, false)
		e.line(fmt.Sprintf("for %s, %s := %s, 0; %s <= %s; %s, %s = %s+1, %s+1 {", cursor, index, start, cursor, end, cursor, index, cursor, index))
	}
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForInStr(loop *checker.ForInStr, returnType checker.Type) error {
	value, err := e.emitExpr(loop.Value)
	if err != nil {
		return err
	}
	cursor := goName(loop.Cursor, false)
	indexName := "_"
	if loop.Index != "" {
		indexName = goName(loop.Index, false)
	}
	e.line(fmt.Sprintf("for %s, __ardRune := range []rune(%s) {", indexName, value))
	e.indent++
	e.line(fmt.Sprintf("%s := string(__ardRune)", cursor))
	if err := e.emitStatements(loop.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForInList(loop *checker.ForInList, returnType checker.Type) error {
	list, err := e.emitExpr(loop.List)
	if err != nil {
		return err
	}
	cursor := goName(loop.Cursor, false)
	indexName := "_"
	if loop.Index != "" {
		indexName = goName(loop.Index, false)
	}
	e.line(fmt.Sprintf("for %s, %s := range %s {", indexName, cursor, list))
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForInMap(loop *checker.ForInMap, returnType checker.Type) error {
	mapExpr, err := e.emitExpr(loop.Map)
	if err != nil {
		return err
	}
	key := goName(loop.Key, false)
	val := goName(loop.Val, false)
	e.line(fmt.Sprintf("for %s, %s := range %s {", key, val, mapExpr))
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitWhileLoop(loop *checker.WhileLoop, returnType checker.Type) error {
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	e.line("for " + condition + " {")
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForLoop(loop *checker.ForLoop, returnType checker.Type) error {
	if loop.Init == nil || loop.Update == nil {
		return fmt.Errorf("unsupported for loop: missing init or update")
	}
	initName := goName(loop.Init.Name, false)
	initValue, err := e.emitExpr(loop.Init.Value)
	if err != nil {
		return err
	}
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	updateTarget, err := emitAssignmentTarget(loop.Update.Target)
	if err != nil {
		return err
	}
	updateValue, err := e.emitExpr(loop.Update.Value)
	if err != nil {
		return err
	}
	e.line(fmt.Sprintf("for %s := %s; %s; %s = %s {", initName, initValue, condition, updateTarget, updateValue))
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, returnType); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func usesNameInStatements(stmts []checker.Statement, name string) bool {
	for _, stmt := range stmts {
		if variableDef, ok := stmt.Stmt.(*checker.VariableDef); ok && variableDef.Name == name {
			if usesNameInExpr(variableDef.Value, name) {
				return true
			}
			return false
		}
		if usesNameInStatement(stmt, name) {
			return true
		}
	}
	return false
}

func usesNameInStatement(stmt checker.Statement, name string) bool {
	if stmt.Expr != nil && usesNameInExpr(stmt.Expr, name) {
		return true
	}
	if stmt.Stmt != nil && usesNameInNonProducing(stmt.Stmt, name) {
		return true
	}
	return false
}

func usesNameInNonProducing(stmt checker.NonProducing, name string) bool {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		return usesNameInExpr(s.Value, name)
	case *checker.Reassignment:
		return usesNameInExpr(s.Target, name) || usesNameInExpr(s.Value, name)
	case *checker.WhileLoop:
		return usesNameInExpr(s.Condition, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForLoop:
		return usesNameInExpr(s.Init.Value, name) || usesNameInExpr(s.Condition, name) || usesNameInExpr(s.Update.Value, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForIntRange:
		return usesNameInExpr(s.Start, name) || usesNameInExpr(s.End, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForInStr:
		return usesNameInExpr(s.Value, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForInList:
		return usesNameInExpr(s.List, name) || usesNameInStatements(s.Body.Stmts, name)
	case *checker.ForInMap:
		return usesNameInExpr(s.Map, name) || usesNameInStatements(s.Body.Stmts, name)
	default:
		return false
	}
}

func usesNameInExpr(expr checker.Expression, name string) bool {
	switch v := expr.(type) {
	case nil:
		return false
	case *checker.Identifier:
		return v.Name == name
	case checker.Variable:
		return v.Name() == name
	case *checker.Variable:
		return v.Name() == name
	case *checker.IntLiteral, *checker.FloatLiteral, *checker.StrLiteral, *checker.BoolLiteral, *checker.VoidLiteral:
		return false
	case *checker.IntAddition:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntSubtraction:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntMultiplication:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntDivision:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntModulo:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatAddition:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatSubtraction:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatMultiplication:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatDivision:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.StrAddition:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntGreater:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntGreaterEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntLess:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.IntLessEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatGreater:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatGreaterEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatLess:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.FloatLessEqual:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.Equality:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.And:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.Or:
		return usesNameInExpr(v.Left, name) || usesNameInExpr(v.Right, name)
	case *checker.Negation:
		return usesNameInExpr(v.Value, name)
	case *checker.Not:
		return usesNameInExpr(v.Value, name)
	case *checker.FunctionCall:
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.CopyExpression:
		return usesNameInExpr(v.Expr, name)
	case *checker.ListLiteral:
		for _, element := range v.Elements {
			if usesNameInExpr(element, name) {
				return true
			}
		}
		return false
	case *checker.ListMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.MapLiteral:
		for i := range v.Keys {
			if usesNameInExpr(v.Keys[i], name) || usesNameInExpr(v.Values[i], name) {
				return true
			}
		}
		return false
	case *checker.MapMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.ResultMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.TryOp:
		if usesNameInExpr(v.Expr(), name) {
			return true
		}
		if v.CatchBlock == nil {
			return false
		}
		if v.CatchVar != "" && v.CatchVar == name {
			return false
		}
		return usesNameInStatements(v.CatchBlock.Stmts, name)
	case *checker.MaybeMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.TemplateStr:
		for _, chunk := range v.Chunks {
			if usesNameInExpr(chunk, name) {
				return true
			}
		}
		return false
	case *checker.StrMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.IntMethod:
		return usesNameInExpr(v.Subject, name)
	case *checker.FloatMethod:
		return usesNameInExpr(v.Subject, name)
	case *checker.BoolMethod:
		return usesNameInExpr(v.Subject, name)
	case *checker.StructInstance:
		for _, fieldName := range sortedStringKeys(v.Fields) {
			if usesNameInExpr(v.Fields[fieldName], name) {
				return true
			}
		}
		return false
	case *checker.InstanceProperty:
		return usesNameInExpr(v.Subject, name)
	case *checker.InstanceMethod:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, arg := range v.Method.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.ModuleStructInstance:
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			if usesNameInExpr(v.Property.Fields[fieldName], name) {
				return true
			}
		}
		return false
	case *checker.ModuleFunctionCall:
		for _, arg := range v.Call.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	case *checker.If:
		if usesNameInExpr(v.Condition, name) || usesNameInStatements(v.Body.Stmts, name) {
			return true
		}
		if v.ElseIf != nil && usesNameInExpr(v.ElseIf, name) {
			return true
		}
		if v.Else != nil && usesNameInStatements(v.Else.Stmts, name) {
			return true
		}
		return false
	case *checker.BoolMatch:
		return usesNameInExpr(v.Subject, name) || usesNameInStatements(v.True.Stmts, name) || usesNameInStatements(v.False.Stmts, name)
	case *checker.IntMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, block := range v.IntCases {
			if usesNameInStatements(block.Stmts, name) {
				return true
			}
		}
		for _, block := range v.RangeCases {
			if usesNameInStatements(block.Stmts, name) {
				return true
			}
		}
		if v.CatchAll != nil && usesNameInStatements(v.CatchAll.Stmts, name) {
			return true
		}
		return false
	case *checker.EnumMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		for _, block := range v.Cases {
			if block != nil && usesNameInStatements(block.Stmts, name) {
				return true
			}
		}
		if v.CatchAll != nil && usesNameInStatements(v.CatchAll.Stmts, name) {
			return true
		}
		return false
	case *checker.OptionMatch:
		return usesNameInExpr(v.Subject, name) || usesNameInStatements(v.Some.Body.Stmts, name) || usesNameInStatements(v.None.Stmts, name)
	case *checker.ResultMatch:
		if usesNameInExpr(v.Subject, name) {
			return true
		}
		okUses := false
		if v.Ok != nil {
			if v.Ok.Pattern != nil && v.Ok.Pattern.Name == name {
				okUses = false
			} else {
				okUses = usesNameInStatements(v.Ok.Body.Stmts, name)
			}
		}
		errUses := false
		if v.Err != nil {
			if v.Err.Pattern != nil && v.Err.Pattern.Name == name {
				errUses = false
			} else {
				errUses = usesNameInStatements(v.Err.Body.Stmts, name)
			}
		}
		return okUses || errUses
	case *checker.FunctionDef:
		for _, param := range v.Parameters {
			if param.Name == name {
				return false
			}
		}
		if v.Body != nil {
			return usesNameInStatements(v.Body.Stmts, name)
		}
		return false
	default:
		return false
	}
}

func (e *emitter) emitExpressionIntoValue(expr checker.Expression, expectedType checker.Type, onValue func(string) error) error {
	if ifExpr, ok := expr.(*checker.If); ok {
		return e.emitIfIntoValue(ifExpr, expectedType, onValue)
	}
	if tryOp, ok := expr.(*checker.TryOp); ok {
		return e.emitTryOp(tryOp, e.fnReturnType, onValue, nil)
	}
	value, err := e.emitValueForType(expr, expectedType)
	if err != nil {
		return err
	}
	copied, err := emitCopiedValue(value, expr.Type())
	if err != nil {
		return err
	}
	return onValue(copied)
}

func (e *emitter) emitBlockValue(block *checker.Block, expectedType checker.Type, onValue func(string) error) error {
	if block == nil || block.Type() == checker.Void {
		return fmt.Errorf("expected value-producing block")
	}
	lastMeaningful := lastMeaningfulStatementIndex(block.Stmts)
	for i, stmt := range block.Stmts {
		if stmt.Break {
			e.line("break")
			continue
		}
		remaining := block.Stmts[i+1:]
		if stmt.Stmt != nil {
			if err := e.emitNonProducing(stmt.Stmt, remaining, e.fnReturnType); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if i == lastMeaningful {
			return e.emitExpressionIntoValue(stmt.Expr, expectedType, onValue)
		}
		if err := e.emitExpressionStatement(stmt.Expr, e.fnReturnType, false); err != nil {
			return err
		}
	}
	return fmt.Errorf("expected value-producing block")
}

func (e *emitter) emitIfIntoValue(expr *checker.If, expectedType checker.Type, onValue func(string) error) error {
	condition, err := e.emitExpr(expr.Condition)
	if err != nil {
		return err
	}
	e.line("if " + condition + " {")
	e.indent++
	if err := e.emitBlockValue(expr.Body, expectedType, onValue); err != nil {
		return err
	}
	e.indent--
	if expr.ElseIf != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitIfIntoValue(expr.ElseIf, expectedType, onValue); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	if expr.Else != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitBlockValue(expr.Else, expectedType, onValue); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	return fmt.Errorf("if expression without else is not supported in value position")
}

func (e *emitter) emitTryExpr(op *checker.TryOp) (string, error) {
	if e.fnReturnType == nil {
		return "", fmt.Errorf("try expressions are only supported in function bodies")
	}
	tempName := e.nextTemp("TryValue")
	typeName, err := emitType(op.Type())
	if err != nil {
		return "", err
	}
	e.line(fmt.Sprintf("var %s %s", tempName, typeName))
	assignValue := func(value string) error {
		copied, err := emitCopiedValue(value, op.Type())
		if err != nil {
			return err
		}
		e.line(fmt.Sprintf("%s = %s", tempName, copied))
		return nil
	}
	if err := e.emitTryOp(op, e.fnReturnType, assignValue, nil); err != nil {
		return "", err
	}
	return tempName, nil
}

func (e *emitter) emitTryOp(op *checker.TryOp, returnType checker.Type, onSuccess func(successValue string) error, onCatchValue func(catchValue string) error) error {
	subject, err := e.emitExpr(op.Expr())
	if err != nil {
		return err
	}
	tempName := e.nextTemp("Try")
	e.line(tempName + " := " + subject)

	switch op.Kind {
	case checker.TryResult:
		e.line("if " + tempName + ".IsErr() {")
		e.indent++
		if op.CatchBlock != nil {
			if op.CatchVar != "" && op.CatchVar != "_" {
				catchName := goName(op.CatchVar, false)
				e.line(fmt.Sprintf("%s := %s.UnwrapErr()", catchName, tempName))
				if !usesNameInStatements(op.CatchBlock.Stmts, op.CatchVar) {
					e.line("_ = " + catchName)
				}
			}
			if onCatchValue != nil {
				if op.CatchBlock.Type() == checker.Void {
					return fmt.Errorf("void try catch block is not supported in value position")
				}
				if err := e.emitBlockValue(op.CatchBlock, op.CatchBlock.Type(), onCatchValue); err != nil {
					return err
				}
			} else {
				if err := e.emitStatements(op.CatchBlock.Stmts, returnType); err != nil {
					return err
				}
				if op.CatchBlock.Type() == checker.Void {
					if returnType == nil || returnType == checker.Void {
						e.line("return")
					} else {
						return fmt.Errorf("void try catch block is not supported for return type %s", returnType)
					}
				}
			}
		} else {
			resultType, ok := e.fnReturnType.(*checker.Result)
			if !ok {
				return fmt.Errorf("try without catch on Result requires function to return a Result type, got %v", e.fnReturnType)
			}
			valueType, err := emitType(resultType.Val())
			if err != nil {
				return err
			}
			errType, err := emitType(resultType.Err())
			if err != nil {
				return err
			}
			e.line(fmt.Sprintf("return %s.Err[%s, %s](%s.UnwrapErr())", helperImportAlias, valueType, errType, tempName))
		}
		e.indent--
		e.line("}")

		if onSuccess != nil {
			successValue, err := emitCopiedValue(tempName+".UnwrapOk()", op.OkType)
			if err != nil {
				return err
			}
			return onSuccess(successValue)
		}
		return nil
	case checker.TryMaybe:
		e.line("if " + tempName + ".IsNone() {")
		e.indent++
		if op.CatchBlock != nil {
			if onCatchValue != nil {
				if op.CatchBlock.Type() == checker.Void {
					return fmt.Errorf("void try catch block is not supported in value position")
				}
				if err := e.emitBlockValue(op.CatchBlock, op.CatchBlock.Type(), onCatchValue); err != nil {
					return err
				}
			} else {
				if err := e.emitStatements(op.CatchBlock.Stmts, returnType); err != nil {
					return err
				}
				if op.CatchBlock.Type() == checker.Void {
					if returnType == nil || returnType == checker.Void {
						e.line("return")
					} else {
						return fmt.Errorf("void try catch block is not supported for return type %s", returnType)
					}
				}
			}
		} else {
			maybeType, ok := e.fnReturnType.(*checker.Maybe)
			if !ok {
				return fmt.Errorf("try without catch on Maybe requires function to return a Maybe type, got %v", e.fnReturnType)
			}
			innerType, err := emitType(maybeType.Of())
			if err != nil {
				return err
			}
			e.line(fmt.Sprintf("return %s.None[%s]()", helperImportAlias, innerType))
		}
		e.indent--
		e.line("}")

		if onSuccess != nil {
			successValue, err := emitCopiedValue(tempName+".Expect("+strconv.Quote("unreachable none in try success path")+")", op.OkType)
			if err != nil {
				return err
			}
			return onSuccess(successValue)
		}
		return nil
	default:
		return fmt.Errorf("unsupported try kind: %v", op.Kind)
	}
}

func (e *emitter) emitExpressionStatement(expr checker.Expression, returnType checker.Type, isLast bool) error {
	if panicExpr, ok := expr.(*checker.Panic); ok {
		message, err := e.emitExpr(panicExpr.Message)
		if err != nil {
			return err
		}
		e.line("panic(" + message + ")")
		return nil
	}
	if panicExpr, ok := expr.(checker.Panic); ok {
		message, err := e.emitExpr(panicExpr.Message)
		if err != nil {
			return err
		}
		e.line("panic(" + message + ")")
		return nil
	}
	if ifExpr, ok := expr.(*checker.If); ok {
		return e.emitIfStatement(ifExpr, returnType, isLast)
	}
	if tryOp, ok := expr.(*checker.TryOp); ok {
		var onSuccess func(string) error
		if isLast && returnType != nil && returnType != checker.Void {
			onSuccess = func(successValue string) error {
				e.line("return " + successValue)
				return nil
			}
		}
		return e.emitTryOp(tryOp, returnType, onSuccess, nil)
	}
	if isLast && returnType != nil && returnType != checker.Void {
		switch typed := expr.(type) {
		case *checker.BoolMatch:
			value, err := e.emitBoolMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.IntMatch:
			value, err := e.emitIntMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.EnumMatch:
			value, err := e.emitEnumMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.OptionMatch:
			value, err := e.emitOptionMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		case *checker.ResultMatch:
			value, err := e.emitResultMatch(typed, returnType)
			if err != nil {
				return err
			}
			e.line("return " + value)
			return nil
		}
		value, err := e.emitValueForType(expr, returnType)
		if err != nil {
			return err
		}
		e.line("return " + value)
		return nil
	}

	value, err := e.emitExpr(expr)
	if err != nil {
		return err
	}
	if isCallExpression(expr) {
		e.line(value)
		return nil
	}
	if returnType == checker.Void && isLast {
		e.line("_ = " + value)
		return nil
	}
	e.line("_ = " + value)
	return nil
}

func (e *emitter) emitIfStatement(expr *checker.If, returnType checker.Type, isLast bool) error {
	condition, err := e.emitExpr(expr.Condition)
	if err != nil {
		return err
	}
	e.line("if " + condition + " {")
	e.indent++
	var branchReturnType checker.Type = checker.Void
	if isLast && returnType != nil && returnType != checker.Void {
		branchReturnType = returnType
	}
	if err := e.emitStatements(expr.Body.Stmts, branchReturnType); err != nil {
		return err
	}
	e.indent--
	if expr.ElseIf != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitIfStatement(expr.ElseIf, returnType, isLast); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	if expr.Else != nil {
		e.line("} else {")
		e.indent++
		if err := e.emitStatements(expr.Else.Stmts, branchReturnType); err != nil {
			return err
		}
		e.indent--
		e.line("}")
		return nil
	}
	if isLast && returnType != nil && returnType != checker.Void {
		return fmt.Errorf("if expression without else is not supported in return position")
	}
	e.line("}")
	return nil
}

func isCallExpression(expr checker.Expression) bool {
	switch expr.(type) {
	case *checker.FunctionCall, *checker.ModuleFunctionCall, *checker.InstanceMethod:
		return true
	default:
		return false
	}
}

func (e *emitter) emitExpr(expr checker.Expression) (string, error) {
	switch v := expr.(type) {
	case *checker.IntLiteral:
		return strconv.Itoa(v.Value), nil
	case *checker.FloatLiteral:
		return strconv.FormatFloat(v.Value, 'g', -1, 64), nil
	case *checker.StrLiteral:
		return strconv.Quote(v.Value), nil
	case *checker.BoolLiteral:
		if v.Value {
			return "true", nil
		}
		return "false", nil
	case *checker.TemplateStr:
		if len(v.Chunks) == 0 {
			return strconv.Quote(""), nil
		}
		chunks := make([]string, 0, len(v.Chunks))
		for _, chunk := range v.Chunks {
			emitted, err := e.emitExpr(chunk)
			if err != nil {
				return "", err
			}
			chunks = append(chunks, emitted)
		}
		return "(" + strings.Join(chunks, " + ") + ")", nil
	case *checker.VoidLiteral:
		return "struct{}{}", nil
	case *checker.EnumVariant:
		return e.emitEnumVariant(v)
	case checker.Panic:
		return e.emitPanicExpr(v.Message, v.Type())
	case *checker.Panic:
		return e.emitPanicExpr(v.Message, v.Type())
	case *checker.CopyExpression:
		return e.emitCopyExpr(v)
	case *checker.ListLiteral:
		elements := make([]string, 0, len(v.Elements))
		for _, element := range v.Elements {
			emitted, err := e.emitExpr(element)
			if err != nil {
				return "", err
			}
			elements = append(elements, emitted)
		}
		typeName, err := emitType(v.ListType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(elements, ", ")), nil
	case *checker.MapLiteral:
		entries := make([]string, 0, len(v.Keys))
		for i := range v.Keys {
			key, err := e.emitExpr(v.Keys[i])
			if err != nil {
				return "", err
			}
			value, err := e.emitExpr(v.Values[i])
			if err != nil {
				return "", err
			}
			entries = append(entries, fmt.Sprintf("%s: %s", key, value))
		}
		typeName, err := emitType(v.Type())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s{%s}", typeName, strings.Join(entries, ", ")), nil
	case *checker.BoolMatch:
		return e.emitBoolMatch(v, nil)
	case *checker.IntMatch:
		return e.emitIntMatch(v, nil)
	case *checker.EnumMatch:
		return e.emitEnumMatch(v, nil)
	case *checker.OptionMatch:
		return e.emitOptionMatch(v, nil)
	case *checker.ResultMatch:
		return e.emitResultMatch(v, nil)
	case *checker.TryOp:
		return e.emitTryExpr(v)
	case *checker.ResultMethod:
		return e.emitResultMethod(v)
	case *checker.MaybeMethod:
		return e.emitMaybeMethod(v)
	case *checker.StrMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.StrSize:
			return fmt.Sprintf("len(%s)", subject), nil
		case checker.StrIsEmpty:
			return fmt.Sprintf("len(%s) == 0", subject), nil
		case checker.StrContains:
			arg, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.Contains(%s, %s)", subject, arg), nil
		case checker.StrReplace:
			oldValue, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			newValue, err := e.emitExpr(v.Args[1])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.Replace(%s, %s, %s, 1)", subject, oldValue, newValue), nil
		case checker.StrReplaceAll:
			oldValue, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			newValue, err := e.emitExpr(v.Args[1])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.ReplaceAll(%s, %s, %s)", subject, oldValue, newValue), nil
		case checker.StrSplit:
			arg, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.Split(%s, %s)", subject, arg), nil
		case checker.StrStartsWith:
			arg, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("strings.HasPrefix(%s, %s)", subject, arg), nil
		case checker.StrToStr:
			return subject, nil
		case checker.StrTrim:
			return fmt.Sprintf("strings.TrimSpace(%s)", subject), nil
		default:
			return "", fmt.Errorf("unsupported string method: %v", v.Kind)
		}
	case *checker.IntMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.IntToStr:
			return fmt.Sprintf("strconv.Itoa(%s)", subject), nil
		default:
			return "", fmt.Errorf("unsupported int method: %v", v.Kind)
		}
	case *checker.FloatMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.FloatToStr:
			return fmt.Sprintf("strconv.FormatFloat(%s, 'f', -1, 64)", subject), nil
		case checker.FloatToInt:
			return fmt.Sprintf("func() int { value := float64(%s); return int(value) }()", subject), nil
		default:
			return "", fmt.Errorf("unsupported float method: %v", v.Kind)
		}
	case *checker.BoolMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.BoolToStr:
			return fmt.Sprintf("strconv.FormatBool(%s)", subject), nil
		default:
			return "", fmt.Errorf("unsupported bool method: %v", v.Kind)
		}
	case *checker.StructInstance:
		fields := make([]string, 0, len(v.Fields))
		for _, fieldName := range sortedStringKeys(v.Fields) {
			value, err := e.emitExpr(v.Fields[fieldName])
			if err != nil {
				return "", err
			}
			fields = append(fields, fmt.Sprintf("%s: %s", goName(fieldName, true), value))
		}
		return fmt.Sprintf("%s{%s}", goName(v.Name, true), strings.Join(fields, ", ")), nil
	case *checker.Identifier:
		return goName(v.Name, false), nil
	case checker.Variable:
		return goName(v.Name(), false), nil
	case *checker.Variable:
		return goName(v.Name(), false), nil
	case *checker.IntAddition:
		return e.emitBinary(v.Left, "+", v.Right)
	case *checker.IntSubtraction:
		return e.emitBinary(v.Left, "-", v.Right)
	case *checker.IntMultiplication:
		return e.emitBinary(v.Left, "*", v.Right)
	case *checker.IntDivision:
		return e.emitBinary(v.Left, "/", v.Right)
	case *checker.IntModulo:
		return e.emitBinary(v.Left, "%", v.Right)
	case *checker.FloatAddition:
		return e.emitBinary(v.Left, "+", v.Right)
	case *checker.FloatSubtraction:
		return e.emitBinary(v.Left, "-", v.Right)
	case *checker.FloatMultiplication:
		return e.emitBinary(v.Left, "*", v.Right)
	case *checker.FloatDivision:
		return e.emitBinary(v.Left, "/", v.Right)
	case *checker.StrAddition:
		return e.emitBinary(v.Left, "+", v.Right)
	case *checker.IntGreater:
		return e.emitBinary(v.Left, ">", v.Right)
	case *checker.IntGreaterEqual:
		return e.emitBinary(v.Left, ">=", v.Right)
	case *checker.IntLess:
		return e.emitBinary(v.Left, "<", v.Right)
	case *checker.IntLessEqual:
		return e.emitBinary(v.Left, "<=", v.Right)
	case *checker.FloatGreater:
		return e.emitBinary(v.Left, ">", v.Right)
	case *checker.FloatGreaterEqual:
		return e.emitBinary(v.Left, ">=", v.Right)
	case *checker.FloatLess:
		return e.emitBinary(v.Left, "<", v.Right)
	case *checker.FloatLessEqual:
		return e.emitBinary(v.Left, "<=", v.Right)
	case *checker.Equality:
		return e.emitBinary(v.Left, "==", v.Right)
	case *checker.And:
		return e.emitBinary(v.Left, "&&", v.Right)
	case *checker.Or:
		return e.emitBinary(v.Left, "||", v.Right)
	case *checker.Negation:
		inner, err := e.emitExpr(v.Value)
		if err != nil {
			return "", err
		}
		return "(-" + inner + ")", nil
	case *checker.Not:
		inner, err := e.emitExpr(v.Value)
		if err != nil {
			return "", err
		}
		return "(!" + inner + ")", nil
	case *checker.FunctionDef:
		return e.emitFunctionLiteral(v)
	case *checker.FunctionCall:
		args, err := e.emitCallArgs(v)
		if err != nil {
			return "", err
		}
		name := e.functionNames[v.Name]
		if name == "" {
			name = goName(v.Name, false)
		}
		return fmt.Sprintf("%s(%s)", name, strings.Join(args, ", ")), nil
	case *checker.InstanceProperty:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", subject, goName(v.Property, true)), nil
	case *checker.InstanceMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		args, err := e.emitCallArgs(v.Method)
		if err != nil {
			return "", err
		}
		methodName := goName(v.Method.Name, false)
		if v.StructType != nil {
			if method, ok := v.StructType.Methods[v.Method.Name]; ok {
				methodName = goName(method.Name, !method.Private)
			}
		}
		return fmt.Sprintf("%s.%s(%s)", subject, methodName, strings.Join(args, ", ")), nil
	case *checker.ListMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.ListSize:
			return fmt.Sprintf("len(%s)", subject), nil
		case checker.ListAt:
			if len(v.Args) != 1 {
				return "", fmt.Errorf("list.at expects one arg")
			}
			index, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("%s[%s]", subject, index), nil
		case checker.ListPush, checker.ListPrepend, checker.ListSet, checker.ListSort, checker.ListSwap:
			return e.emitListMutationExpr(v)
		default:
			return "", fmt.Errorf("unsupported list method: %v", v.Kind)
		}
	case *checker.MapMethod:
		subject, err := e.emitExpr(v.Subject)
		if err != nil {
			return "", err
		}
		switch v.Kind {
		case checker.MapKeys:
			mapType, ok := v.Subject.Type().(*checker.Map)
			if !ok {
				return "", fmt.Errorf("expected map subject, got %s", v.Subject.Type())
			}
			keysType, err := emitType(checker.MakeList(mapType.Key()))
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func() %s { keys := make(%s, 0, len(%s)); for key := range %s { keys = append(keys, key) }; return keys }()", keysType, keysType, subject, subject), nil
		case checker.MapSize:
			return fmt.Sprintf("len(%s)", subject), nil
		case checker.MapGet:
			if len(v.Args) != 1 {
				return "", fmt.Errorf("map.get expects one arg")
			}
			key, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			resultType, err := emitType(v.Type())
			if err != nil {
				return "", err
			}
			maybeType, ok := v.Type().(*checker.Maybe)
			if !ok {
				return "", fmt.Errorf("expected maybe return type for map.get, got %s", v.Type())
			}
			innerType, err := emitType(maybeType.Of())
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func() %s { if value, ok := %s[%s]; ok { return %s.Some(value) }; return %s.None[%s]() }()", resultType, subject, key, helperImportAlias, helperImportAlias, innerType), nil
		case checker.MapHas:
			if len(v.Args) != 1 {
				return "", fmt.Errorf("map.has expects one arg")
			}
			key, err := e.emitExpr(v.Args[0])
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func() bool { _, ok := %s[%s]; return ok }()", subject, key), nil
		case checker.MapSet, checker.MapDrop:
			return e.emitMapMutationExpr(v)
		default:
			return "", fmt.Errorf("unsupported map method: %v", v.Kind)
		}
	case *checker.ModuleStructInstance:
		if strings.HasPrefix(v.Module, "ard/") {
			return "", fmt.Errorf("standard library module struct instances are not supported yet: %s::%s", v.Module, v.Property.Name)
		}
		fields := make([]string, 0, len(v.Property.Fields))
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			value, err := e.emitExpr(v.Property.Fields[fieldName])
			if err != nil {
				return "", err
			}
			fields = append(fields, fmt.Sprintf("%s: %s", goName(fieldName, true), value))
		}
		alias := packageNameForModulePath(v.Module)
		return fmt.Sprintf("%s.%s{%s}", alias, goName(v.Property.Name, true), strings.Join(fields, ", ")), nil
	case *checker.ModuleFunctionCall:
		if v.Module == "ard/maybe" {
			return e.emitMaybeModuleCall(v, nil)
		}
		if v.Module == "ard/result" {
			return e.emitResultModuleCall(v, nil)
		}
		if strings.HasPrefix(v.Module, "ard/") {
			return "", fmt.Errorf("standard library module calls are not supported yet: %s::%s", v.Module, v.Call.Name)
		}
		args, err := e.emitCallArgs(v.Call)
		if err != nil {
			return "", err
		}
		alias := packageNameForModulePath(v.Module)
		name := goName(e.resolvedModuleFunctionName(v.Module, v.Call), true)
		return fmt.Sprintf("%s.%s(%s)", alias, name, strings.Join(args, ", ")), nil
	case *checker.ModuleSymbol:
		if strings.HasPrefix(v.Module, "ard/") {
			return "", fmt.Errorf("standard library module symbols are not supported yet: %s::%s", v.Module, v.Symbol.Name)
		}
		alias := packageNameForModulePath(v.Module)
		name := goName(v.Symbol.Name, true)
		return fmt.Sprintf("%s.%s", alias, name), nil
	default:
		return "", fmt.Errorf("unsupported expression: %T", expr)
	}
}

func emitAssignmentTarget(expr checker.Expression) (string, error) {
	switch target := expr.(type) {
	case *checker.Identifier:
		return goName(target.Name, false), nil
	case checker.Variable:
		return goName(target.Name(), false), nil
	case *checker.Variable:
		return goName(target.Name(), false), nil
	case *checker.InstanceProperty:
		subject, err := emitAssignmentTarget(target.Subject)
		if err != nil {
			subjectExpr, exprErr := emitBareExpr(target.Subject)
			if exprErr != nil {
				return "", exprErr
			}
			subject = subjectExpr
		}
		return fmt.Sprintf("%s.%s", subject, goName(target.Property, true)), nil
	default:
		return "", fmt.Errorf("unsupported reassignment target: %T", expr)
	}
}

func emitBareExpr(expr checker.Expression) (string, error) {
	switch v := expr.(type) {
	case *checker.Identifier:
		return goName(v.Name, false), nil
	case checker.Variable:
		return goName(v.Name(), false), nil
	case *checker.Variable:
		return goName(v.Name(), false), nil
	case *checker.InstanceProperty:
		subject, err := emitBareExpr(v.Subject)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", subject, goName(v.Property, true)), nil
	default:
		return "", fmt.Errorf("unsupported bare expression: %T", expr)
	}
}

func (e *emitter) emitCopyExpr(copy *checker.CopyExpression) (string, error) {
	inner, err := e.emitExpr(copy.Expr)
	if err != nil {
		return "", err
	}
	switch typed := copy.Type_.(type) {
	case *checker.List:
		typeName, err := emitType(typed)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("append(%s(nil), %s...)", typeName, inner), nil
	default:
		return inner, nil
	}
}

func (e *emitter) emitListMutationExpr(method *checker.ListMethod) (string, error) {
	target, err := emitAssignmentTarget(method.Subject)
	if err != nil {
		return "", err
	}
	subjectType, ok := method.Subject.Type().(*checker.List)
	if !ok {
		return "", fmt.Errorf("expected list subject, got %s", method.Subject.Type())
	}
	typeName, err := emitType(subjectType)
	if err != nil {
		return "", err
	}

	switch method.Kind {
	case checker.ListPush:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("list.push expects one arg")
		}
		value, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() %s { %s = append(%s, %s); return %s }()", typeName, target, target, value, target), nil
	case checker.ListPrepend:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("list.prepend expects one arg")
		}
		value, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() %s { %s = append(%s{%s}, %s...); return %s }()", typeName, target, typeName, value, target, target), nil
	case checker.ListSet:
		if len(method.Args) != 2 {
			return "", fmt.Errorf("list.set expects two args")
		}
		index, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		value, err := e.emitExpr(method.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() bool { if %s >= 0 && %s < len(%s) { %s[%s] = %s; return true }; return false }()", index, index, target, target, index, value), nil
	case checker.ListSort:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("list.sort expects one arg")
		}
		cmp, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() struct{} { sort.SliceStable(%s, func(i, j int) bool { return %s(%s[i], %s[j]) }); return struct{}{} }()", target, cmp, target, target), nil
	case checker.ListSwap:
		if len(method.Args) != 2 {
			return "", fmt.Errorf("list.swap expects two args")
		}
		left, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		right, err := e.emitExpr(method.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() struct{} { %s[%s], %s[%s] = %s[%s], %s[%s]; return struct{}{} }()", target, left, target, right, target, right, target, left), nil
	default:
		return "", fmt.Errorf("unsupported mutable list method: %v", method.Kind)
	}
}

func (e *emitter) emitEnumVariant(variant *checker.EnumVariant) (string, error) {
	return fmt.Sprintf("struct{ Tag int }{Tag: %d}", variant.Discriminant), nil
}

func matchReturnType(expected, fallback checker.Type) checker.Type {
	if expected != nil {
		return expected
	}
	return fallback
}

func (e *emitter) emitValueTemp(prefix string, valueType checker.Type) (string, func(string) error, error) {
	if valueType == nil || valueType == checker.Void {
		return "", nil, fmt.Errorf("expected non-void value type")
	}
	tempName := e.nextTemp(prefix)
	typeName, err := emitType(valueType)
	if err != nil {
		return "", nil, err
	}
	e.line(fmt.Sprintf("var %s %s", tempName, typeName))
	assign := func(value string) error {
		e.line(fmt.Sprintf("%s = %s", tempName, value))
		return nil
	}
	return tempName, assign, nil
}

func (e *emitter) emitBoolMatch(match *checker.BoolMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("BoolMatch", returnType)
	if err != nil {
		return "", err
	}
	e.line("if " + subject + " {")
	e.indent++
	if err := e.emitBlockValue(match.True, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("} else {")
	e.indent++
	if err := e.emitBlockValue(match.False, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitIntMatch(match *checker.IntMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("IntMatch", returnType)
	if err != nil {
		return "", err
	}
	e.line("switch {")
	e.indent++
	for _, value := range sortedIntKeys(match.IntCases) {
		block := match.IntCases[value]
		e.line(fmt.Sprintf("case %s == %d:", subject, value))
		e.indent++
		if err := e.emitBlockValue(block, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	for _, intRange := range sortedIntRanges(match.RangeCases) {
		block := match.RangeCases[intRange]
		e.line(fmt.Sprintf("case %s >= %d && %s <= %d:", subject, intRange.Start, subject, intRange.End))
		e.indent++
		if err := e.emitBlockValue(block, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitBlockValue(match.CatchAll, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	} else if returnType != checker.Void {
		e.line("default:")
		e.indent++
		e.line(`panic("non-exhaustive int match")`)
		e.indent--
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitOptionMatch(match *checker.OptionMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("MaybeMatch", returnType)
	if err != nil {
		return "", err
	}
	maybeName := e.nextTemp("Maybe")
	e.line(maybeName + " := " + subject)
	e.line("if " + maybeName + ".IsSome() {")
	e.indent++
	if match.Some != nil && match.Some.Pattern != nil {
		patternName := goName(match.Some.Pattern.Name, false)
		e.line(fmt.Sprintf("%s := %s.Expect(%q)", patternName, maybeName, "unreachable none in maybe match"))
		if !usesNameInStatements(match.Some.Body.Stmts, match.Some.Pattern.Name) {
			e.line("_ = " + patternName)
		}
	}
	if err := e.emitBlockValue(match.Some.Body, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("} else {")
	e.indent++
	if err := e.emitBlockValue(match.None, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitResultMatch(match *checker.ResultMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("ResultMatch", returnType)
	if err != nil {
		return "", err
	}
	resultName := e.nextTemp("Result")
	e.line(resultName + " := " + subject)
	e.line("if " + resultName + ".IsOk() {")
	e.indent++
	if match.Ok != nil && match.Ok.Pattern != nil {
		boundValue, err := emitCopiedValue(resultName+".UnwrapOk()", match.OkType)
		if err != nil {
			return "", err
		}
		okName := goName(match.Ok.Pattern.Name, false)
		e.line(fmt.Sprintf("%s := %s", okName, boundValue))
		if !usesNameInStatements(match.Ok.Body.Stmts, match.Ok.Pattern.Name) {
			e.line("_ = " + okName)
		}
	}
	if err := e.emitBlockValue(match.Ok.Body, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("} else {")
	e.indent++
	if match.Err != nil && match.Err.Pattern != nil {
		errName := goName(match.Err.Pattern.Name, false)
		e.line(fmt.Sprintf("%s := %s.UnwrapErr()", errName, resultName))
		if !usesNameInStatements(match.Err.Body.Stmts, match.Err.Pattern.Name) {
			e.line("_ = " + errName)
		}
	}
	if err := e.emitBlockValue(match.Err.Body, returnType, assign); err != nil {
		return "", err
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitEnumMatch(match *checker.EnumMatch, expectedType checker.Type) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	returnType := matchReturnType(expectedType, match.Type())
	tempName, assign, err := e.emitValueTemp("EnumMatch", returnType)
	if err != nil {
		return "", err
	}
	e.line("switch " + subject + ".Tag {")
	e.indent++
	discriminants := make([]int, 0, len(match.DiscriminantToIndex))
	for discriminant := range match.DiscriminantToIndex {
		discriminants = append(discriminants, discriminant)
	}
	sort.Ints(discriminants)
	for _, discriminant := range discriminants {
		idx := match.DiscriminantToIndex[discriminant]
		if idx < 0 || int(idx) >= len(match.Cases) || match.Cases[idx] == nil {
			continue
		}
		e.line(fmt.Sprintf("case %d:", discriminant))
		e.indent++
		if err := e.emitBlockValue(match.Cases[idx], returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	}
	if match.CatchAll != nil {
		e.line("default:")
		e.indent++
		if err := e.emitBlockValue(match.CatchAll, returnType, assign); err != nil {
			return "", err
		}
		e.indent--
	} else if returnType != checker.Void {
		e.line("default:")
		e.indent++
		e.line(`panic("non-exhaustive enum match")`)
		e.indent--
	}
	e.indent--
	e.line("}")
	return tempName, nil
}

func (e *emitter) emitPanicExpr(message checker.Expression, resultType checker.Type) (string, error) {
	messageExpr, err := e.emitExpr(message)
	if err != nil {
		return "", err
	}
	return e.emitInlineFunc(resultType, func(inner *emitter) error {
		inner.line("panic(" + messageExpr + ")")
		return nil
	})
}

func (e *emitter) emitInlineFunc(returnType checker.Type, body func(inner *emitter) error) (string, error) {
	inner := &emitter{
		module:        e.module,
		packageName:   e.packageName,
		entrypoint:    e.entrypoint,
		imports:       e.imports,
		functionNames: e.functionNames,
		emittedTypes:  e.emittedTypes,
		indent:        1,
		fnReturnType:  e.fnReturnType,
	}
	if err := body(inner); err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.WriteString("func()")
	if returnType != nil && returnType != checker.Void {
		typeName, err := emitType(returnType)
		if err != nil {
			return "", err
		}
		builder.WriteString(" ")
		builder.WriteString(typeName)
	}
	builder.WriteString(" {\n")
	builder.WriteString(inner.builder.String())
	builder.WriteString("}()")
	return builder.String(), nil
}

func (e *emitter) emitResultModuleCall(call *checker.ModuleFunctionCall, expectedType checker.Type) (string, error) {
	switch call.Call.Name {
	case "ok":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Result::ok expects one arg")
		}
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if (!ok || resultHasUnresolvedTypeVar(resultType)) && expectedType != nil {
			if expectedResult, ok := expectedType.(*checker.Result); ok {
				resultType = expectedResult
				ok = true
			}
		}
		if !ok {
			return "", fmt.Errorf("Result::ok expected Result return type, got %s", call.Call.ReturnType)
		}
		arg, err := e.emitValueForType(call.Call.Args[0], resultType.Val())
		if err != nil {
			return "", err
		}
		valueType, err := emitType(resultType.Val())
		if err != nil {
			return "", err
		}
		errType, err := emitType(resultType.Err())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Ok[%s, %s](%s)", helperImportAlias, valueType, errType, arg), nil
	case "err":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Result::err expects one arg")
		}
		resultType, ok := call.Call.ReturnType.(*checker.Result)
		if (!ok || resultHasUnresolvedTypeVar(resultType)) && expectedType != nil {
			if expectedResult, ok := expectedType.(*checker.Result); ok {
				resultType = expectedResult
				ok = true
			}
		}
		if !ok {
			return "", fmt.Errorf("Result::err expected Result return type, got %s", call.Call.ReturnType)
		}
		arg, err := e.emitValueForType(call.Call.Args[0], resultType.Err())
		if err != nil {
			return "", err
		}
		valueType, err := emitType(resultType.Val())
		if err != nil {
			return "", err
		}
		errType, err := emitType(resultType.Err())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Err[%s, %s](%s)", helperImportAlias, valueType, errType, arg), nil
	default:
		return "", fmt.Errorf("unsupported result module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitMaybeMethod(method *checker.MaybeMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	emitMaybeArg := func(index int) (string, error) {
		if index >= len(method.Args) {
			return "", fmt.Errorf("maybe method missing arg %d", index)
		}
		return e.emitExpr(method.Args[index])
	}
	switch method.Kind {
	case checker.MaybeExpect:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.expect expects one arg")
		}
		message, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Expect(%s)", subject, message), nil
	case checker.MaybeIsNone:
		return fmt.Sprintf("%s.IsNone()", subject), nil
	case checker.MaybeIsSome:
		return fmt.Sprintf("%s.IsSome()", subject), nil
	case checker.MaybeOr:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.or expects one arg")
		}
		fallback, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Or(%s)", subject, fallback), nil
	case checker.MaybeMap:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.map expects one arg")
		}
		mapper, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.MaybeMap(%s, %s)", helperImportAlias, subject, mapper), nil
	case checker.MaybeAndThen:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("maybe.and_then expects one arg")
		}
		mapper, err := emitMaybeArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.MaybeAndThen(%s, %s)", helperImportAlias, subject, mapper), nil
	default:
		return "", fmt.Errorf("unsupported maybe method: %v", method.Kind)
	}
}

func (e *emitter) emitResultMethod(method *checker.ResultMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	emitResultArg := func(index int) (string, error) {
		if index >= len(method.Args) {
			return "", fmt.Errorf("result method missing arg %d", index)
		}
		return e.emitExpr(method.Args[index])
	}
	switch method.Kind {
	case checker.ResultExpect:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.expect expects one arg")
		}
		message, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Expect(%s)", subject, message), nil
	case checker.ResultOr:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.or expects one arg")
		}
		fallback, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Or(%s)", subject, fallback), nil
	case checker.ResultIsOk:
		return fmt.Sprintf("%s.IsOk()", subject), nil
	case checker.ResultIsErr:
		return fmt.Sprintf("%s.IsErr()", subject), nil
	case checker.ResultMap:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.map expects one arg")
		}
		mapper, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.ResultMap(%s, %s)", helperImportAlias, subject, mapper), nil
	case checker.ResultMapErr:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.map_err expects one arg")
		}
		mapper, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.ResultMapErr(%s, %s)", helperImportAlias, subject, mapper), nil
	case checker.ResultAndThen:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("result.and_then expects one arg")
		}
		mapper, err := emitResultArg(0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.ResultAndThen(%s, %s)", helperImportAlias, subject, mapper), nil
	default:
		return "", fmt.Errorf("unsupported result method: %v", method.Kind)
	}
}

func (e *emitter) emitMaybeModuleCall(call *checker.ModuleFunctionCall, expectedType checker.Type) (string, error) {
	switch call.Call.Name {
	case "some":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("maybe::some expects one arg")
		}
		arg, err := e.emitExpr(call.Call.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Some(%s)", helperImportAlias, arg), nil
	case "none":
		maybeType, ok := call.Call.ReturnType.(*checker.Maybe)
		if (!ok || maybeHasUnresolvedTypeVar(maybeType)) && expectedType != nil {
			if expectedMaybe, ok := expectedType.(*checker.Maybe); ok {
				maybeType = expectedMaybe
				ok = true
			}
		}
		if !ok {
			return "", fmt.Errorf("maybe::none expected Maybe return type, got %s", call.Call.ReturnType)
		}
		innerType, err := emitType(maybeType.Of())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.None[%s]()", helperImportAlias, innerType), nil
	default:
		return "", fmt.Errorf("unsupported maybe module call: %s", call.Call.Name)
	}
}

func resultHasUnresolvedTypeVar(resultType *checker.Result) bool {
	if resultType == nil {
		return true
	}
	if tv, ok := resultType.Val().(*checker.TypeVar); ok && tv.Actual() == nil {
		return true
	}
	if tv, ok := resultType.Err().(*checker.TypeVar); ok && tv.Actual() == nil {
		return true
	}
	return false
}

func maybeHasUnresolvedTypeVar(maybeType *checker.Maybe) bool {
	if maybeType == nil {
		return true
	}
	if tv, ok := maybeType.Of().(*checker.TypeVar); ok && tv.Actual() == nil {
		return true
	}
	return false
}

func (e *emitter) emitCallArgs(call *checker.FunctionCall) ([]string, error) {
	args := make([]string, 0, len(call.Args))
	var params []checker.Parameter
	if def := call.Definition(); def != nil {
		params = def.Parameters
	}
	for i, arg := range call.Args {
		expectedType := checker.Type(nil)
		if i < len(params) {
			expectedType = params[i].Type
		}
		emitted, err := e.emitValueForType(arg, expectedType)
		if err != nil {
			return nil, err
		}
		args = append(args, emitted)
	}
	return args, nil
}

func moduleFunctionValueName(module checker.Module, def *checker.FunctionDef) string {
	if module == nil || module.Program() == nil || def == nil {
		return ""
	}
	for _, stmt := range module.Program().Statements {
		variableDef, ok := stmt.Stmt.(*checker.VariableDef)
		if !ok {
			continue
		}
		if valueFn, ok := variableDef.Value.(*checker.FunctionDef); ok && (valueFn == def || valueFn.Name == def.Name) {
			return variableDef.Name
		}
		if typeFn, ok := variableDef.Type().(*checker.FunctionDef); ok && (typeFn == def || typeFn.Name == def.Name) {
			return variableDef.Name
		}
	}
	return ""
}

func (e *emitter) resolvedModuleFunctionName(modulePath string, call *checker.FunctionCall) string {
	if call == nil {
		return ""
	}
	name := call.Name
	if def := call.Definition(); def != nil && isFunctionLiteralDef(def) {
		if resolved := moduleFunctionValueName(e.module.Program().Imports[modulePath], def); resolved != "" {
			return resolved
		}
	}
	return name
}

func (e *emitter) emitMapMutationExpr(method *checker.MapMethod) (string, error) {
	target, err := emitAssignmentTarget(method.Subject)
	if err != nil {
		return "", err
	}
	switch method.Kind {
	case checker.MapSet:
		if len(method.Args) != 2 {
			return "", fmt.Errorf("map.set expects two args")
		}
		key, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		value, err := e.emitExpr(method.Args[1])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() bool { %s[%s] = %s; return true }()", target, key, value), nil
	case checker.MapDrop:
		if len(method.Args) != 1 {
			return "", fmt.Errorf("map.drop expects one arg")
		}
		key, err := e.emitExpr(method.Args[0])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() struct{} { delete(%s, %s); return struct{}{} }()", target, key), nil
	default:
		return "", fmt.Errorf("unsupported mutable map method: %v", method.Kind)
	}
}

func (e *emitter) emitBinary(left checker.Expression, op string, right checker.Expression) (string, error) {
	leftExpr, err := e.emitExpr(left)
	if err != nil {
		return "", err
	}
	rightExpr, err := e.emitExpr(right)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(%s %s %s)", leftExpr, op, rightExpr), nil
}

func emitType(t checker.Type) (string, error) {
	switch t {
	case checker.Int:
		return "int", nil
	case checker.Float:
		return "float64", nil
	case checker.Str:
		return "string", nil
	case checker.Bool:
		return "bool", nil
	case checker.Void:
		return "", nil
	}

	switch typed := t.(type) {
	case *checker.TypeVar:
		if actual := typed.Actual(); actual != nil {
			return emitType(actual)
		}
		return "any", nil
	case *checker.Result:
		valueType, err := emitType(typed.Val())
		if err != nil {
			return "", err
		}
		errType, err := emitType(typed.Err())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Result[%s, %s]", helperImportAlias, valueType, errType), nil
	case *checker.Enum:
		return "struct{ Tag int }", nil
	case *checker.Maybe:
		innerType, err := emitType(typed.Of())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.Maybe[%s]", helperImportAlias, innerType), nil
	case *checker.List:
		elementType, err := emitType(typed.Of())
		if err != nil {
			return "", err
		}
		return "[]" + elementType, nil
	case *checker.Map:
		keyType, err := emitType(typed.Key())
		if err != nil {
			return "", err
		}
		valueType, err := emitType(typed.Value())
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("map[%s]%s", keyType, valueType), nil
	case *checker.StructDef:
		return goName(typed.Name, true), nil
	case *checker.FunctionDef:
		return emitFunctionType(typed)
	default:
		return "", fmt.Errorf("unsupported type: %s", t.String())
	}
}

func (e *emitter) line(text string) {
	if text == "" {
		e.builder.WriteString("\n")
		return
	}
	e.builder.WriteString(strings.Repeat("\t", e.indent))
	e.builder.WriteString(text)
	e.builder.WriteString("\n")
}

func goName(name string, exported bool) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	if len(parts) == 0 {
		return "value"
	}
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	for i := range parts {
		if i == 0 && !exported {
			continue
		}
		parts[i] = upperFirst(parts[i])
	}
	result := strings.Join(parts, "")
	if !exported {
		result = lowerFirst(result)
	}
	if result == "" {
		result = "value"
	}
	if isGoKeyword(result) {
		return result + "_"
	}
	return result
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func compilerModuleRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to determine compiler module root")
	}
	return filepath.Dir(filepath.Dir(file)), nil
}

func isGoKeyword(value string) bool {
	switch value {
	case "break", "default", "func", "interface", "select",
		"case", "defer", "go", "map", "struct",
		"chan", "else", "goto", "package", "switch",
		"const", "fallthrough", "if", "range", "type",
		"continue", "for", "import", "return", "var":
		return true
	default:
		return false
	}
}
