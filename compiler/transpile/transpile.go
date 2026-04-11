package transpile

import (
	"fmt"
	gofmt "go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

const generatedGoVersion = "1.26.0"

type emitter struct {
	module        checker.Module
	packageName   string
	entrypoint    bool
	imports       map[string]string
	functionNames map[string]string
	builder       strings.Builder
	indent        int
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
			return nil, fmt.Errorf("extern functions are not supported yet: %s", def.Name)
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
	if err := os.WriteFile(filepath.Join(generatedDir, "go.mod"), []byte(fmt.Sprintf("module %s\n\ngo %s\n", project.ProjectName, generatedGoVersion)), 0o644); err != nil {
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
		case *checker.StructDef:
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
	if stmt.Expr != nil {
		collectImportsFromExpr(stmt.Expr, imports)
	}
	if stmt.Stmt != nil {
		collectImportsFromNonProducing(stmt.Stmt, imports)
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
	case *checker.ModuleStructInstance:
		if !strings.HasPrefix(v.Module, "ard/") {
			imports[v.Module] = packageNameForModulePath(v.Module)
		}
		for _, fieldName := range sortedStringKeys(v.Property.Fields) {
			collectImportsFromExpr(v.Property.Fields[fieldName], imports)
		}
	case *checker.ModuleFunctionCall:
		if !strings.HasPrefix(v.Module, "ard/") {
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
	case *checker.FunctionDef:
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
		def, ok := stmt.Expr.(*checker.FunctionDef)
		if !ok {
			continue
		}
		name := goName(def.Name, !def.Private)
		if e.packageName == "main" && name == "main" {
			name = "ardMain"
		}
		e.functionNames[def.Name] = name
	}
}

func (e *emitter) emitStructDef(def *checker.StructDef) error {
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
	return nil
}

func (e *emitter) emitPackageVariable(def *checker.VariableDef) error {
	value, err := e.emitExpr(def.Value)
	if err != nil {
		return err
	}
	name := goName(def.Name, !def.Mutable)
	e.line(fmt.Sprintf("var %s = %s", name, value))
	return nil
}

func (e *emitter) emitFunction(def *checker.FunctionDef) error {
	params := make([]string, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		typeName, err := emitType(param.Type)
		if err != nil {
			return fmt.Errorf("function %s: %w", def.Name, err)
		}
		params = append(params, fmt.Sprintf("%s %s", goName(param.Name, false), typeName))
	}

	signature := fmt.Sprintf("func %s(%s)", e.functionNames[def.Name], strings.Join(params, ", "))
	if def.ReturnType != checker.Void {
		returnType, err := emitType(def.ReturnType)
		if err != nil {
			return fmt.Errorf("function %s: %w", def.Name, err)
		}
		signature += " " + returnType
	}
	e.line(signature + " {")
	e.indent++
	if err := e.emitStatements(def.Body.Stmts, def.ReturnType); err != nil {
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
			if err := e.emitNonProducing(stmt.Stmt, remaining); err != nil {
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

func (e *emitter) emitNonProducing(stmt checker.NonProducing, remaining []checker.Statement) error {
	switch s := stmt.(type) {
	case *checker.VariableDef:
		value, err := e.emitExpr(s.Value)
		if err != nil {
			return err
		}
		name := goName(s.Name, false)
		e.line(fmt.Sprintf("%s := %s", name, value))
		if !usesNameInStatements(remaining, s.Name) {
			e.line(fmt.Sprintf("_ = %s", name))
		}
		return nil
	case *checker.Reassignment:
		targetName, err := emitAssignmentTarget(s.Target)
		if err != nil {
			return err
		}
		value, err := e.emitExpr(s.Value)
		if err != nil {
			return err
		}
		e.line(fmt.Sprintf("%s = %s", targetName, value))
		return nil
	case *checker.WhileLoop:
		return e.emitWhileLoop(s)
	case *checker.ForLoop:
		return e.emitForLoop(s)
	default:
		return fmt.Errorf("unsupported statement: %T", stmt)
	}
}

func (e *emitter) emitWhileLoop(loop *checker.WhileLoop) error {
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	e.line("for " + condition + " {")
	e.indent++
	if err := e.emitStatements(loop.Body.Stmts, nil); err != nil {
		return err
	}
	e.indent--
	e.line("}")
	return nil
}

func (e *emitter) emitForLoop(loop *checker.ForLoop) error {
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
	if err := e.emitStatements(loop.Body.Stmts, nil); err != nil {
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
	case *checker.StructInstance:
		for _, fieldName := range sortedStringKeys(v.Fields) {
			if usesNameInExpr(v.Fields[fieldName], name) {
				return true
			}
		}
		return false
	case *checker.InstanceProperty:
		return usesNameInExpr(v.Subject, name)
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
	default:
		return false
	}
}

func (e *emitter) emitExpressionStatement(expr checker.Expression, returnType checker.Type, isLast bool) error {
	if ifExpr, ok := expr.(*checker.If); ok {
		return e.emitIfStatement(ifExpr, returnType, isLast)
	}
	if isLast && returnType != nil && returnType != checker.Void {
		value, err := e.emitExpr(expr)
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
	case *checker.FunctionCall, *checker.ModuleFunctionCall:
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
	case *checker.VoidLiteral:
		return "struct{}{}", nil
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
	case *checker.FunctionCall:
		args := make([]string, 0, len(v.Args))
		for _, arg := range v.Args {
			emitted, err := e.emitExpr(arg)
			if err != nil {
				return "", err
			}
			args = append(args, emitted)
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
		case checker.ListPush, checker.ListPrepend, checker.ListSet:
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
		case checker.MapSize:
			return fmt.Sprintf("len(%s)", subject), nil
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
		if strings.HasPrefix(v.Module, "ard/") {
			return "", fmt.Errorf("standard library module calls are not supported yet: %s::%s", v.Module, v.Call.Name)
		}
		args := make([]string, 0, len(v.Call.Args))
		for _, arg := range v.Call.Args {
			emitted, err := e.emitExpr(arg)
			if err != nil {
				return "", err
			}
			args = append(args, emitted)
		}
		alias := packageNameForModulePath(v.Module)
		name := goName(v.Call.Name, true)
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
	default:
		return "", fmt.Errorf("unsupported reassignment target: %T", expr)
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
	default:
		return "", fmt.Errorf("unsupported mutable list method: %v", method.Kind)
	}
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
