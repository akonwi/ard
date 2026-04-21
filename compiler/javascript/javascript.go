package javascript

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

type emitOptions struct {
	invokeMain bool
}

type ffiArtifacts struct {
	useStdlib  bool
	useProject bool
}

type emitter struct {
	target              string
	builder             strings.Builder
	indentLevel         int
	moduleVars          map[string]string
	usedEnumMethods     map[string]map[string]bool
	tempCounter         *int
	currentModule       string
	currentOutputPath   string
	currentFunction     string
	currentReceiver     string
	currentReceiverExpr string
	currentReturnType   checker.Type
	ffi                 ffiArtifacts
	loopDepth           int
	signalBreaks        bool
}

type loweredExpr struct {
	stmts []string
	expr  string
}

func Build(inputPath, outputPath, target string) (string, error) {
	module, projectInfo, err := loadModule(inputPath, target)
	if err != nil {
		return "", err
	}

	resolvedOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	outputDir := filepath.Dir(resolvedOutputPath)
	rootFileName := filepath.Base(resolvedOutputPath)
	files, ffi, err := emitBundle(module, target, emitOptions{}, rootFileName)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	for relPath, source := range files {
		absPath := filepath.Join(outputDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(absPath, source, 0o644); err != nil {
			return "", err
		}
	}
	if err := writeFFICompanions(outputDir, target, projectInfo, ffi); err != nil {
		return "", err
	}
	return outputPath, nil
}

func Run(inputPath, target string, _ []string) error {
	if target == backend.TargetJSBrowser {
		return fmt.Errorf("js-browser cannot be run directly; build and serve the emitted module instead")
	}
	if target != backend.TargetJSServer {
		return fmt.Errorf("unsupported JavaScript run target: %s", target)
	}
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node is required to run js-server output: %w", err)
	}

	module, projectInfo, err := loadModule(inputPath, target)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "ard-js-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	files, ffi, err := emitBundle(module, target, emitOptions{invokeMain: true}, "main.mjs")
	if err != nil {
		return err
	}
	for relPath, source := range files {
		absPath := filepath.Join(tmpDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, source, 0o644); err != nil {
			return err
		}
	}
	if err := writeFFICompanions(tmpDir, target, projectInfo, ffi); err != nil {
		return err
	}
	entryPath := filepath.Join(tmpDir, "main.mjs")

	cmd := exec.Command("node", entryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append([]string{}, os.Environ()...)
	return cmd.Run()
}

func loadModule(inputPath string, target string) (checker.Module, *checker.ProjectInfo, error) {
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

	c := checker.New(relPath, program, moduleResolver, checker.CheckOptions{Target: target})
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			fmt.Println(diagnostic)
		}
		return nil, nil, fmt.Errorf("type errors")
	}

	return c.Module(), moduleResolver.GetProjectInfo(), nil
}

func compilerJSSourcePath(parts ...string) (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to resolve javascript.go path")
	}
	compilerDir := filepath.Dir(filepath.Dir(currentFile))
	all := append([]string{compilerDir}, parts...)
	return filepath.Join(all...), nil
}

func stdlibFFISourcePath(target string) (string, error) {
	switch target {
	case backend.TargetJSServer:
		return compilerJSSourcePath("std_lib", "ffi.js-server.mjs")
	case backend.TargetJSBrowser:
		return compilerJSSourcePath("std_lib", "ffi.js-browser.mjs")
	default:
		return "", fmt.Errorf("unsupported JS ffi target: %s", target)
	}
}

func preludeSourcePath() (string, error) {
	return compilerJSSourcePath("javascript", "ard.prelude.mjs")
}

func writeFFICompanions(outputDir string, target string, projectInfo *checker.ProjectInfo, ffi ffiArtifacts) error {
	if target != backend.TargetJSServer && target != backend.TargetJSBrowser {
		return nil
	}
	preludePath, err := preludeSourcePath()
	if err != nil {
		return err
	}
	preludeContent, err := os.ReadFile(preludePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "ard.prelude.mjs"), preludeContent, 0o644); err != nil {
		return err
	}
	if ffi.useStdlib {
		sourcePath, err := stdlibFFISourcePath(target)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outputDir, "ffi.stdlib."+target+".mjs"), content, 0o644); err != nil {
			return err
		}
	}
	if ffi.useProject && projectInfo != nil {
		sourcePath := filepath.Join(projectInfo.RootPath, "ffi."+target+".mjs")
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outputDir, "ffi.project."+target+".mjs"), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func emitBundle(root checker.Module, target string, options emitOptions, rootFileName string) (map[string][]byte, ffiArtifacts, error) {
	modules, err := collectImportedModules(root)
	if err != nil {
		return nil, ffiArtifacts{}, err
	}

	allModules := make([]checker.Module, 0, len(modules)+1)
	allModules = append(allModules, modules...)
	allModules = append(allModules, root)
	ffi := collectFFIArtifacts(root, modules)
	files := make(map[string][]byte, len(allModules))
	for _, module := range allModules {
		outputPath := rootFileName
		if module != root {
			outputPath = moduleOutputPath(module.Path())
		}
		source, err := emitModuleFile(module, target, outputPath, options.invokeMain && module == root)
		if err != nil {
			return nil, ffiArtifacts{}, err
		}
		files[outputPath] = source
	}
	return files, ffi, nil
}

func emitModuleFile(module checker.Module, target string, outputPath string, invokeMain bool) ([]byte, error) {
	ffi := moduleFFIArtifacts(module)
	tempCounter := 0
	e := &emitter{
		target:            target,
		moduleVars:        make(map[string]string),
		usedEnumMethods:   collectUsedEnumMethods(module.Program()),
		tempCounter:       &tempCounter,
		ffi:               ffi,
		currentOutputPath: outputPath,
	}

	imports := directImportedModules(module)
	for _, imported := range imports {
		e.moduleVars[imported.Path()] = moduleAlias(imported.Path())
	}

	preludeImport := relativeJSImport(outputPath, "ard.prelude.mjs")
	e.line(`import { Maybe, Result, ardEnumValue, ardEq, ardToString, isArdEnum, isArdMaybe, isEnumOf, makeArdError, makeBreakSignal, makeEnum } from ` + strconv.Quote(preludeImport) + `;`)
	if (target == backend.TargetJSServer || target == backend.TargetJSBrowser) && ffi.useStdlib {
		e.line(`import * as stdlib from ` + strconv.Quote(relativeJSImport(outputPath, "ffi.stdlib."+target+".mjs")) + `;`)
	}
	if (target == backend.TargetJSServer || target == backend.TargetJSBrowser) && ffi.useProject {
		e.line(`import * as project from ` + strconv.Quote(relativeJSImport(outputPath, "ffi.project."+target+".mjs")) + `;`)
	}
	for _, imported := range imports {
		e.line(`import * as ` + moduleAlias(imported.Path()) + ` from ` + strconv.Quote(relativeJSImport(outputPath, moduleOutputPath(imported.Path()))) + `;`)
	}
	e.line("")
	e.line("// Generated by Ard JavaScript backend (early preview).")
	e.line("// Target: " + target)
	e.line("")

	if err := e.emitRootModule(module); err != nil {
		return nil, err
	}

	if invokeMain {
		if !moduleHasPublicOrPrivateFunction(module.Program(), "main") {
			return nil, fmt.Errorf("js-server run requires fn main()")
		}
		if !moduleCallsTopLevelFunction(module.Program(), "main") {
			e.line("")
			if target == backend.TargetJSServer {
				e.line("await main();")
			} else {
				e.line("main();")
			}
		}
	}

	return []byte(e.builder.String()), nil
}

func shouldEmitImportedModule(path string) bool {
	switch path {
	case "ard/float", "ard/int", "ard/list", "ard/map", "ard/string":
		return false
	default:
		return true
	}
}

func collectImportedModules(root checker.Module) ([]checker.Module, error) {
	seen := map[string]bool{}
	ordered := make([]checker.Module, 0)
	var visit func(module checker.Module) error
	visit = func(module checker.Module) error {
		if module == nil || module.Program() == nil {
			return nil
		}
		for _, imported := range module.Program().Imports {
			if imported == nil || imported.Program() == nil || !shouldEmitImportedModule(imported.Path()) {
				continue
			}
			if seen[imported.Path()] {
				continue
			}
			seen[imported.Path()] = true
			if err := visit(imported); err != nil {
				return err
			}
			ordered = append(ordered, imported)
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	return ordered, nil
}

func collectFFIArtifacts(root checker.Module, modules []checker.Module) ffiArtifacts {
	ffi := ffiArtifacts{}
	mark := func(module checker.Module) {
		moduleFFI := moduleFFIArtifacts(module)
		ffi.useStdlib = ffi.useStdlib || moduleFFI.useStdlib
		ffi.useProject = ffi.useProject || moduleFFI.useProject
	}
	mark(root)
	for _, module := range modules {
		mark(module)
	}
	return ffi
}

func moduleFFIArtifacts(module checker.Module) ffiArtifacts {
	ffi := ffiArtifacts{}
	if module == nil || module.Program() == nil {
		return ffi
	}
	for _, stmt := range module.Program().Statements {
		if ext, ok := stmt.Expr.(*checker.ExternalFunctionDef); ok && ext.ExternalBinding != "" {
			if strings.HasPrefix(module.Path(), "ard/") {
				ffi.useStdlib = true
			} else {
				ffi.useProject = true
			}
		}
	}
	return ffi
}

func directImportedModules(module checker.Module) []checker.Module {
	if module == nil || module.Program() == nil {
		return nil
	}
	imports := make([]checker.Module, 0, len(module.Program().Imports))
	for _, imported := range module.Program().Imports {
		if imported == nil || imported.Program() == nil || !shouldEmitImportedModule(imported.Path()) {
			continue
		}
		imports = append(imports, imported)
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i].Path() < imports[j].Path() })
	return imports
}

func moduleHasPublicOrPrivateFunction(program *checker.Program, name string) bool {
	for _, stmt := range program.Statements {
		fn, ok := stmt.Expr.(*checker.FunctionDef)
		if ok && fn.Name == name && fn.Receiver == "" {
			return true
		}
	}
	return false
}

func moduleCallsTopLevelFunction(program *checker.Program, name string) bool {
	for _, stmt := range program.Statements {
		call, ok := stmt.Expr.(*checker.FunctionCall)
		if ok && call.Name == name {
			return true
		}
	}
	return false
}

func (e *emitter) emitRootModule(module checker.Module) error {
	if err := e.emitModuleStatements(module); err != nil {
		return err
	}
	exports := exportedNames(module.Program())
	if len(exports) == 0 {
		return nil
	}
	mangled := make([]string, 0, len(exports))
	for _, name := range exports {
		mangled = append(mangled, jsName(name))
	}
	e.line("")
	e.line("export { " + strings.Join(mangled, ", ") + " };")
	return nil
}

func exportedNames(program *checker.Program) []string {
	names := make([]string, 0)
	seen := map[string]bool{}
	for _, enum := range collectEnumDefs(program) {
		if enum.Private || seen[enum.Name] {
			continue
		}
		seen[enum.Name] = true
		names = append(names, enum.Name)
	}
	for _, stmt := range program.Statements {
		switch expr := stmt.Expr.(type) {
		case *checker.FunctionDef:
			if expr.Private || expr.Receiver != "" || seen[expr.Name] {
				continue
			}
			seen[expr.Name] = true
			names = append(names, expr.Name)
		case *checker.ExternalFunctionDef:
			if expr.Private || seen[expr.Name] {
				continue
			}
			seen[expr.Name] = true
			names = append(names, expr.Name)
		}
		switch def := stmt.Stmt.(type) {
		case *checker.VariableDef:
			if def.Mutable || seen[def.Name] {
				continue
			}
			seen[def.Name] = true
			names = append(names, def.Name)
		case *checker.StructDef:
			if def.Private || seen[def.Name] {
				continue
			}
			seen[def.Name] = true
			names = append(names, def.Name)
		case *checker.Enum:
			if def.Private || seen[def.Name] {
				continue
			}
			seen[def.Name] = true
			names = append(names, def.Name)
		case checker.Enum:
			if def.Private || seen[def.Name] {
				continue
			}
			seen[def.Name] = true
			names = append(names, def.Name)
		}
	}
	sort.Strings(names)
	return names
}

func (e *emitter) emitModuleStatements(module checker.Module) error {
	prevModule := e.currentModule
	e.currentModule = module.Path()
	defer func() { e.currentModule = prevModule }()

	emittedStructs := map[string]bool{}
	emittedEnums := map[string]bool{}
	for _, enum := range collectEnumDefs(module.Program()) {
		if emittedEnums[enum.Name] {
			continue
		}
		emittedEnums[enum.Name] = true
		if err := e.emitEnumDef(enum); err != nil {
			return err
		}
	}
	for _, stmt := range module.Program().Statements {
		skip := false
		switch def := stmt.Stmt.(type) {
		case *checker.StructDef:
			if emittedStructs[def.Name] {
				skip = true
			} else {
				emittedStructs[def.Name] = true
			}
		case *checker.Enum:
			if emittedEnums[def.Name] {
				skip = true
			}
		case checker.Enum:
			if emittedEnums[def.Name] {
				skip = true
			}
		}
		if skip {
			continue
		}
		if err := e.emitTopLevelStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitTopLevelStatement(stmt checker.Statement) error {
	if stmt.Break {
		return fmt.Errorf("break is not valid at the top level")
	}
	if stmt.Stmt != nil {
		return e.emitNonProducing(stmt.Stmt)
	}
	if stmt.Expr == nil {
		return nil
	}
	switch expr := stmt.Expr.(type) {
	case *checker.FunctionDef:
		return e.emitFunctionDef(expr)
	case *checker.ExternalFunctionDef:
		return e.emitExternalFunctionDef(expr)
	default:
		value, err := e.emitExpr(expr)
		if err != nil {
			return err
		}
		e.line(value + ";")
		return nil
	}
}

func (e *emitter) emitFunctionDef(fn *checker.FunctionDef) error {
	if fn.Receiver != "" {
		return nil
	}
	params := make([]string, 0, len(fn.Parameters))
	for _, param := range fn.Parameters {
		params = append(params, jsName(param.Name))
	}
	e.line(fmt.Sprintf("function %s(%s) {", jsName(fn.Name), strings.Join(params, ", ")))
	prevFunction := e.currentFunction
	prevReturnType := e.currentReturnType
	e.currentFunction = fn.Name
	e.currentReturnType = fn.ReturnType
	e.indent(func() {
		err := e.emitFunctionBoundary(fn.Body)
		if err != nil {
			panic(err)
		}
	})
	e.currentFunction = prevFunction
	e.currentReturnType = prevReturnType
	e.line("}")
	e.line("")
	return nil
}

func (e *emitter) emitExternalFunctionDef(fn *checker.ExternalFunctionDef) error {
	params := make([]string, 0, len(fn.Parameters))
	for _, param := range fn.Parameters {
		params = append(params, jsName(param.Name))
	}
	e.line(fmt.Sprintf("function %s(%s) {", jsName(fn.Name), strings.Join(params, ", ")))
	e.indent(func() {
		ffiObject := e.externFFIObject()
		if ffiObject == "" || fn.ExternalBinding == "" {
			message := strconv.Quote("external function not implemented for JavaScript backend: " + fn.ExternalBinding)
			e.line("throw makeArdError(\"extern\", " + strconv.Quote(e.currentModule) + ", " + strconv.Quote(fn.Name) + ", 0, " + message + ");")
			return
		}
		call := e.externMemberExpr(ffiObject, fn.ExternalBinding) + "(" + strings.Join(params, ", ") + ")"
		adapted, err := e.emitExternalReturn(call, fn.ReturnType)
		if err != nil {
			panic(err)
		}
		e.line("return " + adapted + ";")
	})
	e.line("}")
	e.line("")
	return nil
}

func (e *emitter) externFFIObject() string {
	if strings.HasPrefix(e.currentModule, "ard/") {
		if e.ffi.useStdlib {
			return "stdlib"
		}
		return ""
	}
	if e.ffi.useProject {
		return "project"
	}
	return ""
}

func (e *emitter) externMemberExpr(objectName string, binding string) string {
	if isJSIdentifier(binding) {
		return objectName + "." + binding
	}
	return objectName + "[" + strconv.Quote(binding) + "]"
}

func (e *emitter) emitExternalReturn(call string, returnType checker.Type) (string, error) {
	switch typed := returnType.(type) {
	case *checker.Maybe:
		adapted, err := e.emitExternalValueAdapter("__extern", typed.Of())
		if err != nil {
			return "", err
		}
		return "(() => { const __extern = " + call + "; return (__extern === undefined || __extern === null) ? Maybe.none() : Maybe.some(" + adapted + "); })()", nil
	case *checker.Result:
		adaptedOk, err := e.emitExternalValueAdapter("__extern.ok", typed.Val())
		if err != nil {
			return "", err
		}
		adaptedErr, err := e.emitExternalValueAdapter("__extern.error", typed.Err())
		if err != nil {
			return "", err
		}
		adaptedAltErr, err := e.emitExternalValueAdapter("__extern.err", typed.Err())
		if err != nil {
			return "", err
		}
		return "(() => { const __extern = " + call + "; if (__extern && Object.prototype.hasOwnProperty.call(__extern, \"ok\")) return Result.ok(" + adaptedOk + "); if (__extern && Object.prototype.hasOwnProperty.call(__extern, \"error\")) return Result.err(" + adaptedErr + "); if (__extern && Object.prototype.hasOwnProperty.call(__extern, \"err\")) return Result.err(" + adaptedAltErr + "); throw makeArdError(\"extern\", " + strconv.Quote(e.currentModule) + ", " + strconv.Quote(e.currentFunction) + ", 0, \"invalid Result return from JS extern\"); })()", nil
	default:
		return call, nil
	}
}

func (e *emitter) emitExternalValueAdapter(value string, t checker.Type) (string, error) {
	if t == nil {
		return value, nil
	}
	switch typed := t.(type) {
	case *checker.TypeVar:
		if typed.Actual() != nil {
			return e.emitExternalValueAdapter(value, typed.Actual())
		}
		return value, nil
	case *checker.StructDef:
		fieldNames := sortedFieldNames(typed.Fields)
		args := make([]string, 0, len(fieldNames))
		for _, field := range fieldNames {
			adapted, err := e.emitExternalValueAdapter(value+"["+strconv.Quote(field)+"]", typed.Fields[field])
			if err != nil {
				return "", err
			}
			args = append(args, adapted)
		}
		return "(" + value + " instanceof " + jsName(typed.Name) + " ? " + value + " : new " + jsName(typed.Name) + "(" + strings.Join(args, ", ") + "))", nil
	case *checker.List:
		adapted, err := e.emitExternalValueAdapter("__item", typed.Of())
		if err != nil {
			return "", err
		}
		return "Array.isArray(" + value + ") ? " + value + ".map((__item) => " + adapted + ") : []", nil
	case *checker.Map:
		adaptedKey, err := e.emitExternalValueAdapter("__key", typed.Key())
		if err != nil {
			return "", err
		}
		adaptedVal, err := e.emitExternalValueAdapter("__value", typed.Value())
		if err != nil {
			return "", err
		}
		return "(() => { const __map = " + value + "; if (__map instanceof Map) return new Map(Array.from(__map.entries(), ([__key, __value]) => [" + adaptedKey + ", " + adaptedVal + "])); return new Map(Object.entries(__map ?? {}).map(([__key, __value]) => [" + adaptedKey + ", " + adaptedVal + "])); })()", nil
	case *checker.Maybe:
		adapted, err := e.emitExternalValueAdapter("__maybe", typed.Of())
		if err != nil {
			return "", err
		}
		return "(() => { const __maybe = " + value + "; return (__maybe === undefined || __maybe === null) ? Maybe.none() : Maybe.some(" + adapted + "); })()", nil
	case *checker.Result:
		return value, nil
	default:
		return value, nil
	}
}

func (e *emitter) childEmitter() *emitter {
	return &emitter{
		target:              e.target,
		moduleVars:          e.moduleVars,
		usedEnumMethods:     e.usedEnumMethods,
		tempCounter:         e.tempCounter,
		currentModule:       e.currentModule,
		currentOutputPath:   e.currentOutputPath,
		currentFunction:     e.currentFunction,
		currentReceiver:     e.currentReceiver,
		currentReceiverExpr: e.currentReceiverExpr,
		currentReturnType:   e.currentReturnType,
		ffi:                 e.ffi,
		loopDepth:           e.loopDepth,
		signalBreaks:        e.signalBreaks || e.loopDepth > 0,
	}
}

func (e *emitter) temp(prefix string) string {
	id := *e.tempCounter
	*e.tempCounter = *e.tempCounter + 1
	return "__" + prefix + strconv.Itoa(id)
}

func (e *emitter) captureOutput(fn func(child *emitter) error) (string, error) {
	child := e.childEmitter()
	if err := fn(child); err != nil {
		return "", err
	}
	return strings.TrimRight(child.builder.String(), "\n"), nil
}

func (e *emitter) emitCaptured(text string) {
	if text == "" {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		e.line(line)
	}
}

func (e *emitter) emitCapturedStatements(stmts []string) {
	for _, stmt := range stmts {
		e.emitCaptured(stmt)
	}
}

func (e *emitter) emitFunctionBoundary(body *checker.Block) error {
	return e.emitBlock(body, true)
}

func (e *emitter) emitBlock(block *checker.Block, returns bool) error {
	if block == nil {
		if returns {
			e.line("return undefined;")
		}
		return nil
	}
	lastNonEmpty := -1
	for i := len(block.Stmts) - 1; i >= 0; i-- {
		stmt := block.Stmts[i]
		if stmt.Break || stmt.Stmt != nil || stmt.Expr != nil {
			lastNonEmpty = i
			break
		}
	}
	for i, stmt := range block.Stmts {
		if stmt.Break {
			if e.signalBreaks {
				e.line("throw makeBreakSignal();")
			} else {
				e.line("break;")
			}
			continue
		}
		if stmt.Stmt != nil {
			if err := e.emitNonProducing(stmt.Stmt); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if returns && i == lastNonEmpty {
			if err := e.emitTailExpr(stmt.Expr); err != nil {
				return err
			}
			continue
		}
		lowered, err := e.lowerExpr(stmt.Expr)
		if err != nil {
			return err
		}
		e.emitCapturedStatements(lowered.stmts)
		e.line(lowered.expr + ";")
	}
	if returns && (lastNonEmpty == -1 || block.Stmts[lastNonEmpty].Expr == nil) {
		e.line("return undefined;")
	}
	return nil
}

func (e *emitter) emitBlockInto(block *checker.Block, dest string) error {
	if block == nil {
		e.line(dest + " = undefined;")
		return nil
	}
	lastNonEmpty := -1
	for i := len(block.Stmts) - 1; i >= 0; i-- {
		stmt := block.Stmts[i]
		if stmt.Break || stmt.Stmt != nil || stmt.Expr != nil {
			lastNonEmpty = i
			break
		}
	}
	for i, stmt := range block.Stmts {
		if stmt.Break {
			if e.signalBreaks {
				e.line("throw makeBreakSignal();")
			} else {
				e.line("break;")
			}
			continue
		}
		if stmt.Stmt != nil {
			if err := e.emitNonProducing(stmt.Stmt); err != nil {
				return err
			}
			continue
		}
		if stmt.Expr == nil {
			continue
		}
		if i == lastNonEmpty {
			if err := e.emitExprInto(stmt.Expr, dest); err != nil {
				return err
			}
			continue
		}
		lowered, err := e.lowerExpr(stmt.Expr)
		if err != nil {
			return err
		}
		e.emitCapturedStatements(lowered.stmts)
		e.line(lowered.expr + ";")
	}
	if lastNonEmpty == -1 || block.Stmts[lastNonEmpty].Expr == nil {
		e.line(dest + " = undefined;")
	}
	return nil
}

func (e *emitter) emitTailExpr(expr checker.Expression) error {
	if containsTry(expr) {
		lowered, err := e.lowerExpr(expr)
		if err != nil {
			return err
		}
		e.emitCapturedStatements(lowered.stmts)
		e.line("return " + lowered.expr + ";")
		return nil
	}
	switch expr := expr.(type) {
	case *checker.If:
		return e.emitIf(expr, true)
	case *checker.Block:
		return e.emitBlock(expr, true)
	case *checker.Panic:
		message, err := e.emitExpr(expr.Message)
		if err != nil {
			return err
		}
		e.line("throw makeArdError(\"panic\", " + strconv.Quote(e.currentModule) + ", " + strconv.Quote(e.currentFunction) + ", 0, " + message + ");")
		return nil
	default:
		value, err := e.emitExpr(expr)
		if err != nil {
			return err
		}
		e.line("return " + value + ";")
		return nil
	}
}

func (e *emitter) emitExprInto(expr checker.Expression, dest string) error {
	if !containsTry(expr) {
		value, err := e.emitExpr(expr)
		if err != nil {
			return err
		}
		e.line(dest + " = " + value + ";")
		return nil
	}
	switch expr := expr.(type) {
	case *checker.If:
		return e.emitIfInto(expr, dest)
	case *checker.Block:
		return e.emitBlockInto(expr, dest)
	case *checker.BoolMatch:
		return e.emitBoolMatchInto(expr, dest)
	case *checker.EnumMatch:
		return e.emitEnumMatchInto(expr, dest)
	case *checker.UnionMatch:
		return e.emitUnionMatchInto(expr, dest)
	case *checker.IntMatch:
		return e.emitIntMatchInto(expr, dest)
	case *checker.ConditionalMatch:
		return e.emitConditionalMatchInto(expr, dest)
	case *checker.OptionMatch:
		return e.emitOptionMatchInto(expr, dest)
	case *checker.ResultMatch:
		return e.emitResultMatchInto(expr, dest)
	default:
		lowered, err := e.lowerExpr(expr)
		if err != nil {
			return err
		}
		e.emitCapturedStatements(lowered.stmts)
		e.line(dest + " = " + lowered.expr + ";")
		return nil
	}
}

func (e *emitter) emitIf(expr *checker.If, returns bool) error {
	condition, err := e.emitExpr(expr.Condition)
	if err != nil {
		return err
	}
	e.line("if (" + condition + ") {")
	e.indent(func() {
		err = e.emitBlock(expr.Body, returns)
		if err != nil {
			panic(err)
		}
	})
	e.line("}")
	if expr.ElseIf != nil {
		elseIf := *expr.ElseIf
		if elseIf.Else == nil {
			elseIf.Else = expr.Else
		}
		e.line("else {")
		e.indent(func() {
			err = e.emitIf(&elseIf, returns)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
		return nil
	}
	if expr.Else != nil {
		e.line("else {")
		e.indent(func() {
			err = e.emitBlock(expr.Else, returns)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
		return nil
	}
	if returns {
		e.line("return undefined;")
	}
	return nil
}

func (e *emitter) emitIfInto(expr *checker.If, dest string) error {
	condition, err := e.lowerExpr(expr.Condition)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(condition.stmts)
	e.line("if (" + condition.expr + ") {")
	e.indent(func() {
		err = e.emitBlockInto(expr.Body, dest)
		if err != nil {
			panic(err)
		}
	})
	e.line("}")
	if expr.ElseIf != nil {
		elseIf := *expr.ElseIf
		if elseIf.Else == nil {
			elseIf.Else = expr.Else
		}
		e.line("else {")
		e.indent(func() {
			err = e.emitIfInto(&elseIf, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
		return nil
	}
	if expr.Else != nil {
		e.line("else {")
		e.indent(func() {
			err = e.emitBlockInto(expr.Else, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
		return nil
	}
	e.line(dest + " = undefined;")
	return nil
}

func (e *emitter) emitBlockIntoWithBindings(block *checker.Block, dest string, bindings []string) error {
	for _, binding := range bindings {
		e.line(binding)
	}
	return e.emitBlockInto(block, dest)
}

func (e *emitter) emitBoolMatchInto(match *checker.BoolMatch, dest string) error {
	subject, err := e.lowerExpr(match.Subject)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(subject.stmts)
	e.line("if (" + subject.expr + ") {")
	e.indent(func() {
		err = e.emitBlockInto(match.True, dest)
		if err != nil {
			panic(err)
		}
	})
	e.line("} else {")
	e.indent(func() {
		err = e.emitBlockInto(match.False, dest)
		if err != nil {
			panic(err)
		}
	})
	e.line("}")
	return nil
}

func (e *emitter) emitEnumMatchInto(match *checker.EnumMatch, dest string) error {
	subject, err := e.lowerExpr(match.Subject)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(subject.stmts)
	matchVar := e.temp("match")
	e.line("const " + matchVar + " = " + subject.expr + ";")
	enumName, err := enumTypeName(match.Subject.Type())
	if err != nil {
		return err
	}
	first := true
	for _, discriminant := range sortedEnumDiscriminants(match.DiscriminantToIndex) {
		idx := match.DiscriminantToIndex[discriminant]
		if idx < 0 || int(idx) >= len(match.Cases) || match.Cases[idx] == nil {
			continue
		}
		prefix := "if"
		if !first {
			prefix = "else if"
		}
		first = false
		e.line(prefix + " (isEnumOf(" + matchVar + ", " + strconv.Quote(enumName) + ") && " + matchVar + ".value === " + strconv.Itoa(discriminant) + ") {")
		e.indent(func() {
			err = e.emitBlockInto(match.Cases[idx], dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	}
	if match.CatchAll != nil {
		if first {
			e.line("if (true) {")
		} else {
			e.line("else {")
		}
		e.indent(func() {
			err = e.emitBlockInto(match.CatchAll, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	} else {
		e.line(`else { throw makeArdError("panic", "match", "enum", 0, "non-exhaustive enum match"); }`)
	}
	return nil
}

func (e *emitter) emitUnionMatchInto(match *checker.UnionMatch, dest string) error {
	subject, err := e.lowerExpr(match.Subject)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(subject.stmts)
	matchVar := e.temp("match")
	e.line("const " + matchVar + " = " + subject.expr + ";")
	first := true
	for _, caseName := range sortedUnionCaseNames(match.TypeCases) {
		matchCase := match.TypeCases[caseName]
		if matchCase == nil {
			continue
		}
		caseType := unionCaseType(match.TypeCasesByType, caseName)
		if caseType == nil {
			return fmt.Errorf("missing union case type for %s", caseName)
		}
		predicate, err := e.emitUnionTypePredicate(caseType, matchVar)
		if err != nil {
			return err
		}
		prefix := "if"
		if !first {
			prefix = "else if"
		}
		first = false
		bindings := []string{}
		if matchCase.Pattern != nil {
			bindings = append(bindings, "const "+jsName(matchCase.Pattern.Name)+" = "+matchVar+";")
		}
		e.line(prefix + " (" + predicate + ") {")
		e.indent(func() {
			err = e.emitBlockIntoWithBindings(matchCase.Body, dest, bindings)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	}
	if match.CatchAll != nil {
		if first {
			e.line("if (true) {")
		} else {
			e.line("else {")
		}
		e.indent(func() {
			err = e.emitBlockInto(match.CatchAll, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	} else {
		if first {
			e.line(`throw makeArdError("panic", "match", "union", 0, "non-exhaustive union match");`)
		} else {
			e.line(`else { throw makeArdError("panic", "match", "union", 0, "non-exhaustive union match"); }`)
		}
	}
	return nil
}

func (e *emitter) emitIntMatchInto(match *checker.IntMatch, dest string) error {
	subject, err := e.lowerExpr(match.Subject)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(subject.stmts)
	matchVar := e.temp("match")
	e.line("const " + matchVar + " = " + subject.expr + ";")
	first := true
	for _, value := range sortedIntCaseKeys(match.IntCases) {
		prefix := "if"
		if !first {
			prefix = "else if"
		}
		first = false
		block := match.IntCases[value]
		e.line(fmt.Sprintf("%s (%s === %d) {", prefix, matchVar, value))
		e.indent(func() {
			err = e.emitBlockInto(block, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	}
	for _, intRange := range sortedIntRangeKeys(match.RangeCases) {
		prefix := "if"
		if !first {
			prefix = "else if"
		}
		first = false
		block := match.RangeCases[intRange]
		e.line(fmt.Sprintf("%s (%s >= %d && %s <= %d) {", prefix, matchVar, intRange.Start, matchVar, intRange.End))
		e.indent(func() {
			err = e.emitBlockInto(block, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	}
	if match.CatchAll != nil {
		if first {
			e.line("if (true) {")
		} else {
			e.line("else {")
		}
		e.indent(func() {
			err = e.emitBlockInto(match.CatchAll, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	} else {
		if first {
			e.line(`throw makeArdError("panic", "match", "int", 0, "non-exhaustive int match");`)
		} else {
			e.line(`else { throw makeArdError("panic", "match", "int", 0, "non-exhaustive int match"); }`)
		}
	}
	return nil
}

func (e *emitter) emitConditionalMatchInto(match *checker.ConditionalMatch, dest string) error {
	first := true
	var err error
	for _, matchCase := range match.Cases {
		condition, err := e.lowerExpr(matchCase.Condition)
		if err != nil {
			return err
		}
		e.emitCapturedStatements(condition.stmts)
		prefix := "if"
		if !first {
			prefix = "else if"
		}
		first = false
		e.line(prefix + " (" + condition.expr + ") {")
		e.indent(func() {
			err = e.emitBlockInto(matchCase.Body, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	}
	if match.CatchAll != nil {
		if first {
			e.line("if (true) {")
		} else {
			e.line("else {")
		}
		e.indent(func() {
			err = e.emitBlockInto(match.CatchAll, dest)
			if err != nil {
				panic(err)
			}
		})
		e.line("}")
	} else {
		if first {
			e.line(`throw makeArdError("panic", "match", "conditional", 0, "non-exhaustive conditional match");`)
		} else {
			e.line(`else { throw makeArdError("panic", "match", "conditional", 0, "non-exhaustive conditional match"); }`)
		}
	}
	return nil
}

func (e *emitter) emitOptionMatchInto(match *checker.OptionMatch, dest string) error {
	subject, err := e.lowerExpr(match.Subject)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(subject.stmts)
	matchVar := e.temp("match")
	e.line("const " + matchVar + " = " + subject.expr + ";")
	e.line("if (" + matchVar + ".isSome()) {")
	e.indent(func() {
		bindings := []string{}
		if match.Some != nil && match.Some.Pattern != nil {
			bindings = append(bindings, "const "+jsName(match.Some.Pattern.Name)+" = "+matchVar+".value;")
		}
		err = e.emitBlockIntoWithBindings(match.Some.Body, dest, bindings)
		if err != nil {
			panic(err)
		}
	})
	e.line("} else {")
	e.indent(func() {
		err = e.emitBlockInto(match.None, dest)
		if err != nil {
			panic(err)
		}
	})
	e.line("}")
	return nil
}

func (e *emitter) emitResultMatchInto(match *checker.ResultMatch, dest string) error {
	subject, err := e.lowerExpr(match.Subject)
	if err != nil {
		return err
	}
	e.emitCapturedStatements(subject.stmts)
	matchVar := e.temp("match")
	e.line("const " + matchVar + " = " + subject.expr + ";")
	e.line("if (" + matchVar + ".isOk()) {")
	e.indent(func() {
		bindings := []string{}
		if match.Ok != nil && match.Ok.Pattern != nil {
			bindings = append(bindings, "const "+jsName(match.Ok.Pattern.Name)+" = "+matchVar+".ok;")
		}
		err = e.emitBlockIntoWithBindings(match.Ok.Body, dest, bindings)
		if err != nil {
			panic(err)
		}
	})
	e.line("} else {")
	e.indent(func() {
		bindings := []string{}
		if match.Err != nil && match.Err.Pattern != nil {
			bindings = append(bindings, "const "+jsName(match.Err.Pattern.Name)+" = "+matchVar+".error;")
		}
		err = e.emitBlockIntoWithBindings(match.Err.Body, dest, bindings)
		if err != nil {
			panic(err)
		}
	})
	e.line("}")
	return nil
}

func (e *emitter) lowerBranchyExpr(expr checker.Expression, prefix string) (loweredExpr, error) {
	temp := e.temp(prefix)
	block, err := e.captureOutput(func(child *emitter) error {
		child.line("let " + temp + ";")
		return child.emitExprInto(expr, temp)
	})
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := []string{}
	if block != "" {
		stmts = append(stmts, block)
	}
	return loweredExpr{stmts: stmts, expr: temp}, nil
}

func (e *emitter) lowerArgs(args []checker.Expression) ([]string, []string, error) {
	stmts := []string{}
	values := make([]string, 0, len(args))
	for _, arg := range args {
		lowered, err := e.lowerExpr(arg)
		if err != nil {
			return nil, nil, err
		}
		stmts = append(stmts, lowered.stmts...)
		values = append(values, lowered.expr)
	}
	return stmts, values, nil
}

func (e *emitter) emitStrMethodValue(kind checker.StrMethodKind, subject string, args []string) (string, error) {
	switch kind {
	case checker.StrSize:
		return subject + ".length", nil
	case checker.StrIsEmpty:
		return "(" + subject + ".length === 0)", nil
	case checker.StrContains:
		if len(args) != 1 {
			return "", fmt.Errorf("str.contains expects one arg")
		}
		return subject + ".includes(" + args[0] + ")", nil
	case checker.StrReplace:
		if len(args) != 2 {
			return "", fmt.Errorf("str.replace expects two args")
		}
		return subject + ".replace(" + args[0] + ", " + args[1] + ")", nil
	case checker.StrReplaceAll:
		if len(args) != 2 {
			return "", fmt.Errorf("str.replace_all expects two args")
		}
		return subject + ".replaceAll(" + args[0] + ", " + args[1] + ")", nil
	case checker.StrSplit:
		if len(args) != 1 {
			return "", fmt.Errorf("str.split expects one arg")
		}
		return subject + ".split(" + args[0] + ")", nil
	case checker.StrStartsWith:
		if len(args) != 1 {
			return "", fmt.Errorf("str.starts_with expects one arg")
		}
		return subject + ".startsWith(" + args[0] + ")", nil
	case checker.StrToStr:
		return subject, nil
	case checker.StrTrim:
		return subject + ".trim()", nil
	default:
		return "", fmt.Errorf("unsupported str method: %v", kind)
	}
}

func (e *emitter) emitIntMethodValue(kind checker.IntMethodKind, subject string) (string, error) {
	switch kind {
	case checker.IntToStr:
		return "String(" + subject + ")", nil
	default:
		return "", fmt.Errorf("unsupported int method: %v", kind)
	}
}

func (e *emitter) emitFloatMethodValue(kind checker.FloatMethodKind, subject string) (string, error) {
	switch kind {
	case checker.FloatToStr:
		return "(" + subject + ").toFixed(2)", nil
	case checker.FloatToInt:
		return "Math.trunc(" + subject + ")", nil
	default:
		return "", fmt.Errorf("unsupported float method: %v", kind)
	}
}

func (e *emitter) emitBoolMethodValue(kind checker.BoolMethodKind, subject string) (string, error) {
	switch kind {
	case checker.BoolToStr:
		return "String(" + subject + ")", nil
	default:
		return "", fmt.Errorf("unsupported bool method: %v", kind)
	}
}

func (e *emitter) emitMaybeModuleCallValue(name string, args []string) (string, error) {
	switch name {
	case "some":
		if len(args) != 1 {
			return "", fmt.Errorf("maybe::some expects one arg")
		}
		return "Maybe.some(" + args[0] + ")", nil
	case "none":
		return "Maybe.none()", nil
	default:
		return "", fmt.Errorf("unsupported maybe module call: %s", name)
	}
}

func (e *emitter) emitResultModuleCallValue(name string, args []string) (string, error) {
	switch name {
	case "ok":
		if len(args) != 1 {
			return "", fmt.Errorf("Result::ok expects one arg")
		}
		return "Result.ok(" + args[0] + ")", nil
	case "err":
		if len(args) != 1 {
			return "", fmt.Errorf("Result::err expects one arg")
		}
		return "Result.err(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported Result module call: %s", name)
	}
}

func (e *emitter) emitFloatModuleCallValue(name string, args []string) (string, error) {
	switch name {
	case "from_int":
		if len(args) != 1 {
			return "", fmt.Errorf("Float::from_int expects one arg")
		}
		return "Number(" + args[0] + ")", nil
	case "from_str":
		if len(args) != 1 {
			return "", fmt.Errorf("Float::from_str expects one arg")
		}
		return "(() => { const __input = String(" + args[0] + ").trim(); if (__input === \"\") return Maybe.none(); const __value = Number(__input); return Number.isNaN(__value) ? Maybe.none() : Maybe.some(__value); })()", nil
	case "floor":
		if len(args) != 1 {
			return "", fmt.Errorf("Float::floor expects one arg")
		}
		return "Math.floor(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported Float module call: %s", name)
	}
}

func (e *emitter) emitIntModuleCallValue(name string, args []string) (string, error) {
	switch name {
	case "from_str":
		if len(args) != 1 {
			return "", fmt.Errorf("Int::from_str expects one arg")
		}
		return "(() => { const __input = String(" + args[0] + ").trim(); if (!/^[-+]?\\d+$/.test(__input)) return Maybe.none(); return Maybe.some(Number.parseInt(__input, 10)); })()", nil
	default:
		return "", fmt.Errorf("unsupported Int module call: %s", name)
	}
}

func (e *emitter) emitListModuleCallValue(name string, args []string) (string, error) {
	switch name {
	case "new":
		return "[]", nil
	case "concat":
		if len(args) != 2 {
			return "", fmt.Errorf("List::concat expects two args")
		}
		return "(" + args[0] + ").concat(" + args[1] + ")", nil
	default:
		return "", fmt.Errorf("unsupported List module call: %s", name)
	}
}

func (e *emitter) emitListMethodValue(kind checker.ListMethodKind, subject string, args []string) (string, error) {
	switch kind {
	case checker.ListSize:
		return subject + ".length", nil
	case checker.ListAt:
		if len(args) != 1 {
			return "", fmt.Errorf("list.at expects one arg")
		}
		return subject + "[" + args[0] + "]", nil
	case checker.ListPush:
		if len(args) != 1 {
			return "", fmt.Errorf("list.push expects one arg")
		}
		return e.emitMutationExpr(subject, []string{"__value.push(" + args[0] + ");"}, "__value")
	case checker.ListPrepend:
		if len(args) != 1 {
			return "", fmt.Errorf("list.prepend expects one arg")
		}
		return e.emitMutationExpr(subject, []string{"__value.unshift(" + args[0] + ");"}, "__value")
	case checker.ListSet:
		if len(args) != 2 {
			return "", fmt.Errorf("list.set expects two args")
		}
		return e.emitMutationExpr(subject, []string{"__value[" + args[0] + "] = " + args[1] + ";"}, "true")
	case checker.ListSwap:
		if len(args) != 2 {
			return "", fmt.Errorf("list.swap expects two args")
		}
		lines := []string{"const __tmp = __value[" + args[0] + "];", "__value[" + args[0] + "] = __value[" + args[1] + "];", "__value[" + args[1] + "] = __tmp;"}
		return e.emitMutationExpr(subject, lines, "undefined")
	case checker.ListSort:
		if len(args) != 1 {
			return "", fmt.Errorf("list.sort expects one arg")
		}
		cmp := args[0]
		lines := []string{"__value.sort((a, b) => " + cmp + "(a, b) ? -1 : (" + cmp + "(b, a) ? 1 : 0));"}
		return e.emitMutationExpr(subject, lines, "undefined")
	default:
		return "", fmt.Errorf("unsupported list method: %v", kind)
	}
}

func (e *emitter) emitMapMethodValue(kind checker.MapMethodKind, subject string, args []string) (string, error) {
	switch kind {
	case checker.MapKeys:
		return "Array.from(" + subject + ".keys())", nil
	case checker.MapSize:
		return subject + ".size", nil
	case checker.MapGet:
		if len(args) != 1 {
			return "", fmt.Errorf("map.get expects one arg")
		}
		return "(" + subject + ".has(" + args[0] + ") ? Maybe.some(" + subject + ".get(" + args[0] + ")) : Maybe.none())", nil
	case checker.MapSet:
		if len(args) != 2 {
			return "", fmt.Errorf("map.set expects two args")
		}
		return e.emitMutationExpr(subject, []string{"__value.set(" + args[0] + ", " + args[1] + ");"}, "true")
	case checker.MapDrop:
		if len(args) != 1 {
			return "", fmt.Errorf("map.drop expects one arg")
		}
		return e.emitMutationExpr(subject, []string{"__value.delete(" + args[0] + ");"}, "undefined")
	case checker.MapHas:
		if len(args) != 1 {
			return "", fmt.Errorf("map.has expects one arg")
		}
		return subject + ".has(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported map method: %v", kind)
	}
}

func (e *emitter) emitMaybeMethodValue(kind checker.MaybeMethodKind, subject string, args []string) (string, error) {
	switch kind {
	case checker.MaybeExpect:
		if len(args) != 1 {
			return "", fmt.Errorf("maybe.expect expects 1 arg(s)")
		}
		return subject + ".expect(" + args[0] + ")", nil
	case checker.MaybeIsNone:
		return subject + ".isNone()", nil
	case checker.MaybeIsSome:
		return subject + ".isSome()", nil
	case checker.MaybeOr:
		if len(args) != 1 {
			return "", fmt.Errorf("maybe.or expects 1 arg(s)")
		}
		return subject + ".or(" + args[0] + ")", nil
	case checker.MaybeMap:
		if len(args) != 1 {
			return "", fmt.Errorf("maybe.map expects 1 arg(s)")
		}
		return subject + ".map(" + args[0] + ")", nil
	case checker.MaybeAndThen:
		if len(args) != 1 {
			return "", fmt.Errorf("maybe.and_then expects 1 arg(s)")
		}
		return subject + ".andThen(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported maybe method: %v", kind)
	}
}

func (e *emitter) emitResultMethodValue(kind checker.ResultMethodKind, subject string, args []string) (string, error) {
	switch kind {
	case checker.ResultExpect:
		if len(args) != 1 {
			return "", fmt.Errorf("result.expect expects 1 arg(s)")
		}
		return subject + ".expect(" + args[0] + ")", nil
	case checker.ResultOr:
		if len(args) != 1 {
			return "", fmt.Errorf("result.or expects 1 arg(s)")
		}
		return subject + ".or(" + args[0] + ")", nil
	case checker.ResultIsOk:
		return subject + ".isOk()", nil
	case checker.ResultIsErr:
		return subject + ".isErr()", nil
	case checker.ResultMap:
		if len(args) != 1 {
			return "", fmt.Errorf("result.map expects 1 arg(s)")
		}
		return subject + ".map(" + args[0] + ")", nil
	case checker.ResultMapErr:
		if len(args) != 1 {
			return "", fmt.Errorf("result.map_err expects 1 arg(s)")
		}
		return subject + ".mapErr(" + args[0] + ")", nil
	case checker.ResultAndThen:
		if len(args) != 1 {
			return "", fmt.Errorf("result.and_then expects 1 arg(s)")
		}
		return subject + ".andThen(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported result method: %v", kind)
	}
}

func (e *emitter) lowerTryExpr(op *checker.TryOp) (loweredExpr, error) {
	subject, err := e.lowerExpr(op.Expr())
	if err != nil {
		return loweredExpr{}, err
	}
	tryVar := e.temp("try")
	stmts := append([]string{}, subject.stmts...)
	stmts = append(stmts, "const "+tryVar+" = "+subject.expr+";")
	fail, err := e.captureOutput(func(child *emitter) error {
		switch op.Kind {
		case checker.TryResult:
			child.line("if (" + tryVar + ".isErr()) {")
			child.indent(func() {
				if op.CatchBlock != nil {
					if op.CatchVar != "" && op.CatchVar != "_" {
						child.line("const " + jsName(op.CatchVar) + " = " + tryVar + ".error;")
					}
					err = child.emitBlock(op.CatchBlock, true)
					if err != nil {
						panic(err)
					}
				} else {
					child.line("return Result.err(" + tryVar + ".error);")
				}
			})
			child.line("}")
		case checker.TryMaybe:
			child.line("if (" + tryVar + ".isNone()) {")
			child.indent(func() {
				if op.CatchBlock != nil {
					err = child.emitBlock(op.CatchBlock, true)
					if err != nil {
						panic(err)
					}
				} else {
					child.line("return Maybe.none();")
				}
			})
			child.line("}")
		default:
			return fmt.Errorf("unsupported try kind: %v", op.Kind)
		}
		return nil
	})
	if err != nil {
		return loweredExpr{}, err
	}
	if fail != "" {
		stmts = append(stmts, fail)
	}
	success := tryVar + ".ok"
	if op.Kind == checker.TryMaybe {
		success = tryVar + ".value"
	}
	return loweredExpr{stmts: stmts, expr: success}, nil
}

func (e *emitter) lowerExpr(expr checker.Expression) (loweredExpr, error) {
	if !containsTry(expr) {
		value, err := e.emitExpr(expr)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{expr: value}, nil
	}
	switch expr := expr.(type) {
	case *checker.TryOp:
		return e.lowerTryExpr(expr)
	case *checker.If:
		return e.lowerBranchyExpr(expr, "if")
	case *checker.Block:
		return e.lowerBranchyExpr(expr, "block")
	case *checker.BoolMatch:
		return e.lowerBranchyExpr(expr, "boolmatch")
	case *checker.EnumMatch:
		return e.lowerBranchyExpr(expr, "enummatch")
	case *checker.UnionMatch:
		return e.lowerBranchyExpr(expr, "unionmatch")
	case *checker.IntMatch:
		return e.lowerBranchyExpr(expr, "intmatch")
	case *checker.ConditionalMatch:
		return e.lowerBranchyExpr(expr, "condmatch")
	case *checker.OptionMatch:
		return e.lowerBranchyExpr(expr, "optionmatch")
	case *checker.ResultMatch:
		return e.lowerBranchyExpr(expr, "resultmatch")
	case *checker.StructInstance:
		fieldNames := sortedStructInstanceFields(expr)
		args := make([]string, 0, len(fieldNames))
		stmts := []string{}
		for _, field := range fieldNames {
			fieldExpr, ok := expr.Fields[field]
			if !ok || fieldExpr == nil {
				fieldType := expr.FieldTypes[field]
				if _, isMaybe := fieldType.(*checker.Maybe); isMaybe {
					args = append(args, "Maybe.none()")
					continue
				}
				return loweredExpr{}, fmt.Errorf("missing struct field value for %s", field)
			}
			lowered, err := e.lowerExpr(fieldExpr)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, lowered.stmts...)
			args = append(args, lowered.expr)
		}
		return loweredExpr{stmts: stmts, expr: "new " + jsName(expr.Name) + "(" + strings.Join(args, ", ") + ")"}, nil
	case *checker.ModuleStructInstance:
		moduleVar, ok := e.moduleVars[expr.Module]
		if !ok {
			return loweredExpr{}, fmt.Errorf("unknown imported module %s", expr.Module)
		}
		fieldNames := sortedStructInstanceFields(expr.Property)
		args := make([]string, 0, len(fieldNames))
		stmts := []string{}
		for _, field := range fieldNames {
			fieldExpr, ok := expr.Property.Fields[field]
			if !ok || fieldExpr == nil {
				fieldType := expr.Property.FieldTypes[field]
				if _, isMaybe := fieldType.(*checker.Maybe); isMaybe {
					args = append(args, "Maybe.none()")
					continue
				}
				return loweredExpr{}, fmt.Errorf("missing struct field value for %s", field)
			}
			lowered, err := e.lowerExpr(fieldExpr)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, lowered.stmts...)
			args = append(args, lowered.expr)
		}
		return loweredExpr{stmts: stmts, expr: "new " + moduleVar + "." + jsName(expr.Property.Name) + "(" + strings.Join(args, ", ") + ")"}, nil
	case *checker.InstanceProperty:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: subject.stmts, expr: subject.expr + "." + jsName(expr.Property)}, nil
	case *checker.ListLiteral:
		stmts, args, err := e.lowerArgs(expr.Elements)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: stmts, expr: "[" + strings.Join(args, ", ") + "]"}, nil
	case *checker.MapLiteral:
		stmts := []string{}
		entries := make([]string, 0, len(expr.Keys))
		for i := range expr.Keys {
			key, err := e.lowerExpr(expr.Keys[i])
			if err != nil {
				return loweredExpr{}, err
			}
			val, err := e.lowerExpr(expr.Values[i])
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, key.stmts...)
			stmts = append(stmts, val.stmts...)
			entries = append(entries, "["+key.expr+", "+val.expr+"]")
		}
		return loweredExpr{stmts: stmts, expr: "new Map([" + strings.Join(entries, ", ") + "])"}, nil
	case *checker.TemplateStr:
		stmts := []string{}
		var out bytes.Buffer
		out.WriteByte('`')
		for _, chunk := range expr.Chunks {
			if literal, ok := chunk.(*checker.StrLiteral); ok {
				out.WriteString(escapeTemplateLiteral(literal.Value))
				continue
			}
			lowered, err := e.lowerExpr(chunk)
			if err != nil {
				return loweredExpr{}, err
			}
			stmts = append(stmts, lowered.stmts...)
			out.WriteString("${")
			out.WriteString(lowered.expr)
			out.WriteByte('}')
		}
		out.WriteByte('`')
		return loweredExpr{stmts: stmts, expr: out.String()}, nil
	case *checker.FunctionCall:
		stmts, args, err := e.lowerArgs(expr.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: stmts, expr: jsName(expr.Name) + "(" + strings.Join(args, ", ") + ")"}, nil
	case *checker.ModuleFunctionCall:
		stmts, args, err := e.lowerArgs(expr.Call.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		var call string
		switch expr.Module {
		case "ard/maybe":
			call, err = e.emitMaybeModuleCallValue(expr.Call.Name, args)
		case "ard/result":
			call, err = e.emitResultModuleCallValue(expr.Call.Name, args)
		case "ard/float":
			call, err = e.emitFloatModuleCallValue(expr.Call.Name, args)
		case "ard/int":
			call, err = e.emitIntModuleCallValue(expr.Call.Name, args)
		case "ard/list":
			call, err = e.emitListModuleCallValue(expr.Call.Name, args)
		default:
			moduleVar, ok := e.moduleVars[expr.Module]
			if !ok {
				return loweredExpr{}, fmt.Errorf("unknown imported module %s", expr.Module)
			}
			call = moduleVar + "." + jsName(expr.Call.Name) + "(" + strings.Join(args, ", ") + ")"
		}
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: stmts, expr: call}, nil
	case *checker.InstanceMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		argStmts, args, err := e.lowerArgs(expr.Method.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append([]string{}, subject.stmts...)
		stmts = append(stmts, argStmts...)
		if expr.ReceiverKind == checker.ReceiverTrait && expr.TraitType != nil && expr.TraitType.Name == "ToString" && expr.Method.Name == "to_str" {
			return loweredExpr{stmts: stmts, expr: "ardToString(" + subject.expr + ")"}, nil
		}
		if expr.ReceiverKind == checker.ReceiverEnum && expr.EnumType != nil {
			callArgs := append([]string{subject.expr}, args...)
			return loweredExpr{stmts: stmts, expr: enumMethodName(expr.EnumType.Name, expr.Method.Name) + "(" + strings.Join(callArgs, ", ") + ")"}, nil
		}
		return loweredExpr{stmts: stmts, expr: subject.expr + "." + jsName(expr.Method.Name) + "(" + strings.Join(args, ", ") + ")"}, nil
	case *checker.CopyExpression:
		return e.lowerExpr(expr.Expr)
	case *checker.StrMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		argStmts, args, err := e.lowerArgs(expr.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitStrMethodValue(expr.Kind, subject.expr, args)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: append(subject.stmts, argStmts...), expr: value}, nil
	case *checker.IntMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitIntMethodValue(expr.Kind, subject.expr)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: subject.stmts, expr: value}, nil
	case *checker.FloatMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitFloatMethodValue(expr.Kind, subject.expr)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: subject.stmts, expr: value}, nil
	case *checker.BoolMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitBoolMethodValue(expr.Kind, subject.expr)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: subject.stmts, expr: value}, nil
	case *checker.ListMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		argStmts, args, err := e.lowerArgs(expr.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitListMethodValue(expr.Kind, subject.expr, args)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: append(subject.stmts, argStmts...), expr: value}, nil
	case *checker.MapMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		argStmts, args, err := e.lowerArgs(expr.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitMapMethodValue(expr.Kind, subject.expr, args)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: append(subject.stmts, argStmts...), expr: value}, nil
	case *checker.MaybeMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		argStmts, args, err := e.lowerArgs(expr.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitMaybeMethodValue(expr.Kind, subject.expr, args)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: append(subject.stmts, argStmts...), expr: value}, nil
	case *checker.ResultMethod:
		subject, err := e.lowerExpr(expr.Subject)
		if err != nil {
			return loweredExpr{}, err
		}
		argStmts, args, err := e.lowerArgs(expr.Args)
		if err != nil {
			return loweredExpr{}, err
		}
		value, err := e.emitResultMethodValue(expr.Kind, subject.expr, args)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: append(subject.stmts, argStmts...), expr: value}, nil
	case *checker.IntAddition:
		return e.lowerBinary(expr.Left, "+", expr.Right)
	case *checker.IntSubtraction:
		return e.lowerBinary(expr.Left, "-", expr.Right)
	case *checker.IntMultiplication:
		return e.lowerBinary(expr.Left, "*", expr.Right)
	case *checker.IntDivision:
		left, err := e.lowerExpr(expr.Left)
		if err != nil {
			return loweredExpr{}, err
		}
		right, err := e.lowerExpr(expr.Right)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts := append([]string{}, left.stmts...)
		stmts = append(stmts, right.stmts...)
		return loweredExpr{stmts: stmts, expr: "Math.trunc((" + left.expr + ") / (" + right.expr + "))"}, nil
	case *checker.IntModulo:
		return e.lowerBinary(expr.Left, "%", expr.Right)
	case *checker.IntGreater:
		return e.lowerIntComparison(expr.Left, ">", expr.Right)
	case *checker.IntGreaterEqual:
		return e.lowerIntComparison(expr.Left, ">=", expr.Right)
	case *checker.IntLess:
		return e.lowerIntComparison(expr.Left, "<", expr.Right)
	case *checker.IntLessEqual:
		return e.lowerIntComparison(expr.Left, "<=", expr.Right)
	case *checker.FloatAddition:
		return e.lowerBinary(expr.Left, "+", expr.Right)
	case *checker.FloatSubtraction:
		return e.lowerBinary(expr.Left, "-", expr.Right)
	case *checker.FloatMultiplication:
		return e.lowerBinary(expr.Left, "*", expr.Right)
	case *checker.FloatDivision:
		return e.lowerBinary(expr.Left, "/", expr.Right)
	case *checker.FloatGreater:
		return e.lowerBinary(expr.Left, ">", expr.Right)
	case *checker.FloatGreaterEqual:
		return e.lowerBinary(expr.Left, ">=", expr.Right)
	case *checker.FloatLess:
		return e.lowerBinary(expr.Left, "<", expr.Right)
	case *checker.FloatLessEqual:
		return e.lowerBinary(expr.Left, "<=", expr.Right)
	case *checker.StrAddition:
		return e.lowerBinary(expr.Left, "+", expr.Right)
	case *checker.Equality:
		return e.lowerEquality(expr.Left, expr.Right)
	case *checker.And:
		return e.lowerBinary(expr.Left, "&&", expr.Right)
	case *checker.Or:
		return e.lowerBinary(expr.Left, "||", expr.Right)
	case *checker.Negation:
		value, err := e.lowerExpr(expr.Value)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: value.stmts, expr: "(-" + value.expr + ")"}, nil
	case *checker.Not:
		value, err := e.lowerExpr(expr.Value)
		if err != nil {
			return loweredExpr{}, err
		}
		return loweredExpr{stmts: value.stmts, expr: "(!" + value.expr + ")"}, nil
	default:
		return loweredExpr{}, fmt.Errorf("js backend does not yet support try-aware lowering for expression type %T", expr)
	}
}

func (e *emitter) lowerNumericExpr(build func() (string, error), parts ...checker.Expression) (loweredExpr, error) {
	stmts := []string{}
	for _, part := range parts {
		lowered, err := e.lowerExpr(part)
		if err != nil {
			return loweredExpr{}, err
		}
		stmts = append(stmts, lowered.stmts...)
	}
	value, err := build()
	if err != nil {
		return loweredExpr{}, err
	}
	return loweredExpr{stmts: stmts, expr: value}, nil
}

func (e *emitter) lowerBinary(left checker.Expression, op string, right checker.Expression) (loweredExpr, error) {
	leftValue, err := e.lowerExpr(left)
	if err != nil {
		return loweredExpr{}, err
	}
	rightValue, err := e.lowerExpr(right)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]string{}, leftValue.stmts...)
	stmts = append(stmts, rightValue.stmts...)
	return loweredExpr{stmts: stmts, expr: "(" + leftValue.expr + " " + op + " " + rightValue.expr + ")"}, nil
}

func (e *emitter) lowerEquality(left checker.Expression, right checker.Expression) (loweredExpr, error) {
	leftValue, err := e.lowerExpr(left)
	if err != nil {
		return loweredExpr{}, err
	}
	rightValue, err := e.lowerExpr(right)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]string{}, leftValue.stmts...)
	stmts = append(stmts, rightValue.stmts...)
	if requiresSpecialEquality(left.Type(), right.Type()) {
		return loweredExpr{stmts: stmts, expr: "ardEq(" + leftValue.expr + ", " + rightValue.expr + ")"}, nil
	}
	return loweredExpr{stmts: stmts, expr: "(" + leftValue.expr + " === " + rightValue.expr + ")"}, nil
}

func (e *emitter) lowerIntComparison(left checker.Expression, op string, right checker.Expression) (loweredExpr, error) {
	leftValue, err := e.lowerExpr(left)
	if err != nil {
		return loweredExpr{}, err
	}
	rightValue, err := e.lowerExpr(right)
	if err != nil {
		return loweredExpr{}, err
	}
	stmts := append([]string{}, leftValue.stmts...)
	stmts = append(stmts, rightValue.stmts...)
	if requiresEnumAwareComparison(left.Type(), right.Type()) {
		return loweredExpr{stmts: stmts, expr: "(ardEnumValue(" + leftValue.expr + ") " + op + " ardEnumValue(" + rightValue.expr + "))"}, nil
	}
	return loweredExpr{stmts: stmts, expr: "(" + leftValue.expr + " " + op + " " + rightValue.expr + ")"}, nil
}

func (e *emitter) emitNonProducing(stmt checker.NonProducing) error {
	switch stmt := stmt.(type) {
	case *checker.StructDef:
		return e.emitStructDef(stmt)
	case *checker.ExternType:
		return nil
	case *checker.Enum:
		return e.emitEnumDef(stmt)
	case checker.Enum:
		defCopy := stmt
		return e.emitEnumDef(&defCopy)
	case *checker.VariableDef:
		value, err := e.lowerExpr(stmt.Value)
		if err != nil {
			return err
		}
		keyword := "const"
		if stmt.Mutable {
			keyword = "let"
		}
		e.emitCapturedStatements(value.stmts)
		e.line(fmt.Sprintf("%s %s = %s;", keyword, jsName(stmt.Name), value.expr))
		return nil
	case *checker.Reassignment:
		target, err := e.emitAssignable(stmt.Target)
		if err != nil {
			return err
		}
		value, err := e.lowerExpr(stmt.Value)
		if err != nil {
			return err
		}
		e.emitCapturedStatements(value.stmts)
		e.line(target + " = " + value.expr + ";")
		return nil
	case *checker.WhileLoop:
		return e.emitWhileLoop(stmt)
	case checker.WhileLoop:
		loop := stmt
		return e.emitWhileLoop(&loop)
	case *checker.ForLoop:
		return e.emitForLoop(stmt)
	case checker.ForLoop:
		loop := stmt
		return e.emitForLoop(&loop)
	case *checker.ForIntRange:
		return e.emitForIntRange(stmt)
	case checker.ForIntRange:
		loop := stmt
		return e.emitForIntRange(&loop)
	case *checker.ForInStr:
		return e.emitForInStr(stmt)
	case checker.ForInStr:
		loop := stmt
		return e.emitForInStr(&loop)
	case *checker.ForInList:
		return e.emitForInList(stmt)
	case checker.ForInList:
		loop := stmt
		return e.emitForInList(&loop)
	case *checker.ForInMap:
		return e.emitForInMap(stmt)
	case checker.ForInMap:
		loop := stmt
		return e.emitForInMap(&loop)
	default:
		return fmt.Errorf("js backend does not yet support statement type %T", stmt)
	}
}

func (e *emitter) emitForInit(def *checker.VariableDef) (string, error) {
	if def == nil {
		return "", nil
	}
	value, err := e.emitExpr(def.Value)
	if err != nil {
		return "", err
	}
	keyword := "const"
	if def.Mutable {
		keyword = "let"
	}
	return fmt.Sprintf("%s %s = %s", keyword, jsName(def.Name), value), nil
}

func (e *emitter) emitReassignmentInline(stmt *checker.Reassignment) (string, error) {
	target, err := e.emitAssignable(stmt.Target)
	if err != nil {
		return "", err
	}
	value, err := e.emitExpr(stmt.Value)
	if err != nil {
		return "", err
	}
	return target + " = " + value, nil
}

func (e *emitter) emitWhileLoop(loop *checker.WhileLoop) error {
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	e.line("while (" + condition + ") {")
	e.indent(func() {
		e.line("try {")
		e.indent(func() {
			e.loopDepth++
			err = e.emitBlock(loop.Body, false)
			e.loopDepth--
			if err != nil {
				panic(err)
			}
		})
		e.line("} catch (__ard_break) {")
		e.indent(func() {
			e.line("if (__ard_break && __ard_break.__ard_break) break;")
			e.line("throw __ard_break;")
		})
		e.line("}")
	})
	e.line("}")
	return nil
}

func (e *emitter) emitForLoop(loop *checker.ForLoop) error {
	init, err := e.emitForInit(loop.Init)
	if err != nil {
		return err
	}
	condition, err := e.emitExpr(loop.Condition)
	if err != nil {
		return err
	}
	update, err := e.emitReassignmentInline(loop.Update)
	if err != nil {
		return err
	}
	e.line("for (" + init + "; " + condition + "; " + update + ") {")
	e.indent(func() {
		e.line("try {")
		e.indent(func() {
			e.loopDepth++
			err = e.emitBlock(loop.Body, false)
			e.loopDepth--
			if err != nil {
				panic(err)
			}
		})
		e.line("} catch (__ard_break) {")
		e.indent(func() {
			e.line("if (__ard_break && __ard_break.__ard_break) break;")
			e.line("throw __ard_break;")
		})
		e.line("}")
	})
	e.line("}")
	return nil
}

func (e *emitter) emitForIntRange(loop *checker.ForIntRange) error {
	start, err := e.emitExpr(loop.Start)
	if err != nil {
		return err
	}
	end, err := e.emitExpr(loop.End)
	if err != nil {
		return err
	}
	e.line("{")
	e.indent(func() {
		e.line("const __range_start = " + start + ";")
		e.line("const __range_end = " + end + ";")
		if loop.Index == "" {
			e.line("for (let " + jsName(loop.Cursor) + " = __range_start; " + jsName(loop.Cursor) + " <= __range_end; " + jsName(loop.Cursor) + "++) {")
		} else {
			e.line("for (let " + jsName(loop.Cursor) + " = __range_start, " + jsName(loop.Index) + " = 0; " + jsName(loop.Cursor) + " <= __range_end; " + jsName(loop.Cursor) + "++, " + jsName(loop.Index) + "++) {")
		}
		e.indent(func() {
			e.line("try {")
			e.indent(func() {
				e.loopDepth++
				err = e.emitBlock(loop.Body, false)
				e.loopDepth--
				if err != nil {
					panic(err)
				}
			})
			e.line("} catch (__ard_break) {")
			e.indent(func() {
				e.line("if (__ard_break && __ard_break.__ard_break) break;")
				e.line("throw __ard_break;")
			})
			e.line("}")
		})
		e.line("}")
	})
	e.line("}")
	return nil
}

func (e *emitter) emitForInStr(loop *checker.ForInStr) error {
	value, err := e.emitExpr(loop.Value)
	if err != nil {
		return err
	}
	e.line("{")
	e.indent(func() {
		e.line("const __string_value = Array.from(" + value + ");")
		if loop.Index == "" {
			e.line("for (const " + jsName(loop.Cursor) + " of __string_value) {")
		} else {
			e.line("for (const [" + jsName(loop.Index) + ", " + jsName(loop.Cursor) + "] of __string_value.entries()) {")
		}
		e.indent(func() {
			e.line("try {")
			e.indent(func() {
				e.loopDepth++
				err = e.emitBlock(loop.Body, false)
				e.loopDepth--
				if err != nil {
					panic(err)
				}
			})
			e.line("} catch (__ard_break) {")
			e.indent(func() {
				e.line("if (__ard_break && __ard_break.__ard_break) break;")
				e.line("throw __ard_break;")
			})
			e.line("}")
		})
		e.line("}")
	})
	e.line("}")
	return nil
}

func (e *emitter) emitForInList(loop *checker.ForInList) error {
	list, err := e.emitExpr(loop.List)
	if err != nil {
		return err
	}
	e.line("{")
	e.indent(func() {
		e.line("const __list_value = " + list + ";")
		if loop.Index == "" {
			e.line("for (const " + jsName(loop.Cursor) + " of __list_value) {")
		} else {
			e.line("for (const [" + jsName(loop.Index) + ", " + jsName(loop.Cursor) + "] of __list_value.entries()) {")
		}
		e.indent(func() {
			e.line("try {")
			e.indent(func() {
				e.loopDepth++
				err = e.emitBlock(loop.Body, false)
				e.loopDepth--
				if err != nil {
					panic(err)
				}
			})
			e.line("} catch (__ard_break) {")
			e.indent(func() {
				e.line("if (__ard_break && __ard_break.__ard_break) break;")
				e.line("throw __ard_break;")
			})
			e.line("}")
		})
		e.line("}")
	})
	e.line("}")
	return nil
}

func (e *emitter) emitForInMap(loop *checker.ForInMap) error {
	mapExpr, err := e.emitExpr(loop.Map)
	if err != nil {
		return err
	}
	e.line("{")
	e.indent(func() {
		e.line("const __map_value = " + mapExpr + ";")
		e.line("for (const [" + jsName(loop.Key) + ", " + jsName(loop.Val) + "] of __map_value.entries()) {")
		e.indent(func() {
			e.line("try {")
			e.indent(func() {
				e.loopDepth++
				err = e.emitBlock(loop.Body, false)
				e.loopDepth--
				if err != nil {
					panic(err)
				}
			})
			e.line("} catch (__ard_break) {")
			e.indent(func() {
				e.line("if (__ard_break && __ard_break.__ard_break) break;")
				e.line("throw __ard_break;")
			})
			e.line("}")
		})
		e.line("}")
	})
	e.line("}")
	return nil
}

func (e *emitter) emitAssignable(expr checker.Expression) (string, error) {
	switch expr := expr.(type) {
	case *checker.Variable:
		return e.emitVariableName(expr.Name()), nil
	case *checker.InstanceProperty:
		subject, err := e.emitExpr(expr.Subject)
		if err != nil {
			return "", err
		}
		return subject + "." + jsName(expr.Property), nil
	default:
		return "", fmt.Errorf("js backend does not yet support assignment target %T", expr)
	}
}

func (e *emitter) emitExpr(expr checker.Expression) (string, error) {
	switch expr := expr.(type) {
	case *checker.StructInstance:
		return e.emitStructInstance(expr, jsName(expr.Name))
	case *checker.ModuleStructInstance:
		moduleVar, ok := e.moduleVars[expr.Module]
		if !ok {
			return "", fmt.Errorf("unknown imported module %s", expr.Module)
		}
		return e.emitStructInstance(expr.Property, moduleVar+"."+jsName(expr.Property.Name))
	case *checker.InstanceProperty:
		subject, err := e.emitExpr(expr.Subject)
		if err != nil {
			return "", err
		}
		return subject + "." + jsName(expr.Property), nil
	case *checker.ListLiteral:
		elements, err := e.emitArgs(expr.Elements)
		if err != nil {
			return "", err
		}
		return "[" + strings.Join(elements, ", ") + "]", nil
	case *checker.MapLiteral:
		entries := make([]string, 0, len(expr.Keys))
		for i := range expr.Keys {
			key, err := e.emitExpr(expr.Keys[i])
			if err != nil {
				return "", err
			}
			value, err := e.emitExpr(expr.Values[i])
			if err != nil {
				return "", err
			}
			entries = append(entries, "["+key+", "+value+"]")
		}
		return "new Map([" + strings.Join(entries, ", ") + "])", nil
	case *checker.StrLiteral:
		return strconv.Quote(expr.Value), nil
	case *checker.TemplateStr:
		return e.emitTemplateStr(expr)
	case *checker.BoolLiteral:
		if expr.Value {
			return "true", nil
		}
		return "false", nil
	case *checker.VoidLiteral:
		return "undefined", nil
	case *checker.IntLiteral:
		return strconv.Itoa(expr.Value), nil
	case *checker.FloatLiteral:
		return strconv.FormatFloat(expr.Value, 'g', -1, 64), nil
	case *checker.Variable:
		return e.emitVariableName(expr.Name()), nil
	case *checker.ModuleSymbol:
		moduleVar, ok := e.moduleVars[expr.Module]
		if !ok {
			return "", fmt.Errorf("unknown imported module %s", expr.Module)
		}
		return moduleVar + "." + jsName(expr.Symbol.Name), nil
	case *checker.EnumVariant:
		if enum, ok := expr.Type().(*checker.Enum); ok {
			variantName := enum.Values[expr.Variant].Name
			return jsName(enum.Name) + "." + jsName(variantName), nil
		}
		return strconv.Itoa(expr.Discriminant), nil
	case *checker.FunctionDef:
		return e.emitFunctionLiteral(expr)
	case *checker.FunctionCall:
		args, err := e.emitArgs(expr.Args)
		if err != nil {
			return "", err
		}
		return jsName(expr.Name) + "(" + strings.Join(args, ", ") + ")", nil
	case *checker.ModuleFunctionCall:
		if expr.Module == "ard/maybe" {
			return e.emitMaybeModuleCall(expr)
		}
		if expr.Module == "ard/result" {
			return e.emitResultModuleCall(expr)
		}
		if expr.Module == "ard/float" {
			return e.emitFloatModuleCall(expr)
		}
		if expr.Module == "ard/int" {
			return e.emitIntModuleCall(expr)
		}
		if expr.Module == "ard/list" {
			return e.emitListModuleCall(expr)
		}
		moduleVar, ok := e.moduleVars[expr.Module]
		if !ok {
			return "", fmt.Errorf("unknown imported module %s", expr.Module)
		}
		args, err := e.emitArgs(expr.Call.Args)
		if err != nil {
			return "", err
		}
		return moduleVar + "." + jsName(expr.Call.Name) + "(" + strings.Join(args, ", ") + ")", nil
	case *checker.InstanceMethod:
		if expr.ReceiverKind == checker.ReceiverTrait && expr.TraitType != nil && expr.TraitType.Name == "ToString" && expr.Method.Name == "to_str" {
			subject, err := e.emitExpr(expr.Subject)
			if err != nil {
				return "", err
			}
			return "ardToString(" + subject + ")", nil
		}
		subject, err := e.emitExpr(expr.Subject)
		if err != nil {
			return "", err
		}
		args, err := e.emitArgs(expr.Method.Args)
		if err != nil {
			return "", err
		}
		if expr.ReceiverKind == checker.ReceiverEnum && expr.EnumType != nil {
			callArgs := append([]string{subject}, args...)
			return enumMethodName(expr.EnumType.Name, expr.Method.Name) + "(" + strings.Join(callArgs, ", ") + ")", nil
		}
		return subject + "." + jsName(expr.Method.Name) + "(" + strings.Join(args, ", ") + ")", nil
	case *checker.CopyExpression:
		return e.emitExpr(expr.Expr)
	case *checker.TryOp:
		return "", fmt.Errorf("unexpected raw try expression during JavaScript emission")
	case *checker.StrMethod:
		return e.emitStrMethod(expr)
	case *checker.IntMethod:
		return e.emitIntMethod(expr)
	case *checker.FloatMethod:
		return e.emitFloatMethod(expr)
	case *checker.BoolMethod:
		return e.emitBoolMethod(expr)
	case *checker.BoolMatch:
		return e.emitBoolMatch(expr)
	case *checker.EnumMatch:
		return e.emitEnumMatch(expr)
	case *checker.UnionMatch:
		return e.emitUnionMatch(expr)
	case *checker.IntMatch:
		return e.emitIntMatch(expr)
	case *checker.ConditionalMatch:
		return e.emitConditionalMatch(expr)
	case *checker.OptionMatch:
		return e.emitOptionMatch(expr)
	case *checker.ResultMatch:
		return e.emitResultMatch(expr)
	case *checker.ListMethod:
		return e.emitListMethod(expr)
	case *checker.MapMethod:
		return e.emitMapMethod(expr)
	case *checker.MaybeMethod:
		return e.emitMaybeMethod(expr)
	case *checker.ResultMethod:
		return e.emitResultMethod(expr)
	case *checker.IntAddition:
		return e.emitBinary(expr.Left, "+", expr.Right)
	case *checker.IntSubtraction:
		return e.emitBinary(expr.Left, "-", expr.Right)
	case *checker.IntMultiplication:
		return e.emitBinary(expr.Left, "*", expr.Right)
	case *checker.IntDivision:
		leftValue, err := e.emitExpr(expr.Left)
		if err != nil {
			return "", err
		}
		rightValue, err := e.emitExpr(expr.Right)
		if err != nil {
			return "", err
		}
		return "Math.trunc((" + leftValue + ") / (" + rightValue + "))", nil
	case *checker.IntModulo:
		return e.emitBinary(expr.Left, "%", expr.Right)
	case *checker.IntGreater:
		return e.emitIntComparison(expr.Left, ">", expr.Right)
	case *checker.IntGreaterEqual:
		return e.emitIntComparison(expr.Left, ">=", expr.Right)
	case *checker.IntLess:
		return e.emitIntComparison(expr.Left, "<", expr.Right)
	case *checker.IntLessEqual:
		return e.emitIntComparison(expr.Left, "<=", expr.Right)
	case *checker.FloatAddition:
		return e.emitBinary(expr.Left, "+", expr.Right)
	case *checker.FloatSubtraction:
		return e.emitBinary(expr.Left, "-", expr.Right)
	case *checker.FloatMultiplication:
		return e.emitBinary(expr.Left, "*", expr.Right)
	case *checker.FloatDivision:
		return e.emitBinary(expr.Left, "/", expr.Right)
	case *checker.FloatGreater:
		return e.emitBinary(expr.Left, ">", expr.Right)
	case *checker.FloatGreaterEqual:
		return e.emitBinary(expr.Left, ">=", expr.Right)
	case *checker.FloatLess:
		return e.emitBinary(expr.Left, "<", expr.Right)
	case *checker.FloatLessEqual:
		return e.emitBinary(expr.Left, "<=", expr.Right)
	case *checker.StrAddition:
		return e.emitBinary(expr.Left, "+", expr.Right)
	case *checker.Equality:
		return e.emitEquality(expr.Left, expr.Right)
	case *checker.And:
		return e.emitBinary(expr.Left, "&&", expr.Right)
	case *checker.Or:
		return e.emitBinary(expr.Left, "||", expr.Right)
	case *checker.Negation:
		value, err := e.emitExpr(expr.Value)
		if err != nil {
			return "", err
		}
		return "(-" + value + ")", nil
	case *checker.Not:
		value, err := e.emitExpr(expr.Value)
		if err != nil {
			return "", err
		}
		return "(!" + value + ")", nil
	case *checker.If:
		return e.emitInlineClosure(expr)
	case *checker.Block:
		return e.emitInlineClosure(expr)
	case *checker.Panic:
		message, err := e.emitExpr(expr.Message)
		if err != nil {
			return "", err
		}
		return "(() => { throw makeArdError(\"panic\", " + strconv.Quote(e.currentModule) + ", " + strconv.Quote(e.currentFunction) + ", 0, " + message + "); })()", nil
	default:
		return "", fmt.Errorf("js backend does not yet support expression type %T", expr)
	}
}

func (e *emitter) emitEnumDef(def *checker.Enum) error {
	entries := make([]string, 0, len(def.Values))
	for _, value := range def.Values {
		entries = append(entries, jsName(value.Name)+": makeEnum("+strconv.Quote(def.Name)+", "+strconv.Quote(value.Name)+", "+strconv.Itoa(value.Value)+")")
	}
	e.line("const " + jsName(def.Name) + " = Object.freeze({ " + strings.Join(entries, ", ") + " });")
	methodNames := sortedFunctionNames(def.Methods)
	for _, methodName := range methodNames {
		if !e.enumMethodUsed(def.Name, methodName) {
			continue
		}
		e.line("")
		if err := e.emitEnumMethod(def, def.Methods[methodName]); err != nil {
			return err
		}
	}
	e.line("")
	return nil
}

func (e *emitter) emitEnumMethod(def *checker.Enum, method *checker.FunctionDef) error {
	params := make([]string, 0, len(method.Parameters)+1)
	params = append(params, "__enum_self")
	for _, param := range method.Parameters {
		params = append(params, jsName(param.Name))
	}

	e.line(fmt.Sprintf("function %s(%s) {", enumMethodName(def.Name, method.Name), strings.Join(params, ", ")))
	prevFunction := e.currentFunction
	prevReceiver := e.currentReceiver
	prevReceiverExpr := e.currentReceiverExpr
	prevReturnType := e.currentReturnType
	e.currentFunction = def.Name + "." + method.Name
	e.currentReceiver = method.Receiver
	e.currentReceiverExpr = "__enum_self"
	e.currentReturnType = method.ReturnType
	e.indent(func() {
		err := e.emitFunctionBoundary(method.Body)
		if err != nil {
			panic(err)
		}
	})
	e.currentFunction = prevFunction
	e.currentReceiver = prevReceiver
	e.currentReceiverExpr = prevReceiverExpr
	e.currentReturnType = prevReturnType
	e.line("}")
	return nil
}

func (e *emitter) emitStructDef(def *checker.StructDef) error {
	fieldNames := sortedFieldNames(def.Fields)
	params := make([]string, 0, len(fieldNames))
	for _, field := range fieldNames {
		params = append(params, jsName(field))
	}

	e.line(fmt.Sprintf("class %s {", jsName(def.Name)))
	e.indent(func() {
		e.line("constructor(" + strings.Join(params, ", ") + ") {")
		e.indent(func() {
			for _, field := range fieldNames {
				name := jsName(field)
				e.line("this." + name + " = " + name + ";")
			}
		})
		e.line("}")
		methodNames := sortedFunctionNames(def.Methods)
		for _, methodName := range methodNames {
			e.line("")
			err := e.emitStructMethod(def, def.Methods[methodName])
			if err != nil {
				panic(err)
			}
		}
	})
	e.line("}")
	e.line("")
	return nil
}

func (e *emitter) emitStructMethod(def *checker.StructDef, method *checker.FunctionDef) error {
	params := make([]string, 0, len(method.Parameters))
	for _, param := range method.Parameters {
		params = append(params, jsName(param.Name))
	}

	e.line(fmt.Sprintf("%s(%s) {", jsName(method.Name), strings.Join(params, ", ")))
	prevFunction := e.currentFunction
	prevReceiver := e.currentReceiver
	prevReceiverExpr := e.currentReceiverExpr
	prevReturnType := e.currentReturnType
	e.currentFunction = def.Name + "." + method.Name
	e.currentReceiver = method.Receiver
	e.currentReceiverExpr = "this"
	e.currentReturnType = method.ReturnType
	e.indent(func() {
		err := e.emitFunctionBoundary(method.Body)
		if err != nil {
			panic(err)
		}
	})
	e.currentFunction = prevFunction
	e.currentReceiver = prevReceiver
	e.currentReceiverExpr = prevReceiverExpr
	e.currentReturnType = prevReturnType
	e.line("}")
	return nil
}

func (e *emitter) emitStructInstance(instance *checker.StructInstance, ctor string) (string, error) {
	fieldNames := sortedStructInstanceFields(instance)
	args := make([]string, 0, len(fieldNames))
	for _, field := range fieldNames {
		expr, ok := instance.Fields[field]
		if !ok || expr == nil {
			fieldType := instance.FieldTypes[field]
			if _, isMaybe := fieldType.(*checker.Maybe); isMaybe {
				args = append(args, "Maybe.none()")
				continue
			}
			return "", fmt.Errorf("missing struct field value for %s", field)
		}
		value, err := e.emitExpr(expr)
		if err != nil {
			return "", err
		}
		args = append(args, value)
	}
	return "new " + ctor + "(" + strings.Join(args, ", ") + ")", nil
}

func (e *emitter) emitFunctionLiteral(def *checker.FunctionDef) (string, error) {
	child := e.childEmitter()
	child.currentReturnType = def.ReturnType
	params := make([]string, 0, len(def.Parameters))
	for _, param := range def.Parameters {
		params = append(params, jsName(param.Name))
	}
	child.line("function(" + strings.Join(params, ", ") + ") {")
	child.indent(func() {
		if err := child.emitFunctionBoundary(def.Body); err != nil {
			panic(err)
		}
	})
	child.line("}")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitListMethod(method *checker.ListMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	args, err := e.emitArgs(method.Args)
	if err != nil {
		return "", err
	}

	switch method.Kind {
	case checker.ListSize:
		return subject + ".length", nil
	case checker.ListAt:
		if len(args) != 1 {
			return "", fmt.Errorf("list.at expects one arg")
		}
		return subject + "[" + args[0] + "]", nil
	case checker.ListPush:
		if len(args) != 1 {
			return "", fmt.Errorf("list.push expects one arg")
		}
		return e.emitMutationExpr(subject, []string{"__value.push(" + args[0] + ");"}, "__value")
	case checker.ListPrepend:
		if len(args) != 1 {
			return "", fmt.Errorf("list.prepend expects one arg")
		}
		return e.emitMutationExpr(subject, []string{"__value.unshift(" + args[0] + ");"}, "__value")
	case checker.ListSet:
		if len(args) != 2 {
			return "", fmt.Errorf("list.set expects two args")
		}
		return e.emitMutationExpr(subject, []string{"__value[" + args[0] + "] = " + args[1] + ";"}, "true")
	case checker.ListSwap:
		if len(args) != 2 {
			return "", fmt.Errorf("list.swap expects two args")
		}
		lines := []string{
			"const __tmp = __value[" + args[0] + "];",
			"__value[" + args[0] + "] = __value[" + args[1] + "];",
			"__value[" + args[1] + "] = __tmp;",
		}
		return e.emitMutationExpr(subject, lines, "undefined")
	case checker.ListSort:
		if len(args) != 1 {
			return "", fmt.Errorf("list.sort expects one arg")
		}
		cmp := args[0]
		lines := []string{
			"__value.sort((a, b) => " + cmp + "(a, b) ? -1 : (" + cmp + "(b, a) ? 1 : 0));",
		}
		return e.emitMutationExpr(subject, lines, "undefined")
	default:
		return "", fmt.Errorf("unsupported list method: %v", method.Kind)
	}
}

func (e *emitter) emitMapMethod(method *checker.MapMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	args, err := e.emitArgs(method.Args)
	if err != nil {
		return "", err
	}

	switch method.Kind {
	case checker.MapKeys:
		return "Array.from(" + subject + ".keys())", nil
	case checker.MapSize:
		return subject + ".size", nil
	case checker.MapGet:
		if len(args) != 1 {
			return "", fmt.Errorf("map.get expects one arg")
		}
		return "(" + subject + ".has(" + args[0] + ") ? Maybe.some(" + subject + ".get(" + args[0] + ")) : Maybe.none())", nil
	case checker.MapSet:
		if len(args) != 2 {
			return "", fmt.Errorf("map.set expects two args")
		}
		return e.emitMutationExpr(subject, []string{"__value.set(" + args[0] + ", " + args[1] + ");"}, "true")
	case checker.MapDrop:
		if len(args) != 1 {
			return "", fmt.Errorf("map.drop expects one arg")
		}
		return e.emitMutationExpr(subject, []string{"__value.delete(" + args[0] + ");"}, "undefined")
	case checker.MapHas:
		if len(args) != 1 {
			return "", fmt.Errorf("map.has expects one arg")
		}
		return subject + ".has(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported map method: %v", method.Kind)
	}
}

func (e *emitter) emitStrMethod(method *checker.StrMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	args, err := e.emitArgs(method.Args)
	if err != nil {
		return "", err
	}
	switch method.Kind {
	case checker.StrSize:
		return subject + ".length", nil
	case checker.StrIsEmpty:
		return "(" + subject + ".length === 0)", nil
	case checker.StrContains:
		if len(args) != 1 {
			return "", fmt.Errorf("str.contains expects one arg")
		}
		return subject + ".includes(" + args[0] + ")", nil
	case checker.StrReplace:
		if len(args) != 2 {
			return "", fmt.Errorf("str.replace expects two args")
		}
		return subject + ".replace(" + args[0] + ", " + args[1] + ")", nil
	case checker.StrReplaceAll:
		if len(args) != 2 {
			return "", fmt.Errorf("str.replace_all expects two args")
		}
		return subject + ".replaceAll(" + args[0] + ", " + args[1] + ")", nil
	case checker.StrSplit:
		if len(args) != 1 {
			return "", fmt.Errorf("str.split expects one arg")
		}
		return subject + ".split(" + args[0] + ")", nil
	case checker.StrStartsWith:
		if len(args) != 1 {
			return "", fmt.Errorf("str.starts_with expects one arg")
		}
		return subject + ".startsWith(" + args[0] + ")", nil
	case checker.StrToStr:
		return subject, nil
	case checker.StrTrim:
		return subject + ".trim()", nil
	default:
		return "", fmt.Errorf("unsupported str method: %v", method.Kind)
	}
}

func (e *emitter) emitIntMethod(method *checker.IntMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	switch method.Kind {
	case checker.IntToStr:
		return "String(" + subject + ")", nil
	default:
		return "", fmt.Errorf("unsupported int method: %v", method.Kind)
	}
}

func (e *emitter) emitFloatMethod(method *checker.FloatMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	switch method.Kind {
	case checker.FloatToStr:
		return "(" + subject + ").toFixed(2)", nil
	case checker.FloatToInt:
		return "Math.trunc(" + subject + ")", nil
	default:
		return "", fmt.Errorf("unsupported float method: %v", method.Kind)
	}
}

func (e *emitter) emitBoolMethod(method *checker.BoolMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	switch method.Kind {
	case checker.BoolToStr:
		return "String(" + subject + ")", nil
	default:
		return "", fmt.Errorf("unsupported bool method: %v", method.Kind)
	}
}

func (e *emitter) emitBoundBlockExpr(block *checker.Block, bindings []string) (string, error) {
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		for _, binding := range bindings {
			child.line(binding)
		}
		err := child.emitBlock(block, true)
		if err != nil {
			panic(err)
		}
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitBoolMatch(match *checker.BoolMatch) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	trueExpr, err := e.emitBoundBlockExpr(match.True, nil)
	if err != nil {
		return "", err
	}
	falseExpr, err := e.emitBoundBlockExpr(match.False, nil)
	if err != nil {
		return "", err
	}
	return "(() => { const __match = " + subject + "; return __match ? " + trueExpr + " : " + falseExpr + "; })()", nil
}

func (e *emitter) emitEnumMatch(match *checker.EnumMatch) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		child.line("const __match = " + subject + ";")
		enumName, err := enumTypeName(match.Subject.Type())
		if err != nil {
			panic(err)
		}
		for _, discriminant := range sortedEnumDiscriminants(match.DiscriminantToIndex) {
			idx := match.DiscriminantToIndex[discriminant]
			if idx < 0 || int(idx) >= len(match.Cases) || match.Cases[idx] == nil {
				continue
			}
			blockExpr, err := e.emitBoundBlockExpr(match.Cases[idx], nil)
			if err != nil {
				panic(err)
			}
			child.line("if (isEnumOf(__match, " + strconv.Quote(enumName) + ") && __match.value === " + strconv.Itoa(discriminant) + ") return " + blockExpr + ";")
		}
		if match.CatchAll != nil {
			catchAllExpr, err := e.emitBoundBlockExpr(match.CatchAll, nil)
			if err != nil {
				panic(err)
			}
			child.line("return " + catchAllExpr + ";")
		} else {
			child.line(`throw makeArdError("panic", "match", "enum", 0, "non-exhaustive enum match");`)
		}
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitUnionMatch(match *checker.UnionMatch) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		child.line("const __match = " + subject + ";")
		for _, caseName := range sortedUnionCaseNames(match.TypeCases) {
			matchCase := match.TypeCases[caseName]
			if matchCase == nil {
				continue
			}
			caseType := unionCaseType(match.TypeCasesByType, caseName)
			if caseType == nil {
				panic(fmt.Errorf("missing union case type for %s", caseName))
			}
			predicate, err := e.emitUnionTypePredicate(caseType, "__match")
			if err != nil {
				panic(err)
			}
			bindings := []string{}
			if matchCase.Pattern != nil {
				bindings = append(bindings, "const "+jsName(matchCase.Pattern.Name)+" = __match;")
			}
			blockExpr, err := e.emitBoundBlockExpr(matchCase.Body, bindings)
			if err != nil {
				panic(err)
			}
			child.line("if (" + predicate + ") return " + blockExpr + ";")
		}
		if match.CatchAll != nil {
			catchAllExpr, err := e.emitBoundBlockExpr(match.CatchAll, nil)
			if err != nil {
				panic(err)
			}
			child.line("return " + catchAllExpr + ";")
		} else {
			child.line(`throw makeArdError("panic", "match", "union", 0, "non-exhaustive union match");`)
		}
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitIntMatch(match *checker.IntMatch) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		child.line("const __match = " + subject + ";")
		for _, value := range sortedIntCaseKeys(match.IntCases) {
			block := match.IntCases[value]
			blockExpr, err := e.emitBoundBlockExpr(block, nil)
			if err != nil {
				panic(err)
			}
			child.line(fmt.Sprintf("if (__match === %d) return %s;", value, blockExpr))
		}
		for _, intRange := range sortedIntRangeKeys(match.RangeCases) {
			block := match.RangeCases[intRange]
			blockExpr, err := e.emitBoundBlockExpr(block, nil)
			if err != nil {
				panic(err)
			}
			child.line(fmt.Sprintf("if (__match >= %d && __match <= %d) return %s;", intRange.Start, intRange.End, blockExpr))
		}
		if match.CatchAll != nil {
			catchAllExpr, err := e.emitBoundBlockExpr(match.CatchAll, nil)
			if err != nil {
				panic(err)
			}
			child.line("return " + catchAllExpr + ";")
		} else {
			child.line(`throw makeArdError("panic", "match", "int", 0, "non-exhaustive int match");`)
		}
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitConditionalMatch(match *checker.ConditionalMatch) (string, error) {
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		for _, matchCase := range match.Cases {
			condition, err := e.emitExpr(matchCase.Condition)
			if err != nil {
				panic(err)
			}
			blockExpr, err := e.emitBoundBlockExpr(matchCase.Body, nil)
			if err != nil {
				panic(err)
			}
			child.line("if (" + condition + ") return " + blockExpr + ";")
		}
		if match.CatchAll != nil {
			catchAllExpr, err := e.emitBoundBlockExpr(match.CatchAll, nil)
			if err != nil {
				panic(err)
			}
			child.line("return " + catchAllExpr + ";")
		} else {
			child.line(`throw makeArdError("panic", "match", "conditional", 0, "non-exhaustive conditional match");`)
		}
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitOptionMatch(match *checker.OptionMatch) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	someBindings := []string{}
	if match.Some != nil && match.Some.Pattern != nil {
		someBindings = append(someBindings, "const "+jsName(match.Some.Pattern.Name)+" = __match.value;")
	}
	someExpr, err := e.emitBoundBlockExpr(match.Some.Body, someBindings)
	if err != nil {
		return "", err
	}
	noneExpr, err := e.emitBoundBlockExpr(match.None, nil)
	if err != nil {
		return "", err
	}
	return "(() => { const __match = " + subject + "; return __match.isSome() ? " + someExpr + " : " + noneExpr + "; })()", nil
}

func (e *emitter) emitResultMatch(match *checker.ResultMatch) (string, error) {
	subject, err := e.emitExpr(match.Subject)
	if err != nil {
		return "", err
	}
	okBindings := []string{}
	if match.Ok != nil && match.Ok.Pattern != nil {
		okBindings = append(okBindings, "const "+jsName(match.Ok.Pattern.Name)+" = __match.ok;")
	}
	okExpr, err := e.emitBoundBlockExpr(match.Ok.Body, okBindings)
	if err != nil {
		return "", err
	}
	errBindings := []string{}
	if match.Err != nil && match.Err.Pattern != nil {
		errBindings = append(errBindings, "const "+jsName(match.Err.Pattern.Name)+" = __match.error;")
	}
	errExpr, err := e.emitBoundBlockExpr(match.Err.Body, errBindings)
	if err != nil {
		return "", err
	}
	return "(() => { const __match = " + subject + "; return __match.isOk() ? " + okExpr + " : " + errExpr + "; })()", nil
}

func (e *emitter) emitMaybeModuleCall(call *checker.ModuleFunctionCall) (string, error) {
	switch call.Call.Name {
	case "some":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("maybe::some expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "Maybe.some(" + args[0] + ")", nil
	case "none":
		return "Maybe.none()", nil
	default:
		return "", fmt.Errorf("unsupported maybe module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitResultModuleCall(call *checker.ModuleFunctionCall) (string, error) {
	switch call.Call.Name {
	case "ok":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Result::ok expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "Result.ok(" + args[0] + ")", nil
	case "err":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Result::err expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "Result.err(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported Result module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitFloatModuleCall(call *checker.ModuleFunctionCall) (string, error) {
	switch call.Call.Name {
	case "from_int":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Float::from_int expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "Number(" + args[0] + ")", nil
	case "from_str":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Float::from_str expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "(() => { const __input = String(" + args[0] + ").trim(); if (__input === \"\") return Maybe.none(); const __value = Number(__input); return Number.isNaN(__value) ? Maybe.none() : Maybe.some(__value); })()", nil
	case "floor":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Float::floor expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "Math.floor(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported Float module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitIntModuleCall(call *checker.ModuleFunctionCall) (string, error) {
	switch call.Call.Name {
	case "from_str":
		if len(call.Call.Args) != 1 {
			return "", fmt.Errorf("Int::from_str expects one arg")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "(() => { const __input = String(" + args[0] + ").trim(); if (!/^[-+]?\\d+$/.test(__input)) return Maybe.none(); return Maybe.some(Number.parseInt(__input, 10)); })()", nil
	default:
		return "", fmt.Errorf("unsupported Int module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitListModuleCall(call *checker.ModuleFunctionCall) (string, error) {
	switch call.Call.Name {
	case "new":
		return "[]", nil
	case "concat":
		if len(call.Call.Args) != 2 {
			return "", fmt.Errorf("List::concat expects two args")
		}
		args, err := e.emitArgs(call.Call.Args)
		if err != nil {
			return "", err
		}
		return "(" + args[0] + ").concat(" + args[1] + ")", nil
	default:
		return "", fmt.Errorf("unsupported List module call: %s", call.Call.Name)
	}
}

func (e *emitter) emitMaybeMethod(method *checker.MaybeMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	args, err := e.emitArgs(method.Args)
	if err != nil {
		return "", err
	}
	failArgs := func(name string, want int) error {
		return fmt.Errorf("maybe.%s expects %d arg(s)", name, want)
	}

	switch method.Kind {
	case checker.MaybeExpect:
		if len(args) != 1 {
			return "", failArgs("expect", 1)
		}
		return subject + ".expect(" + args[0] + ")", nil
	case checker.MaybeIsNone:
		return subject + ".isNone()", nil
	case checker.MaybeIsSome:
		return subject + ".isSome()", nil
	case checker.MaybeOr:
		if len(args) != 1 {
			return "", failArgs("or", 1)
		}
		return subject + ".or(" + args[0] + ")", nil
	case checker.MaybeMap:
		if len(args) != 1 {
			return "", failArgs("map", 1)
		}
		return subject + ".map(" + args[0] + ")", nil
	case checker.MaybeAndThen:
		if len(args) != 1 {
			return "", failArgs("and_then", 1)
		}
		return subject + ".andThen(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported maybe method: %v", method.Kind)
	}
}

func (e *emitter) emitResultMethod(method *checker.ResultMethod) (string, error) {
	subject, err := e.emitExpr(method.Subject)
	if err != nil {
		return "", err
	}
	args, err := e.emitArgs(method.Args)
	if err != nil {
		return "", err
	}
	failArgs := func(name string, want int) error {
		return fmt.Errorf("result.%s expects %d arg(s)", name, want)
	}

	switch method.Kind {
	case checker.ResultExpect:
		if len(args) != 1 {
			return "", failArgs("expect", 1)
		}
		return subject + ".expect(" + args[0] + ")", nil
	case checker.ResultOr:
		if len(args) != 1 {
			return "", failArgs("or", 1)
		}
		return subject + ".or(" + args[0] + ")", nil
	case checker.ResultIsOk:
		return subject + ".isOk()", nil
	case checker.ResultIsErr:
		return subject + ".isErr()", nil
	case checker.ResultMap:
		if len(args) != 1 {
			return "", failArgs("map", 1)
		}
		return subject + ".map(" + args[0] + ")", nil
	case checker.ResultMapErr:
		if len(args) != 1 {
			return "", failArgs("map_err", 1)
		}
		return subject + ".mapErr(" + args[0] + ")", nil
	case checker.ResultAndThen:
		if len(args) != 1 {
			return "", failArgs("and_then", 1)
		}
		return subject + ".andThen(" + args[0] + ")", nil
	default:
		return "", fmt.Errorf("unsupported result method: %v", method.Kind)
	}
}

func (e *emitter) emitMutationExpr(subject string, lines []string, returnExpr string) (string, error) {
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		child.line("const __value = " + subject + ";")
		for _, line := range lines {
			child.line(line)
		}
		child.line("return " + returnExpr + ";")
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitTemplateStr(expr *checker.TemplateStr) (string, error) {
	var out bytes.Buffer
	out.WriteByte('`')
	for _, chunk := range expr.Chunks {
		if literal, ok := chunk.(*checker.StrLiteral); ok {
			out.WriteString(escapeTemplateLiteral(literal.Value))
			continue
		}
		value, err := e.emitExpr(chunk)
		if err != nil {
			return "", err
		}
		out.WriteString("${")
		out.WriteString(value)
		out.WriteByte('}')
	}
	out.WriteByte('`')
	return out.String(), nil
}

func (e *emitter) emitInlineClosure(expr checker.Expression) (string, error) {
	child := e.childEmitter()
	child.line("(() => {")
	child.indent(func() {
		var err error
		switch expr := expr.(type) {
		case *checker.If:
			err = child.emitIf(expr, true)
		case *checker.Block:
			err = child.emitBlock(expr, true)
		default:
			err = fmt.Errorf("unsupported inline closure expression %T", expr)
		}
		if err != nil {
			panic(err)
		}
	})
	child.line("})()")
	return strings.TrimSpace(child.builder.String()), nil
}

func (e *emitter) emitBinary(left checker.Expression, op string, right checker.Expression) (string, error) {
	leftValue, err := e.emitExpr(left)
	if err != nil {
		return "", err
	}
	rightValue, err := e.emitExpr(right)
	if err != nil {
		return "", err
	}
	return "(" + leftValue + " " + op + " " + rightValue + ")", nil
}

func (e *emitter) emitEquality(left checker.Expression, right checker.Expression) (string, error) {
	leftValue, err := e.emitExpr(left)
	if err != nil {
		return "", err
	}
	rightValue, err := e.emitExpr(right)
	if err != nil {
		return "", err
	}
	if requiresSpecialEquality(left.Type(), right.Type()) {
		return "ardEq(" + leftValue + ", " + rightValue + ")", nil
	}
	return "(" + leftValue + " === " + rightValue + ")", nil
}

func (e *emitter) emitIntComparison(left checker.Expression, op string, right checker.Expression) (string, error) {
	leftValue, err := e.emitExpr(left)
	if err != nil {
		return "", err
	}
	rightValue, err := e.emitExpr(right)
	if err != nil {
		return "", err
	}
	if requiresEnumAwareComparison(left.Type(), right.Type()) {
		return "(ardEnumValue(" + leftValue + ") " + op + " ardEnumValue(" + rightValue + "))", nil
	}
	return "(" + leftValue + " " + op + " " + rightValue + ")", nil
}

func (e *emitter) emitArgs(args []checker.Expression) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		value, err := e.emitExpr(arg)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (e *emitter) line(value string) {
	if value == "" {
		e.builder.WriteByte('\n')
		return
	}
	for i := 0; i < e.indentLevel; i++ {
		e.builder.WriteString("  ")
	}
	e.builder.WriteString(value)
	e.builder.WriteByte('\n')
}

func (e *emitter) indent(fn func()) {
	e.indentLevel++
	defer func() { e.indentLevel-- }()
	fn()
}

func containsTry(expr checker.Expression) bool {
	var exprContains func(checker.Expression) bool
	var blockContains func(*checker.Block) bool
	var stmtContains func(checker.Statement) bool

	exprContains = func(expr checker.Expression) bool {
		if expr == nil {
			return false
		}
		value := reflect.ValueOf(expr)
		if value.Kind() == reflect.Ptr && value.IsNil() {
			return false
		}
		switch expr := expr.(type) {
		case *checker.TryOp:
			return true
		case *checker.FunctionDef:
			return false
		case *checker.StructInstance:
			for _, field := range expr.Fields {
				if exprContains(field) {
					return true
				}
			}
		case *checker.ModuleStructInstance:
			return exprContains(expr.Property)
		case *checker.InstanceProperty:
			return exprContains(expr.Subject)
		case *checker.ListLiteral:
			for _, element := range expr.Elements {
				if exprContains(element) {
					return true
				}
			}
		case *checker.MapLiteral:
			for i := range expr.Keys {
				if exprContains(expr.Keys[i]) || exprContains(expr.Values[i]) {
					return true
				}
			}
		case *checker.TemplateStr:
			for _, chunk := range expr.Chunks {
				if exprContains(chunk) {
					return true
				}
			}
		case *checker.FunctionCall:
			for _, arg := range expr.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.ModuleFunctionCall:
			for _, arg := range expr.Call.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.InstanceMethod:
			if exprContains(expr.Subject) {
				return true
			}
			for _, arg := range expr.Method.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.CopyExpression:
			return exprContains(expr.Expr)
		case *checker.StrMethod:
			if exprContains(expr.Subject) {
				return true
			}
			for _, arg := range expr.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.IntMethod:
			return exprContains(expr.Subject)
		case *checker.FloatMethod:
			return exprContains(expr.Subject)
		case *checker.BoolMethod:
			return exprContains(expr.Subject)
		case *checker.ListMethod:
			if exprContains(expr.Subject) {
				return true
			}
			for _, arg := range expr.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.MapMethod:
			if exprContains(expr.Subject) {
				return true
			}
			for _, arg := range expr.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.MaybeMethod:
			if exprContains(expr.Subject) {
				return true
			}
			for _, arg := range expr.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.ResultMethod:
			if exprContains(expr.Subject) {
				return true
			}
			for _, arg := range expr.Args {
				if exprContains(arg) {
					return true
				}
			}
		case *checker.If:
			return exprContains(expr.Condition) || blockContains(expr.Body) || blockContains(expr.Else) || exprContains(expr.ElseIf)
		case *checker.Block:
			return blockContains(expr)
		case *checker.BoolMatch:
			return exprContains(expr.Subject) || blockContains(expr.True) || blockContains(expr.False)
		case *checker.EnumMatch:
			if exprContains(expr.Subject) {
				return true
			}
			for _, block := range expr.Cases {
				if blockContains(block) {
					return true
				}
			}
			return blockContains(expr.CatchAll)
		case *checker.UnionMatch:
			if exprContains(expr.Subject) {
				return true
			}
			for _, matchCase := range expr.TypeCases {
				if matchCase != nil && blockContains(matchCase.Body) {
					return true
				}
			}
			return blockContains(expr.CatchAll)
		case *checker.IntMatch:
			if exprContains(expr.Subject) {
				return true
			}
			for _, block := range expr.IntCases {
				if blockContains(block) {
					return true
				}
			}
			for _, block := range expr.RangeCases {
				if blockContains(block) {
					return true
				}
			}
			return blockContains(expr.CatchAll)
		case *checker.ConditionalMatch:
			for _, matchCase := range expr.Cases {
				if exprContains(matchCase.Condition) || blockContains(matchCase.Body) {
					return true
				}
			}
			return blockContains(expr.CatchAll)
		case *checker.OptionMatch:
			return exprContains(expr.Subject) || (expr.Some != nil && blockContains(expr.Some.Body)) || blockContains(expr.None)
		case *checker.ResultMatch:
			return exprContains(expr.Subject) || (expr.Ok != nil && blockContains(expr.Ok.Body)) || (expr.Err != nil && blockContains(expr.Err.Body))
		case *checker.IntAddition:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntSubtraction:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntMultiplication:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntDivision:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntModulo:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntGreater:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntGreaterEqual:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntLess:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.IntLessEqual:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatAddition:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatSubtraction:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatMultiplication:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatDivision:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatGreater:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatGreaterEqual:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatLess:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.FloatLessEqual:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.StrAddition:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.Equality:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.And:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.Or:
			return exprContains(expr.Left) || exprContains(expr.Right)
		case *checker.Negation:
			return exprContains(expr.Value)
		case *checker.Not:
			return exprContains(expr.Value)
		}
		return false
	}
	blockContains = func(block *checker.Block) bool {
		if block == nil {
			return false
		}
		for _, stmt := range block.Stmts {
			if stmtContains(stmt) {
				return true
			}
		}
		return false
	}
	stmtContains = func(stmt checker.Statement) bool {
		if stmt.Stmt != nil {
			switch s := stmt.Stmt.(type) {
			case *checker.VariableDef:
				return exprContains(s.Value)
			case *checker.Reassignment:
				return exprContains(s.Target) || exprContains(s.Value)
			case checker.ForInList:
				return exprContains(s.List) || blockContains(s.Body)
			case checker.ForInMap:
				return exprContains(s.Map) || blockContains(s.Body)
			case checker.ForInStr:
				return exprContains(s.Value) || blockContains(s.Body)
			case checker.ForIntRange:
				return exprContains(s.Start) || exprContains(s.End) || blockContains(s.Body)
			case checker.ForLoop:
				return (s.Init != nil && exprContains(s.Init.Value)) || exprContains(s.Condition) || (s.Update != nil && exprContains(s.Update.Value)) || blockContains(s.Body)
			case checker.WhileLoop:
				return exprContains(s.Condition) || blockContains(s.Body)
			}
		}
		return exprContains(stmt.Expr)
	}
	return exprContains(expr)
}

func (e *emitter) emitVariableName(name string) string {
	if e.currentReceiver != "" && name == e.currentReceiver {
		if e.currentReceiverExpr != "" {
			return e.currentReceiverExpr
		}
		return "this"
	}
	return jsName(name)
}

func collectEnumDefs(program *checker.Program) []*checker.Enum {
	collected := map[string]*checker.Enum{}
	var visitType func(t checker.Type)
	visitType = func(t checker.Type) {
		switch typed := t.(type) {
		case *checker.Enum:
			collected[typed.Name] = typed
		case *checker.FunctionDef:
			for _, param := range typed.Parameters {
				visitType(param.Type)
			}
			visitType(typed.ReturnType)
		case *checker.ExternalFunctionDef:
			for _, param := range typed.Parameters {
				visitType(param.Type)
			}
			visitType(typed.ReturnType)
		case *checker.Maybe:
			visitType(typed.Of())
		case *checker.Result:
			visitType(typed.Val())
			visitType(typed.Err())
		case *checker.List:
			visitType(typed.Of())
		case *checker.Map:
			visitType(typed.Key())
			visitType(typed.Value())
		case *checker.Union:
			for _, inner := range typed.Types {
				visitType(inner)
			}
		case *checker.StructDef:
			for _, field := range typed.Fields {
				visitType(field)
			}
		}
	}
	var visitStmt func(stmt checker.Statement)
	var visitExpr func(expr checker.Expression)
	visitExpr = func(expr checker.Expression) {
		switch expr := expr.(type) {
		case *checker.EnumVariant:
			if enum, ok := expr.Type().(*checker.Enum); ok {
				collected[enum.Name] = enum
			}
		case *checker.FunctionDef:
			visitType(expr)
			for _, stmt := range expr.Body.Stmts {
				visitStmt(stmt)
			}
		case *checker.StructInstance:
			for _, field := range expr.Fields {
				visitExpr(field)
			}
		case *checker.ModuleStructInstance:
			visitExpr(expr.Property)
		case *checker.InstanceProperty:
			visitExpr(expr.Subject)
		case *checker.InstanceMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Method.Args {
				visitExpr(arg)
			}
		case *checker.FunctionCall:
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.ModuleFunctionCall:
			for _, arg := range expr.Call.Args {
				visitExpr(arg)
			}
		case *checker.ListLiteral:
			for _, element := range expr.Elements {
				visitExpr(element)
			}
		case *checker.MapLiteral:
			for i := range expr.Keys {
				visitExpr(expr.Keys[i])
				visitExpr(expr.Values[i])
			}
		case *checker.CopyExpression:
			visitExpr(expr.Expr)
		case *checker.TryOp:
			visitExpr(expr.Expr())
			if expr.CatchBlock != nil {
				for _, stmt := range expr.CatchBlock.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.TemplateStr:
			for _, chunk := range expr.Chunks {
				visitExpr(chunk)
			}
		case *checker.IntAddition:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntSubtraction:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntMultiplication:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntDivision:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntModulo:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntGreater:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntGreaterEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntLess:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntLessEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatAddition:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatSubtraction:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatMultiplication:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatDivision:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatGreater:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatGreaterEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatLess:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatLessEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.StrAddition:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.Equality:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.And:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.Or:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.Negation:
			visitExpr(expr.Value)
		case *checker.Not:
			visitExpr(expr.Value)
		case *checker.If:
			visitExpr(expr.Condition)
			for _, stmt := range expr.Body.Stmts {
				visitStmt(stmt)
			}
			if expr.ElseIf != nil {
				visitExpr(expr.ElseIf)
			}
			if expr.Else != nil {
				for _, stmt := range expr.Else.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.Block:
			for _, stmt := range expr.Stmts {
				visitStmt(stmt)
			}
		case *checker.ListMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.MapMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.MaybeMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.ResultMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.BoolMatch:
			visitExpr(expr.Subject)
			for _, stmt := range expr.True.Stmts {
				visitStmt(stmt)
			}
			for _, stmt := range expr.False.Stmts {
				visitStmt(stmt)
			}
		case *checker.IntMatch:
			visitExpr(expr.Subject)
			for _, block := range expr.IntCases {
				for _, stmt := range block.Stmts {
					visitStmt(stmt)
				}
			}
			for _, block := range expr.RangeCases {
				for _, stmt := range block.Stmts {
					visitStmt(stmt)
				}
			}
			if expr.CatchAll != nil {
				for _, stmt := range expr.CatchAll.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.ConditionalMatch:
			for _, c := range expr.Cases {
				visitExpr(c.Condition)
				for _, stmt := range c.Body.Stmts {
					visitStmt(stmt)
				}
			}
			if expr.CatchAll != nil {
				for _, stmt := range expr.CatchAll.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.OptionMatch:
			visitExpr(expr.Subject)
			for _, stmt := range expr.Some.Body.Stmts {
				visitStmt(stmt)
			}
			for _, stmt := range expr.None.Stmts {
				visitStmt(stmt)
			}
		case *checker.ResultMatch:
			visitExpr(expr.Subject)
			for _, stmt := range expr.Ok.Body.Stmts {
				visitStmt(stmt)
			}
			for _, stmt := range expr.Err.Body.Stmts {
				visitStmt(stmt)
			}
		}
		visitType(expr.Type())
	}
	visitStmt = func(stmt checker.Statement) {
		if stmt.Stmt != nil {
			switch s := stmt.Stmt.(type) {
			case *checker.VariableDef:
				visitExpr(s.Value)
				visitType(s.Type())
			case *checker.Reassignment:
				visitExpr(s.Target)
				visitExpr(s.Value)
			case *checker.StructDef:
				visitType(s)
			case *checker.Enum:
				collected[s.Name] = s
			case checker.Enum:
				sCopy := s
				collected[s.Name] = &sCopy
			case checker.ForLoop:
				if s.Init != nil {
					visitExpr(s.Init.Value)
				}
				visitExpr(s.Condition)
				if s.Update != nil {
					visitExpr(s.Update.Value)
				}
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			case checker.WhileLoop:
				visitExpr(s.Condition)
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			}
		}
		if stmt.Expr != nil {
			visitExpr(stmt.Expr)
		}
	}
	for _, stmt := range program.Statements {
		visitStmt(stmt)
	}
	out := make([]*checker.Enum, 0, len(collected))
	for _, enum := range collected {
		out = append(out, enum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func collectUsedEnumMethods(program *checker.Program) map[string]map[string]bool {
	used := map[string]map[string]bool{}
	var visitStmt func(stmt checker.Statement)
	var visitExpr func(expr checker.Expression)
	visitExpr = func(expr checker.Expression) {
		switch expr := expr.(type) {
		case *checker.InstanceMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Method.Args {
				visitExpr(arg)
			}
			if expr.ReceiverKind == checker.ReceiverEnum && expr.EnumType != nil {
				methods := used[expr.EnumType.Name]
				if methods == nil {
					methods = map[string]bool{}
					used[expr.EnumType.Name] = methods
				}
				methods[expr.Method.Name] = true
			}
		case *checker.FunctionDef:
			for _, stmt := range expr.Body.Stmts {
				visitStmt(stmt)
			}
		case *checker.StructInstance:
			for _, field := range expr.Fields {
				visitExpr(field)
			}
		case *checker.ModuleStructInstance:
			visitExpr(expr.Property)
		case *checker.InstanceProperty:
			visitExpr(expr.Subject)
		case *checker.FunctionCall:
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.ModuleFunctionCall:
			for _, arg := range expr.Call.Args {
				visitExpr(arg)
			}
		case *checker.ListLiteral:
			for _, element := range expr.Elements {
				visitExpr(element)
			}
		case *checker.MapLiteral:
			for i := range expr.Keys {
				visitExpr(expr.Keys[i])
				visitExpr(expr.Values[i])
			}
		case *checker.CopyExpression:
			visitExpr(expr.Expr)
		case *checker.TryOp:
			visitExpr(expr.Expr())
			if expr.CatchBlock != nil {
				for _, stmt := range expr.CatchBlock.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.TemplateStr:
			for _, chunk := range expr.Chunks {
				visitExpr(chunk)
			}
		case *checker.IntAddition:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntSubtraction:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntMultiplication:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntDivision:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntModulo:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntGreater:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntGreaterEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntLess:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.IntLessEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatAddition:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatSubtraction:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatMultiplication:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatDivision:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatGreater:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatGreaterEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatLess:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.FloatLessEqual:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.StrAddition:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.Equality:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.And:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.Or:
			visitExpr(expr.Left)
			visitExpr(expr.Right)
		case *checker.Negation:
			visitExpr(expr.Value)
		case *checker.Not:
			visitExpr(expr.Value)
		case *checker.If:
			visitExpr(expr.Condition)
			for _, stmt := range expr.Body.Stmts {
				visitStmt(stmt)
			}
			if expr.ElseIf != nil {
				visitExpr(expr.ElseIf)
			}
			if expr.Else != nil {
				for _, stmt := range expr.Else.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.Block:
			for _, stmt := range expr.Stmts {
				visitStmt(stmt)
			}
		case *checker.ListMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.MapMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.MaybeMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.ResultMethod:
			visitExpr(expr.Subject)
			for _, arg := range expr.Args {
				visitExpr(arg)
			}
		case *checker.BoolMatch:
			visitExpr(expr.Subject)
			for _, stmt := range expr.True.Stmts {
				visitStmt(stmt)
			}
			for _, stmt := range expr.False.Stmts {
				visitStmt(stmt)
			}
		case *checker.IntMatch:
			visitExpr(expr.Subject)
			for _, block := range expr.IntCases {
				for _, stmt := range block.Stmts {
					visitStmt(stmt)
				}
			}
			for _, block := range expr.RangeCases {
				for _, stmt := range block.Stmts {
					visitStmt(stmt)
				}
			}
			if expr.CatchAll != nil {
				for _, stmt := range expr.CatchAll.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.ConditionalMatch:
			for _, c := range expr.Cases {
				visitExpr(c.Condition)
				for _, stmt := range c.Body.Stmts {
					visitStmt(stmt)
				}
			}
			if expr.CatchAll != nil {
				for _, stmt := range expr.CatchAll.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.OptionMatch:
			visitExpr(expr.Subject)
			for _, stmt := range expr.Some.Body.Stmts {
				visitStmt(stmt)
			}
			for _, stmt := range expr.None.Stmts {
				visitStmt(stmt)
			}
		case *checker.ResultMatch:
			visitExpr(expr.Subject)
			for _, stmt := range expr.Ok.Body.Stmts {
				visitStmt(stmt)
			}
			for _, stmt := range expr.Err.Body.Stmts {
				visitStmt(stmt)
			}
		case *checker.EnumMatch:
			visitExpr(expr.Subject)
			for _, block := range expr.Cases {
				for _, stmt := range block.Stmts {
					visitStmt(stmt)
				}
			}
			if expr.CatchAll != nil {
				for _, stmt := range expr.CatchAll.Stmts {
					visitStmt(stmt)
				}
			}
		case *checker.UnionMatch:
			visitExpr(expr.Subject)
			for _, block := range expr.TypeCases {
				for _, stmt := range block.Body.Stmts {
					visitStmt(stmt)
				}
			}
			if expr.CatchAll != nil {
				for _, stmt := range expr.CatchAll.Stmts {
					visitStmt(stmt)
				}
			}
		}
	}
	visitStmt = func(stmt checker.Statement) {
		if stmt.Stmt != nil {
			switch s := stmt.Stmt.(type) {
			case *checker.VariableDef:
				visitExpr(s.Value)
			case *checker.Reassignment:
				visitExpr(s.Target)
				visitExpr(s.Value)
			case checker.ForInList:
				visitExpr(s.List)
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			case checker.ForInMap:
				visitExpr(s.Map)
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			case checker.ForInStr:
				visitExpr(s.Value)
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			case checker.ForIntRange:
				visitExpr(s.Start)
				visitExpr(s.End)
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			case checker.ForLoop:
				if s.Init != nil {
					visitExpr(s.Init.Value)
				}
				visitExpr(s.Condition)
				if s.Update != nil {
					visitExpr(s.Update.Value)
				}
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			case checker.WhileLoop:
				visitExpr(s.Condition)
				for _, bodyStmt := range s.Body.Stmts {
					visitStmt(bodyStmt)
				}
			}
		}
		if stmt.Expr != nil {
			visitExpr(stmt.Expr)
		}
	}
	for _, stmt := range program.Statements {
		visitStmt(stmt)
	}
	return used
}

func (e *emitter) enumMethodUsed(enumName, methodName string) bool {
	methods := e.usedEnumMethods[enumName]
	return methods != nil && methods[methodName]
}

func isEnumType(t checker.Type) bool {
	switch t.(type) {
	case *checker.Enum, checker.Enum:
		return true
	default:
		return false
	}
}

func enumTypeName(t checker.Type) (string, error) {
	switch typed := t.(type) {
	case *checker.Enum:
		return typed.Name, nil
	case checker.Enum:
		return typed.Name, nil
	default:
		return "", fmt.Errorf("expected enum type, got %s", t.String())
	}
}

func isMaybeType(t checker.Type) bool {
	_, ok := t.(*checker.Maybe)
	return ok
}

func requiresSpecialEquality(left checker.Type, right checker.Type) bool {
	return isEnumType(left) || isMaybeType(left) || isMaybeType(right)
}

func requiresEnumAwareComparison(left checker.Type, right checker.Type) bool {
	return isEnumType(left) || isEnumType(right)
}

func sortedUnionCaseNames(cases map[string]*checker.Match) []string {
	names := make([]string, 0, len(cases))
	for name := range cases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func unionCaseType(typeCasesByType map[checker.Type]*checker.Match, caseName string) checker.Type {
	for t := range typeCasesByType {
		if t.String() == caseName {
			return t
		}
	}
	return nil
}

func (e *emitter) emitUnionTypePredicate(t checker.Type, subject string) (string, error) {
	switch typed := t.(type) {
	case *checker.StructDef:
		return subject + " instanceof " + jsName(typed.Name), nil
	case *checker.Enum:
		return "isEnumOf(" + subject + ", " + strconv.Quote(typed.Name) + ")", nil
	case *checker.Maybe:
		return subject + " instanceof Maybe", nil
	case *checker.Result:
		return subject + " instanceof Result", nil
	case *checker.List:
		return "Array.isArray(" + subject + ")", nil
	case *checker.Map:
		return subject + " instanceof Map", nil
	}

	switch t {
	case checker.Str:
		return "typeof " + subject + " === \"string\"", nil
	case checker.Int, checker.Float:
		return "typeof " + subject + " === \"number\"", nil
	case checker.Bool:
		return "typeof " + subject + " === \"boolean\"", nil
	case checker.Dynamic:
		return "true", nil
	default:
		return "", fmt.Errorf("unsupported union case type %s", t.String())
	}
}

func sortedEnumDiscriminants(values map[int]int8) []int {
	discriminants := make([]int, 0, len(values))
	for discriminant := range values {
		discriminants = append(discriminants, discriminant)
	}
	sort.Ints(discriminants)
	return discriminants
}

func sortedIntCaseKeys(cases map[int]*checker.Block) []int {
	keys := make([]int, 0, len(cases))
	for value := range cases {
		keys = append(keys, value)
	}
	sort.Ints(keys)
	return keys
}

func sortedIntRangeKeys(cases map[checker.IntRange]*checker.Block) []checker.IntRange {
	ranges := make([]checker.IntRange, 0, len(cases))
	for intRange := range cases {
		ranges = append(ranges, intRange)
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start != ranges[j].Start {
			return ranges[i].Start < ranges[j].Start
		}
		return ranges[i].End < ranges[j].End
	})
	return ranges
}

func sortedStructInstanceFields(instance *checker.StructInstance) []string {
	if instance == nil {
		return nil
	}
	if structDef, ok := instance.StructType.(*checker.StructDef); ok && structDef != nil {
		return sortedFieldNames(structDef.Fields)
	}
	if len(instance.FieldTypes) > 0 {
		return sortedFieldNames(instance.FieldTypes)
	}
	fields := make([]string, 0, len(instance.Fields))
	for field := range instance.Fields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func sortedFunctionNames(methods map[string]*checker.FunctionDef) []string {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedFieldNames(fields map[string]checker.Type) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func moduleAlias(path string) string {
	replacer := strings.NewReplacer("/", "_", "-", "_", ".", "_")
	return jsName(replacer.Replace(path))
}

func moduleOutputPath(path string) string {
	return filepath.ToSlash(path) + ".mjs"
}

func relativeJSImport(fromOutputPath string, toOutputPath string) string {
	fromDir := filepath.Dir(filepath.FromSlash(fromOutputPath))
	toPath := filepath.FromSlash(toOutputPath)
	rel, err := filepath.Rel(fromDir, toPath)
	if err != nil {
		return "./" + filepath.ToSlash(toOutputPath)
	}
	out := filepath.ToSlash(rel)
	if !strings.HasPrefix(out, "./") && !strings.HasPrefix(out, "../") {
		out = "./" + out
	}
	return out
}

func enumMethodName(enumName, methodName string) string {
	return "__enum_method__" + jsName(enumName) + "__" + jsName(methodName)
}

func isJSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '$' && r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '$' && r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func jsName(name string) string {
	replacer := strings.NewReplacer("::", "__", "-", "_", ".", "_")
	out := replacer.Replace(name)
	switch out {
	case "break", "case", "catch", "class", "const", "continue", "debugger", "default", "delete", "do", "else", "export", "extends", "finally", "for", "function", "if", "import", "in", "instanceof", "new", "return", "super", "switch", "this", "throw", "try", "typeof", "var", "void", "while", "with", "yield", "let", "static", "enum", "await", "implements", "package", "protected", "interface", "private", "public", "null", "true", "false":
		return out + "_"
	default:
		return out
	}
}

func escapeTemplateLiteral(raw string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "`", "\\`", "${", "\\${")
	return replacer.Replace(raw)
}
