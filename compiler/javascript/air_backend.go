package javascript

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
)

// FFIArtifacts describes JavaScript companion files used by generated output.
type FFIArtifacts = ffiArtifacts

// Options configures AIR-backed JavaScript source generation.
type Options struct {
	Target       string
	RootFileName string
	InvokeMain   bool
}

func GenerateSources(program *air.Program, options Options) (map[string][]byte, FFIArtifacts, error) {
	files, ffi, err := generateSourcesFromAIR(program, options)
	return files, ffi, err
}

func RunProgram(program *air.Program, target string, _ []string, projectInfo *checker.ProjectInfo) error {
	if target == backend.TargetJSBrowser {
		return fmt.Errorf("js-browser cannot be run directly; build and serve the emitted module instead")
	}
	if target != backend.TargetJSServer {
		return fmt.Errorf("unsupported JavaScript run target: %s", target)
	}
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node is required to run js-server output: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "ard-js-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	files, ffi, err := GenerateSources(program, Options{Target: target, RootFileName: "main.mjs", InvokeMain: true})
	if err != nil {
		return err
	}
	if err := writeJSSources(tmpDir, files); err != nil {
		return err
	}
	if err := writeFFICompanions(tmpDir, target, projectInfo, ffi); err != nil {
		return err
	}
	cmd := exec.Command("node", filepath.Join(tmpDir, "main.mjs"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append([]string{}, os.Environ()...)
	return cmd.Run()
}

func BuildProgram(program *air.Program, outputPath string, target string, projectInfo *checker.ProjectInfo) (string, error) {
	resolvedOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	outputDir := filepath.Dir(resolvedOutputPath)
	rootFileName := filepath.Base(resolvedOutputPath)
	files, ffi, err := GenerateSources(program, Options{Target: target, RootFileName: rootFileName})
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}
	if err := writeJSSources(outputDir, files); err != nil {
		return "", err
	}
	if err := writeFFICompanions(outputDir, target, projectInfo, ffi); err != nil {
		return "", err
	}
	return outputPath, nil
}

func writeJSSources(outputDir string, files map[string][]byte) error {
	for relPath, source := range files {
		absPath := filepath.Join(outputDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, source, 0o644); err != nil {
			return err
		}
	}
	return nil
}

type airJSLowerer struct {
	program     *air.Program
	target      string
	rootFile    string
	moduleFiles map[air.ModuleID]string
	tempCounter int
}

func generateSourcesFromAIR(program *air.Program, options Options) (map[string][]byte, FFIArtifacts, error) {
	if program == nil {
		return nil, FFIArtifacts{}, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, FFIArtifacts{}, err
	}
	target := options.Target
	if target == "" {
		target = backend.TargetJSServer
	}
	rootFile := options.RootFileName
	if rootFile == "" {
		rootFile = "main.mjs"
	}
	l := &airJSLowerer{program: program, target: target, rootFile: rootFile, moduleFiles: map[air.ModuleID]string{}}
	l.planModuleFiles()
	files := make(map[string][]byte, len(program.Modules))
	moduleIDs := make([]int, 0, len(program.Modules))
	for _, module := range program.Modules {
		moduleIDs = append(moduleIDs, int(module.ID))
	}
	sort.Ints(moduleIDs)
	for _, rawID := range moduleIDs {
		module := program.Modules[rawID]
		source, err := l.lowerModule(module, options.InvokeMain)
		if err != nil {
			return nil, FFIArtifacts{}, err
		}
		files[l.moduleFiles[module.ID]] = []byte(source)
	}
	return files, l.collectFFIArtifacts(), nil
}

func (l *airJSLowerer) planModuleFiles() {
	rootModule := air.ModuleID(-1)
	if rootID, ok := airJSRootModuleFunction(l.program); ok {
		rootModule = l.program.Functions[rootID].Module
	} else if len(l.program.Modules) > 0 {
		rootModule = l.program.Modules[len(l.program.Modules)-1].ID
	}
	for _, module := range l.program.Modules {
		if module.ID == rootModule {
			l.moduleFiles[module.ID] = l.rootFile
			continue
		}
		l.moduleFiles[module.ID] = moduleOutputPath(module.Path)
	}
}

func (l *airJSLowerer) lowerModule(module air.Module, invokeRoot bool) (string, error) {
	var b strings.Builder
	outputPath := l.moduleFiles[module.ID]
	preludeImport := relativeJSImport(outputPath, "ard.prelude.mjs")
	imports := []string{
		jsNamedImportLine([]string{"Maybe", "Result", "ardEq", "ardToString", "makeArdError", "makeEnum", "isEnumOf"}, preludeImport),
		jsNamespaceImportLine("prelude", preludeImport),
	}
	ffi := l.collectFFIArtifacts()
	if ffi.useStdlib {
		imports = append(imports, jsNamespaceImportLine("stdlib", relativeJSImport(outputPath, "ffi.stdlib."+l.target+".mjs")))
	}
	if ffi.useProject {
		imports = append(imports, jsNamespaceImportLine("project", relativeJSImport(outputPath, "ffi.project."+l.target+".mjs")))
	}
	for _, importedID := range l.moduleDependencyIDs(module) {
		if importPath, ok := l.moduleFiles[importedID]; ok {
			imports = append(imports, jsNamespaceImportLine(moduleAlias(l.program.Modules[importedID].Path), relativeJSImport(outputPath, importPath)))
		}
	}
	b.WriteString(renderJSDoc(jsModulePreambleDoc(imports, l.target)))
	b.WriteByte('\n')
	for _, typ := range l.program.Types {
		decl, err := l.lowerTypeDecl(typ.ID)
		if err != nil {
			return "", fmt.Errorf("module %s type %d: %w", module.Path, typ.ID, err)
		}
		if strings.TrimSpace(decl) != "" {
			b.WriteString(decl)
			b.WriteString("\n\n")
		}
	}
	functionIDs := append([]air.FunctionID(nil), module.Functions...)
	sort.Slice(functionIDs, func(i, j int) bool { return functionIDs[i] < functionIDs[j] })
	for _, functionID := range functionIDs {
		fn := l.program.Functions[functionID]
		if fn.IsScript {
			continue
		}
		decl, err := l.lowerFunction(fn)
		if err != nil {
			return "", fmt.Errorf("module %s function %s: %w", module.Path, fn.Name, err)
		}
		b.WriteString(decl)
		b.WriteString("\n\n")
	}
	if l.program.Script != air.NoFunction && l.program.Functions[l.program.Script].Module == module.ID {
		script, err := l.lowerScriptFunction(l.program.Functions[l.program.Script])
		if err != nil {
			return "", err
		}
		b.WriteString(script)
		b.WriteString("\n\n")
	}
	if invokeRoot && l.program.Script == air.NoFunction {
		if rootID, ok := airJSRootFunction(l.program); ok && l.program.Functions[rootID].Module == module.ID {
			b.WriteString("await ")
			b.WriteString(l.functionName(rootID))
			b.WriteString("();\n\n")
		}
	}
	exports := l.moduleExports(module)
	if len(exports) > 0 {
		b.WriteString(renderJSDoc(jsExportListDoc(exports)))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func (l *airJSLowerer) moduleDependencyIDs(module air.Module) []air.ModuleID {
	seen := map[air.ModuleID]bool{}
	add := func(id air.ModuleID) {
		if id != module.ID {
			seen[id] = true
		}
	}
	for _, importedID := range module.Imports {
		add(importedID)
	}
	var visitBlock func(air.Block)
	var visitExpr func(air.Expr)
	visitBlock = func(block air.Block) {
		for _, stmt := range block.Stmts {
			if stmt.Value != nil {
				visitExpr(*stmt.Value)
			}
			if stmt.Expr != nil {
				visitExpr(*stmt.Expr)
			}
			if stmt.Target != nil {
				visitExpr(*stmt.Target)
			}
			if stmt.Condition != nil {
				visitExpr(*stmt.Condition)
			}
			visitBlock(stmt.Body)
		}
		if block.Result != nil {
			visitExpr(*block.Result)
		}
	}
	visitExpr = func(expr air.Expr) {
		if expr.Kind == air.ExprCall && int(expr.Function) >= 0 && int(expr.Function) < len(l.program.Functions) {
			add(l.program.Functions[expr.Function].Module)
		}
		if expr.Kind == air.ExprMakeStruct || expr.Kind == air.ExprEnumVariant {
			if owner, ok := l.typeModule(expr.Type); ok {
				add(owner)
			}
		}
		for _, arg := range expr.Args {
			visitExpr(arg)
		}
		for _, entry := range expr.Entries {
			visitExpr(entry.Key)
			visitExpr(entry.Value)
		}
		for _, field := range expr.Fields {
			visitExpr(field.Value)
		}
		for _, matchCase := range expr.EnumCases {
			visitBlock(matchCase.Body)
		}
		for _, matchCase := range expr.IntCases {
			visitBlock(matchCase.Body)
		}
		for _, matchCase := range expr.RangeCases {
			visitBlock(matchCase.Body)
		}
		for _, matchCase := range expr.UnionCases {
			visitBlock(matchCase.Body)
		}
		if expr.Target != nil {
			visitExpr(*expr.Target)
		}
		if expr.Left != nil {
			visitExpr(*expr.Left)
		}
		if expr.Right != nil {
			visitExpr(*expr.Right)
		}
		if expr.Condition != nil {
			visitExpr(*expr.Condition)
		}
		visitBlock(expr.Body)
		visitBlock(expr.Then)
		visitBlock(expr.Else)
		visitBlock(expr.CatchAll)
		visitBlock(expr.Some)
		visitBlock(expr.None)
		visitBlock(expr.Ok)
		visitBlock(expr.Err)
		visitBlock(expr.Catch)
	}
	for _, functionID := range module.Functions {
		if int(functionID) >= 0 && int(functionID) < len(l.program.Functions) {
			visitBlock(l.program.Functions[functionID].Body)
		}
	}
	if l.program.Script != air.NoFunction && int(l.program.Script) < len(l.program.Functions) && l.program.Functions[l.program.Script].Module == module.ID {
		visitBlock(l.program.Functions[l.program.Script].Body)
	}
	ids := make([]air.ModuleID, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (l *airJSLowerer) lowerTypeDecl(typeID air.TypeID) (string, error) {
	t, ok := l.typeInfo(typeID)
	if !ok {
		return "", fmt.Errorf("unknown AIR type %d", typeID)
	}
	switch t.Kind {
	case air.TypeStruct:
		params := make([]string, len(t.Fields))
		for i, field := range t.Fields {
			params[i] = jsName(field.Name)
		}
		lines := make([]string, 0, len(t.Fields))
		for _, field := range t.Fields {
			name := jsName(field.Name)
			lines = append(lines, "this."+name+" = "+name+";")
		}
		ctor := renderJSDoc(jsBlockDoc("constructor("+strings.Join(params, ", ")+")", strings.Join(lines, "\n")))
		return renderJSDoc(jsClassDoc(jsName(t.Name), []string{ctor})), nil
	case air.TypeEnum:
		fields := make([]jsObjectField, 0, len(t.Variants))
		for _, variant := range t.Variants {
			fields = append(fields, jsObjectField{Key: jsName(variant.Name), Value: renderJSDoc(jsCallDoc("makeEnum", []string{strconv.Quote(t.Name), strconv.Quote(variant.Name), strconv.Itoa(variant.Discriminant)}))})
		}
		return renderJSDoc(jsVarDeclDoc("const", jsName(t.Name), renderJSDoc(jsCallDoc("Object.freeze", []string{renderJSDoc(jsObjectDoc(fields))})))), nil
	default:
		return "", nil
	}
}

func (l *airJSLowerer) lowerFunction(fn air.Function) (string, error) {
	params := make([]string, 0, len(fn.Captures)+len(fn.Signature.Params))
	for _, capture := range fn.Captures {
		params = append(params, jsName(capture.Name))
	}
	for _, param := range fn.Signature.Params {
		params = append(params, jsName(param.Name))
	}
	body, err := l.lowerBlock(fn, fn.Body, true)
	if err != nil {
		return "", err
	}
	return renderJSDoc(jsBlockDoc("function "+l.functionName(fn.ID)+"("+strings.Join(params, ", ")+")", body)), nil
}

func (l *airJSLowerer) lowerScriptFunction(fn air.Function) (string, error) {
	return l.lowerBlock(fn, fn.Body, false)
}

func (l *airJSLowerer) lowerBlock(fn air.Function, block air.Block, returns bool) (string, error) {
	var lines []string
	for _, stmt := range block.Stmts {
		line, err := l.lowerStmt(fn, stmt)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if block.Result != nil {
		if returns && (block.Result.Kind == air.ExprTryResult || block.Result.Kind == air.ExprTryMaybe) {
			tryLines, err := l.lowerTryTail(fn, *block.Result)
			if err != nil {
				return "", err
			}
			lines = append(lines, tryLines...)
			return strings.Join(lines, "\n"), nil
		}
		expr, err := l.lowerExpr(fn, *block.Result)
		if err != nil {
			return "", err
		}
		if returns {
			lines = append(lines, "return "+expr+";")
		} else {
			lines = append(lines, expr+";")
		}
	} else if returns {
		lines = append(lines, "return undefined;")
	}
	return strings.Join(lines, "\n"), nil
}

func (l *airJSLowerer) lowerStmt(fn air.Function, stmt air.Stmt) (string, error) {
	switch stmt.Kind {
	case air.StmtLet:
		if stmt.Value != nil && (stmt.Value.Kind == air.ExprTryResult || stmt.Value.Kind == air.ExprTryMaybe) {
			return l.lowerTryLet(fn, stmt)
		}
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return "", err
		}
		keyword := "const"
		if stmt.Mutable {
			keyword = "let"
		}
		return keyword + " " + l.localName(fn, stmt.Local) + " = " + value + ";", nil
	case air.StmtAssign:
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return "", err
		}
		return l.localName(fn, stmt.Local) + " = " + value + ";", nil
	case air.StmtSetField:
		target, err := l.lowerExpr(fn, *stmt.Target)
		if err != nil {
			return "", err
		}
		value, err := l.lowerExpr(fn, *stmt.Value)
		if err != nil {
			return "", err
		}
		t, ok := l.typeInfo(stmt.Target.Type)
		if !ok || stmt.Field < 0 || stmt.Field >= len(t.Fields) {
			return "", fmt.Errorf("unknown field %d on type %d", stmt.Field, stmt.Target.Type)
		}
		return target + "." + jsName(t.Fields[stmt.Field].Name) + " = " + value + ";", nil
	case air.StmtExpr:
		if stmt.Expr != nil && stmt.Expr.Kind == air.ExprIf {
			return l.lowerIfStmt(fn, *stmt.Expr)
		}
		value, err := l.lowerExpr(fn, *stmt.Expr)
		if err != nil {
			return "", err
		}
		return value + ";", nil
	case air.StmtWhile:
		condition, err := l.lowerExpr(fn, *stmt.Condition)
		if err != nil {
			return "", err
		}
		body, err := l.lowerBlock(fn, stmt.Body, false)
		if err != nil {
			return "", err
		}
		return renderJSDoc(jsBlockDoc("while ("+condition+")", body)), nil
	case air.StmtBreak:
		return "break;", nil
	default:
		return "", fmt.Errorf("unsupported AIR JS statement kind %d", stmt.Kind)
	}
}

func (l *airJSLowerer) lowerIfStmt(fn air.Function, expr air.Expr) (string, error) {
	condition, err := l.lowerExpr(fn, *expr.Condition)
	if err != nil {
		return "", err
	}
	thenBody, err := l.lowerBlock(fn, expr.Then, false)
	if err != nil {
		return "", err
	}
	elseBody, err := l.lowerBlock(fn, expr.Else, false)
	if err != nil {
		return "", err
	}
	return renderJSDoc(jsIfDoc(condition, thenBody, jsBareBlockDoc(elseBody))), nil
}

func (l *airJSLowerer) lowerExpr(fn air.Function, expr air.Expr) (string, error) {
	switch expr.Kind {
	case air.ExprConstVoid:
		return "undefined", nil
	case air.ExprConstInt:
		return strconv.Itoa(expr.Int), nil
	case air.ExprConstFloat:
		return strconv.FormatFloat(expr.Float, 'g', -1, 64), nil
	case air.ExprConstBool:
		if expr.Bool {
			return "true", nil
		}
		return "false", nil
	case air.ExprConstStr:
		return strconv.Quote(expr.Str), nil
	case air.ExprLoadLocal:
		return l.localName(fn, expr.Local), nil
	case air.ExprUnionWrap:
		return l.lowerUnionWrap(fn, expr)
	case air.ExprMatchUnion:
		return l.lowerUnionMatch(fn, expr)
	case air.ExprCall:
		args, err := l.lowerArgs(fn, expr.Args)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsCallExprIR{Callee: l.functionRef(fn.Module, expr.Function), Args: args}), nil
	case air.ExprCallExtern:
		args, err := l.lowerArgs(fn, expr.Args)
		if err != nil {
			return "", err
		}
		callee, err := l.externRef(expr.Extern)
		if err != nil {
			return "", err
		}
		call := renderJSExpr(jsCallExprIR{Callee: callee, Args: args})
		return l.adaptExternReturn(call, expr.Type)
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
	case air.ExprMakeStruct:
		args := make([]string, len(expr.Fields))
		for i, field := range expr.Fields {
			value, err := l.lowerExpr(fn, field.Value)
			if err != nil {
				return "", err
			}
			args[i] = value
		}
		if _, ok := l.typeInfo(expr.Type); !ok {
			return "", fmt.Errorf("unknown struct type %d", expr.Type)
		}
		return renderJSExpr(jsNewExprIR{Ctor: l.typeRef(fn.Module, expr.Type), Args: args}), nil
	case air.ExprGetField:
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		t, ok := l.typeInfo(expr.Target.Type)
		if !ok || expr.Field < 0 || expr.Field >= len(t.Fields) {
			return "", fmt.Errorf("unknown field %d on type %d", expr.Field, expr.Target.Type)
		}
		field := t.Fields[expr.Field]
		return target + "." + jsName(field.Name), nil
	case air.ExprEnumVariant:
		t, ok := l.typeInfo(expr.Type)
		if !ok || expr.Variant < 0 || expr.Variant >= len(t.Variants) {
			return "", fmt.Errorf("unknown enum variant %d on type %d", expr.Variant, expr.Type)
		}
		return l.typeRef(fn.Module, expr.Type) + "." + jsName(t.Variants[expr.Variant].Name), nil
	case air.ExprMakeList:
		args, err := l.lowerArgs(fn, expr.Args)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsArrayExprIR{Items: args}), nil
	case air.ExprMakeMap:
		entries := make([]string, len(expr.Entries))
		for i, entry := range expr.Entries {
			key, err := l.lowerExpr(fn, entry.Key)
			if err != nil {
				return "", err
			}
			value, err := l.lowerExpr(fn, entry.Value)
			if err != nil {
				return "", err
			}
			entries[i] = renderJSExpr(jsArrayExprIR{Items: []string{key, value}})
		}
		return renderJSExpr(jsNewExprIR{Ctor: "Map", Args: []string{renderJSExpr(jsArrayExprIR{Items: entries})}}), nil
	case air.ExprListAt, air.ExprListPrepend, air.ExprListPush, air.ExprListSet, air.ExprListSize, air.ExprListSort, air.ExprListSwap:
		return l.lowerListOp(fn, expr)
	case air.ExprMapKeys, air.ExprMapSize, air.ExprMapGet, air.ExprMapSet, air.ExprMapDrop, air.ExprMapHas, air.ExprMapKeyAt, air.ExprMapValueAt:
		return l.lowerMapOp(fn, expr)
	case air.ExprMakeMaybeSome, air.ExprMakeMaybeNone, air.ExprMaybeExpect, air.ExprMaybeIsNone, air.ExprMaybeIsSome, air.ExprMaybeOr, air.ExprMaybeMap, air.ExprMaybeAndThen:
		return l.lowerMaybeOp(fn, expr)
	case air.ExprMakeResultOk, air.ExprMakeResultErr, air.ExprResultExpect, air.ExprResultOr, air.ExprResultIsOk, air.ExprResultIsErr, air.ExprResultMap, air.ExprResultMapErr, air.ExprResultAndThen:
		return l.lowerResultOp(fn, expr)
	case air.ExprStrAt, air.ExprStrSize, air.ExprStrIsEmpty, air.ExprStrContains, air.ExprStrReplace, air.ExprStrReplaceAll, air.ExprStrSplit, air.ExprStrStartsWith, air.ExprStrTrim:
		return l.lowerStrOp(fn, expr)
	case air.ExprTryResult, air.ExprTryMaybe:
		return l.lowerTryExpr(fn, expr)
	case air.ExprMatchEnum:
		return l.lowerEnumMatch(fn, expr)
	case air.ExprMatchInt:
		return l.lowerIntMatch(fn, expr)
	case air.ExprMatchMaybe:
		return l.lowerMaybeMatch(fn, expr)
	case air.ExprMatchResult:
		return l.lowerResultMatch(fn, expr)
	case air.ExprBlock:
		body, err := l.lowerBlock(fn, expr.Body, true)
		if err != nil {
			return "", err
		}
		return renderJSDoc(jsIIFEDoc(body)), nil
	case air.ExprIf:
		condition, err := l.lowerExpr(fn, *expr.Condition)
		if err != nil {
			return "", err
		}
		thenBody, err := l.lowerBlock(fn, expr.Then, true)
		if err != nil {
			return "", err
		}
		elseBody, err := l.lowerBlock(fn, expr.Else, true)
		if err != nil {
			return "", err
		}
		return renderJSDoc(jsIIFEDoc(renderJSDoc(jsIfDoc(condition, thenBody, jsBareBlockDoc(elseBody))))), nil
	case air.ExprIntDiv:
		return l.lowerBinaryCall(fn, expr, func(left, right string) string { return "Math.trunc((" + left + ") / (" + right + "))" })
	case air.ExprIntAdd, air.ExprFloatAdd, air.ExprStrConcat:
		return l.lowerBinaryOp(fn, expr, "+")
	case air.ExprIntSub, air.ExprFloatSub:
		return l.lowerBinaryOp(fn, expr, "-")
	case air.ExprIntMul, air.ExprFloatMul:
		return l.lowerBinaryOp(fn, expr, "*")
	case air.ExprIntMod:
		return l.lowerBinaryOp(fn, expr, "%")
	case air.ExprFloatDiv:
		return l.lowerBinaryOp(fn, expr, "/")
	case air.ExprLt:
		return l.lowerBinaryOp(fn, expr, "<")
	case air.ExprLte:
		return l.lowerBinaryOp(fn, expr, "<=")
	case air.ExprGt:
		return l.lowerBinaryOp(fn, expr, ">")
	case air.ExprGte:
		return l.lowerBinaryOp(fn, expr, ">=")
	case air.ExprEq:
		left, right, err := l.lowerBinaryParts(fn, expr)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsCallExprIR{Callee: "ardEq", Args: []string{left, right}}), nil
	case air.ExprNotEq:
		left, right, err := l.lowerBinaryParts(fn, expr)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsUnaryExprIR{Op: "!", Value: jsCallExprIR{Callee: "ardEq", Args: []string{left, right}}}), nil
	case air.ExprAnd:
		return l.lowerBinaryOp(fn, expr, "&&")
	case air.ExprOr:
		return l.lowerBinaryOp(fn, expr, "||")
	case air.ExprNot:
		value, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsUnaryExprIR{Op: "!", Value: rawJSExpr(value)}), nil
	case air.ExprNeg:
		value, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsUnaryExprIR{Op: "-", Value: rawJSExpr(value)}), nil
	case air.ExprCallTrait:
		return l.lowerTraitCall(fn, expr)
	case air.ExprToStr:
		value, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsCallExprIR{Callee: "ardToString", Args: []string{value}}), nil
	case air.ExprToDynamic, air.ExprTraitUpcast:
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprCopy:
		return l.lowerExpr(fn, *expr.Target)
	case air.ExprPanic:
		message, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		return renderJSDoc(jsIIFEDoc("throw makeArdError(\"panic\", \"air\", " + strconv.Quote(fn.Name) + ", 0, " + message + ");")), nil
	default:
		return "", fmt.Errorf("unsupported AIR JS expression kind %d", expr.Kind)
	}
}

func (l *airJSLowerer) lowerSpawnFiber(fn air.Function, expr air.Expr) (string, error) {
	var call string
	if expr.Target != nil {
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		call = renderJSExpr(jsCallExprIR{Callee: target})
	} else {
		call = renderJSExpr(jsCallExprIR{Callee: l.functionRef(fn.Module, expr.Function)})
	}
	return renderJSDoc(jsObjectDoc([]jsObjectField{{Key: "value", Value: call}, {Key: "done", Value: "true"}})), nil
}

func (l *airJSLowerer) lowerFiberGet(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("fiber get missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	return target + ".value", nil
}

func (l *airJSLowerer) lowerFiberJoin(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("fiber join missing target")
	}
	if _, err := l.lowerExpr(fn, *expr.Target); err != nil {
		return "", err
	}
	return "undefined", nil
}

func (l *airJSLowerer) lowerTraitCall(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("trait call missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	if expr.Trait < 0 || int(expr.Trait) >= len(l.program.Traits) {
		return "", fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := l.program.Traits[expr.Trait]
	if expr.Method < 0 || expr.Method >= len(trait.Methods) {
		return "", fmt.Errorf("invalid trait method %d for %s", expr.Method, trait.Name)
	}
	method := trait.Methods[expr.Method]
	if trait.Name == "ToString" && method.Name == "to_str" {
		return renderJSExpr(jsCallExprIR{Callee: "ardToString", Args: []string{target}}), nil
	}
	return "", fmt.Errorf("unsupported AIR JS trait call %s.%s", trait.Name, method.Name)
}

func (l *airJSLowerer) adaptExternReturn(call string, typeID air.TypeID) (string, error) {
	t, ok := l.typeInfo(typeID)
	if !ok {
		return call, nil
	}
	switch t.Kind {
	case air.TypeMaybe:
		adapted, err := l.adaptExternValue("__extern", t.Elem)
		if err != nil {
			return "", err
		}
		body := "const __extern = " + call + ";\nreturn (__extern === undefined || __extern === null) ? Maybe.none() : Maybe.some(" + adapted + ");"
		return renderJSDoc(jsIIFEDoc(body)), nil
	case air.TypeResult:
		adaptedOk, err := l.adaptExternValue("__extern.ok", t.Value)
		if err != nil {
			return "", err
		}
		adaptedErr, err := l.adaptExternValue("__extern.error", t.Error)
		if err != nil {
			return "", err
		}
		adaptedAltErr, err := l.adaptExternValue("__extern.err", t.Error)
		if err != nil {
			return "", err
		}
		body := "const __extern = " + call + ";\nif (__extern && Object.prototype.hasOwnProperty.call(__extern, \"ok\")) return Result.ok(" + adaptedOk + ");\nif (__extern && Object.prototype.hasOwnProperty.call(__extern, \"error\")) return Result.err(" + adaptedErr + ");\nif (__extern && Object.prototype.hasOwnProperty.call(__extern, \"err\")) return Result.err(" + adaptedAltErr + ");\nreturn __extern;"
		return renderJSDoc(jsIIFEDoc(body)), nil
	default:
		return l.adaptExternValue(call, typeID)
	}
}

func (l *airJSLowerer) adaptExternValue(value string, typeID air.TypeID) (string, error) {
	t, ok := l.typeInfo(typeID)
	if !ok {
		return value, nil
	}
	switch t.Kind {
	case air.TypeStruct:
		args := make([]string, len(t.Fields))
		for i, field := range t.Fields {
			adapted, err := l.adaptExternValue(value+"["+strconv.Quote(field.Name)+"]", field.Type)
			if err != nil {
				return "", err
			}
			args[i] = adapted
		}
		return "(" + value + " instanceof " + jsName(t.Name) + " ? " + value + " : new " + jsName(t.Name) + "(" + strings.Join(args, ", ") + "))", nil
	case air.TypeList:
		adapted, err := l.adaptExternValue("__item", t.Elem)
		if err != nil {
			return "", err
		}
		return "Array.isArray(" + value + ") ? " + value + ".map((__item) => " + adapted + ") : []", nil
	case air.TypeMap:
		adaptedKey, err := l.adaptExternValue("__key", t.Key)
		if err != nil {
			return "", err
		}
		adaptedValue, err := l.adaptExternValue("__value", t.Value)
		if err != nil {
			return "", err
		}
		body := "const __map = " + value + ";\nif (__map instanceof Map) return new Map(Array.from(__map.entries(), ([__key, __value]) => [" + adaptedKey + ", " + adaptedValue + "]));\nreturn new Map(Object.entries(__map ?? {}).map(([__key, __value]) => [" + adaptedKey + ", " + adaptedValue + "]));"
		return renderJSDoc(jsIIFEDoc(body)), nil
	case air.TypeMaybe:
		adapted, err := l.adaptExternValue("__maybe", t.Elem)
		if err != nil {
			return "", err
		}
		body := "const __maybe = " + value + ";\nreturn (__maybe === undefined || __maybe === null) ? Maybe.none() : Maybe.some(" + adapted + ");"
		return renderJSDoc(jsIIFEDoc(body)), nil
	default:
		return value, nil
	}
}

func (l *airJSLowerer) lowerTryLet(fn air.Function, stmt air.Stmt) (string, error) {
	tryExpr := *stmt.Value
	if tryExpr.Target == nil {
		return "", fmt.Errorf("try expression missing target")
	}
	target, err := l.lowerExpr(fn, *tryExpr.Target)
	if err != nil {
		return "", err
	}
	tryVar := l.temp("try")
	lines := []string{"const " + tryVar + " = " + target + ";"}
	if tryExpr.Kind == air.ExprTryResult {
		if tryExpr.HasCatch {
			catchLines, err := l.lowerTryCatchReturn(fn, tryExpr, tryVar)
			if err != nil {
				return "", err
			}
			lines = append(lines, "if ("+tryVar+".isErr()) "+renderJSDoc(jsBareBlockDoc(strings.Join(catchLines, "\n"))))
		} else {
			lines = append(lines, "if ("+tryVar+".isErr()) return Result.err("+tryVar+".error);")
		}
		lines = append(lines, "const "+l.localName(fn, stmt.Local)+" = "+tryVar+".ok;")
	} else {
		if tryExpr.HasCatch {
			catchLines, err := l.lowerTryCatchReturn(fn, tryExpr, tryVar)
			if err != nil {
				return "", err
			}
			lines = append(lines, "if ("+tryVar+".isNone()) "+renderJSDoc(jsBareBlockDoc(strings.Join(catchLines, "\n"))))
		} else {
			lines = append(lines, "if ("+tryVar+".isNone()) return Maybe.none();")
		}
		lines = append(lines, "const "+l.localName(fn, stmt.Local)+" = "+tryVar+".value;")
	}
	return strings.Join(lines, "\n"), nil
}

func (l *airJSLowerer) lowerTryTail(fn air.Function, expr air.Expr) ([]string, error) {
	if expr.Target == nil {
		return nil, fmt.Errorf("try expression missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return nil, err
	}
	tryVar := l.temp("try")
	lines := []string{"const " + tryVar + " = " + target + ";"}
	if expr.Kind == air.ExprTryResult {
		if expr.HasCatch {
			catchLines, err := l.lowerTryCatchReturn(fn, expr, tryVar)
			if err != nil {
				return nil, err
			}
			lines = append(lines, "if ("+tryVar+".isErr()) "+renderJSDoc(jsBareBlockDoc(strings.Join(catchLines, "\n"))))
		} else {
			lines = append(lines, "if ("+tryVar+".isErr()) return Result.err("+tryVar+".error);")
		}
		lines = append(lines, "return "+tryVar+".ok;")
	} else {
		if expr.HasCatch {
			catchLines, err := l.lowerTryCatchReturn(fn, expr, tryVar)
			if err != nil {
				return nil, err
			}
			lines = append(lines, "if ("+tryVar+".isNone()) "+renderJSDoc(jsBareBlockDoc(strings.Join(catchLines, "\n"))))
		} else {
			lines = append(lines, "if ("+tryVar+".isNone()) return Maybe.none();")
		}
		lines = append(lines, "return "+tryVar+".value;")
	}
	return lines, nil
}

func (l *airJSLowerer) lowerTryCatchReturn(fn air.Function, expr air.Expr, tryVar string) ([]string, error) {
	bindings := []string{}
	if expr.Kind == air.ExprTryResult && expr.CatchLocal >= 0 {
		bindings = append(bindings, "const "+l.localName(fn, expr.CatchLocal)+" = "+tryVar+".error;")
	}
	body, err := l.lowerBlockWithBindings(fn, expr.Catch, bindings)
	if err != nil {
		return nil, err
	}
	return strings.Split(body, "\n"), nil
}

func (l *airJSLowerer) lowerTryExpr(fn air.Function, expr air.Expr) (string, error) {
	lines, err := l.lowerTryTail(fn, expr)
	if err != nil {
		return "", err
	}
	return renderJSDoc(jsIIFEDoc(strings.Join(lines, "\n"))), nil
}

func (l *airJSLowerer) lowerUnionWrap(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("union wrap missing target")
	}
	value, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	return renderJSDoc(jsObjectDoc([]jsObjectField{{Key: "__ard_union_tag", Value: strconv.Itoa(int(expr.Tag))}, {Key: "value", Value: value}})), nil
}

func (l *airJSLowerer) lowerUnionMatch(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("union match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	matchVar := l.temp("match")
	lines := []string{"const " + matchVar + " = " + target + ";"}
	for _, unionCase := range expr.UnionCases {
		bindings := []string{}
		if unionCase.Local >= 0 {
			bindings = append(bindings, "const "+l.localName(fn, unionCase.Local)+" = "+matchVar+".value;")
		}
		body, err := l.lowerBlockWithBindings(fn, unionCase.Body, bindings)
		if err != nil {
			return "", err
		}
		lines = append(lines, "if ("+matchVar+".__ard_union_tag === "+strconv.Itoa(int(unionCase.Tag))+") "+renderJSDoc(jsBareBlockDoc(body)))
	}
	if !airBlockIsZero(expr.CatchAll) {
		body, err := l.lowerBlock(fn, expr.CatchAll, true)
		if err != nil {
			return "", err
		}
		lines = append(lines, renderJSDoc(jsBareBlockDoc(body)))
	} else {
		lines = append(lines, `throw makeArdError("panic", "match", "union", 0, "non-exhaustive union match");`)
	}
	return renderJSDoc(jsIIFEDoc(strings.Join(lines, "\n"))), nil
}

func (l *airJSLowerer) lowerMakeClosure(fn air.Function, expr air.Expr) (string, error) {
	if int(expr.Function) < 0 || int(expr.Function) >= len(l.program.Functions) {
		return "", fmt.Errorf("invalid closure function %d", expr.Function)
	}
	closureFn := l.program.Functions[expr.Function]
	params := make([]string, 0, len(closureFn.Signature.Params))
	callArgs := make([]string, 0, len(expr.CaptureLocals)+len(closureFn.Signature.Params))
	for _, local := range expr.CaptureLocals {
		callArgs = append(callArgs, l.localName(fn, local))
	}
	for _, param := range closureFn.Signature.Params {
		name := jsName(param.Name)
		params = append(params, name)
		callArgs = append(callArgs, name)
	}
	call := renderJSExpr(jsCallExprIR{Callee: l.functionRef(fn.Module, expr.Function), Args: callArgs})
	body := "return " + call + ";"
	return renderJSDoc(jsBlockDoc("function("+strings.Join(params, ", ")+")", body)), nil
}

func (l *airJSLowerer) lowerCallClosure(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("call closure missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	args, err := l.lowerArgs(fn, expr.Args)
	if err != nil {
		return "", err
	}
	return renderJSExpr(jsCallExprIR{Callee: target, Args: args}), nil
}

func (l *airJSLowerer) lowerBlockWithBindings(fn air.Function, block air.Block, bindings []string) (string, error) {
	body, err := l.lowerBlock(fn, block, true)
	if err != nil {
		return "", err
	}
	if len(bindings) == 0 {
		return body, nil
	}
	return strings.Join(append(bindings, body), "\n"), nil
}

func (l *airJSLowerer) lowerEnumMatch(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("enum match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	targetType, ok := l.typeInfo(expr.Target.Type)
	if !ok {
		return "", fmt.Errorf("unknown enum match type %d", expr.Target.Type)
	}
	matchVar := l.temp("match")
	lines := []string{"const " + matchVar + " = " + target + ";"}
	for _, matchCase := range expr.EnumCases {
		body, err := l.lowerBlock(fn, matchCase.Body, true)
		if err != nil {
			return "", err
		}
		lines = append(lines, "if (isEnumOf("+matchVar+", "+strconv.Quote(targetType.Name)+") && "+matchVar+".value === "+strconv.Itoa(matchCase.Discriminant)+") "+renderJSDoc(jsBareBlockDoc(body)))
	}
	if !airBlockIsZero(expr.CatchAll) {
		body, err := l.lowerBlock(fn, expr.CatchAll, true)
		if err != nil {
			return "", err
		}
		lines = append(lines, renderJSDoc(jsBareBlockDoc(body)))
	} else {
		lines = append(lines, `throw makeArdError("panic", "match", "enum", 0, "non-exhaustive enum match");`)
	}
	return renderJSDoc(jsIIFEDoc(strings.Join(lines, "\n"))), nil
}

func (l *airJSLowerer) lowerIntMatch(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("int match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	matchVar := l.temp("match")
	lines := []string{"const " + matchVar + " = " + target + ";"}
	for _, matchCase := range expr.IntCases {
		body, err := l.lowerBlock(fn, matchCase.Body, true)
		if err != nil {
			return "", err
		}
		lines = append(lines, "if ("+matchVar+" === "+strconv.Itoa(matchCase.Value)+") "+renderJSDoc(jsBareBlockDoc(body)))
	}
	for _, matchCase := range expr.RangeCases {
		body, err := l.lowerBlock(fn, matchCase.Body, true)
		if err != nil {
			return "", err
		}
		lines = append(lines, "if ("+matchVar+" >= "+strconv.Itoa(matchCase.Start)+" && "+matchVar+" <= "+strconv.Itoa(matchCase.End)+") "+renderJSDoc(jsBareBlockDoc(body)))
	}
	if !airBlockIsZero(expr.CatchAll) {
		body, err := l.lowerBlock(fn, expr.CatchAll, true)
		if err != nil {
			return "", err
		}
		lines = append(lines, renderJSDoc(jsBareBlockDoc(body)))
	} else {
		lines = append(lines, `throw makeArdError("panic", "match", "int", 0, "non-exhaustive int match");`)
	}
	return renderJSDoc(jsIIFEDoc(strings.Join(lines, "\n"))), nil
}

func (l *airJSLowerer) lowerMaybeMatch(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("maybe match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	matchVar := l.temp("match")
	bindings := []string{}
	if expr.SomeLocal >= 0 {
		bindings = append(bindings, "const "+l.localName(fn, expr.SomeLocal)+" = "+matchVar+".value;")
	}
	someBody, err := l.lowerBlockWithBindings(fn, expr.Some, bindings)
	if err != nil {
		return "", err
	}
	noneBody, err := l.lowerBlock(fn, expr.None, true)
	if err != nil {
		return "", err
	}
	lines := []string{"const " + matchVar + " = " + target + ";", renderJSDoc(jsIfDoc(matchVar+".isSome()", someBody, jsBareBlockDoc(noneBody)))}
	return renderJSDoc(jsIIFEDoc(strings.Join(lines, "\n"))), nil
}

func (l *airJSLowerer) lowerResultMatch(fn air.Function, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("result match missing target")
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	matchVar := l.temp("match")
	okBindings := []string{}
	if expr.OkLocal >= 0 {
		okBindings = append(okBindings, "const "+l.localName(fn, expr.OkLocal)+" = "+matchVar+".ok;")
	}
	okBody, err := l.lowerBlockWithBindings(fn, expr.Ok, okBindings)
	if err != nil {
		return "", err
	}
	errBindings := []string{}
	if expr.ErrLocal >= 0 {
		errBindings = append(errBindings, "const "+l.localName(fn, expr.ErrLocal)+" = "+matchVar+".error;")
	}
	errBody, err := l.lowerBlockWithBindings(fn, expr.Err, errBindings)
	if err != nil {
		return "", err
	}
	lines := []string{"const " + matchVar + " = " + target + ";", renderJSDoc(jsIfDoc(matchVar+".isOk()", okBody, jsBareBlockDoc(errBody)))}
	return renderJSDoc(jsIIFEDoc(strings.Join(lines, "\n"))), nil
}

func (l *airJSLowerer) lowerListOp(fn air.Function, expr air.Expr) (string, error) {
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	args, err := l.lowerArgs(fn, expr.Args)
	if err != nil {
		return "", err
	}
	switch expr.Kind {
	case air.ExprListAt:
		return target + "[" + args[0] + "]", nil
	case air.ExprListSize:
		return target + ".length", nil
	case air.ExprListPrepend:
		return renderJSDoc(jsIIFEDoc("const __value = " + target + ";\n__value.unshift(" + args[0] + ");\nreturn __value;")), nil
	case air.ExprListPush:
		return renderJSDoc(jsIIFEDoc("const __value = " + target + ";\n__value.push(" + args[0] + ");\nreturn __value;")), nil
	case air.ExprListSet:
		return renderJSDoc(jsIIFEDoc("const __value = " + target + ";\n__value[" + args[0] + "] = " + args[1] + ";\nreturn true;")), nil
	case air.ExprListSwap:
		body := "const __value = " + target + ";\nconst __tmp = __value[" + args[0] + "];\n__value[" + args[0] + "] = __value[" + args[1] + "];\n__value[" + args[1] + "] = __tmp;\nreturn undefined;"
		return renderJSDoc(jsIIFEDoc(body)), nil
	case air.ExprListSort:
		if len(args) != 1 {
			return "", fmt.Errorf("list sort expects comparator")
		}
		cmp := args[0]
		return renderJSDoc(jsIIFEDoc("const __value = " + target + ";\n__value.sort((a, b) => " + cmp + "(a, b) ? -1 : (" + cmp + "(b, a) ? 1 : 0));\nreturn undefined;")), nil
	default:
		return "", fmt.Errorf("unsupported AIR JS list op %d", expr.Kind)
	}
}

func (l *airJSLowerer) lowerMapOp(fn air.Function, expr air.Expr) (string, error) {
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	args, err := l.lowerArgs(fn, expr.Args)
	if err != nil {
		return "", err
	}
	switch expr.Kind {
	case air.ExprMapKeys:
		return renderJSExpr(jsCallExprIR{Callee: "Array.from", Args: []string{renderJSExpr(jsCallExprIR{Callee: target + ".keys"})}}), nil
	case air.ExprMapSize:
		return target + ".size", nil
	case air.ExprMapGet:
		return "(" + target + ".has(" + args[0] + ") ? Maybe.some(" + target + ".get(" + args[0] + ")) : Maybe.none())", nil
	case air.ExprMapSet:
		return renderJSDoc(jsIIFEDoc("const __value = " + target + ";\n__value.set(" + args[0] + ", " + args[1] + ");\nreturn true;")), nil
	case air.ExprMapDrop:
		return renderJSDoc(jsIIFEDoc("const __value = " + target + ";\n__value.delete(" + args[0] + ");\nreturn undefined;")), nil
	case air.ExprMapHas:
		return target + ".has(" + args[0] + ")", nil
	case air.ExprMapKeyAt:
		return "Array.from(" + target + ".keys())[" + args[0] + "]", nil
	case air.ExprMapValueAt:
		return "Array.from(" + target + ".values())[" + args[0] + "]", nil
	default:
		return "", fmt.Errorf("unsupported AIR JS map op %d", expr.Kind)
	}
}

func (l *airJSLowerer) lowerMaybeOp(fn air.Function, expr air.Expr) (string, error) {
	if expr.Kind == air.ExprMakeMaybeNone {
		return renderJSExpr(jsCallExprIR{Callee: "Maybe.none"}), nil
	}
	if expr.Kind == air.ExprMakeMaybeSome {
		value, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsCallExprIR{Callee: "Maybe.some", Args: []string{value}}), nil
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	args, err := l.lowerArgs(fn, expr.Args)
	if err != nil {
		return "", err
	}
	switch expr.Kind {
	case air.ExprMaybeExpect:
		return renderJSExpr(jsCallExprIR{Callee: target + ".expect", Args: args}), nil
	case air.ExprMaybeIsNone:
		return renderJSExpr(jsCallExprIR{Callee: target + ".isNone"}), nil
	case air.ExprMaybeIsSome:
		return renderJSExpr(jsCallExprIR{Callee: target + ".isSome"}), nil
	case air.ExprMaybeOr:
		return renderJSExpr(jsCallExprIR{Callee: target + ".or", Args: args}), nil
	case air.ExprMaybeMap:
		return renderJSExpr(jsCallExprIR{Callee: target + ".map", Args: args}), nil
	case air.ExprMaybeAndThen:
		return renderJSExpr(jsCallExprIR{Callee: target + ".andThen", Args: args}), nil
	default:
		return "", fmt.Errorf("unsupported AIR JS maybe op %d", expr.Kind)
	}
}

func (l *airJSLowerer) lowerResultOp(fn air.Function, expr air.Expr) (string, error) {
	if expr.Kind == air.ExprMakeResultOk || expr.Kind == air.ExprMakeResultErr {
		value, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		callee := "Result.ok"
		if expr.Kind == air.ExprMakeResultErr {
			callee = "Result.err"
		}
		return renderJSExpr(jsCallExprIR{Callee: callee, Args: []string{value}}), nil
	}
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	args, err := l.lowerArgs(fn, expr.Args)
	if err != nil {
		return "", err
	}
	switch expr.Kind {
	case air.ExprResultExpect:
		return renderJSExpr(jsCallExprIR{Callee: target + ".expect", Args: args}), nil
	case air.ExprResultOr:
		return renderJSExpr(jsCallExprIR{Callee: target + ".or", Args: args}), nil
	case air.ExprResultIsOk:
		return renderJSExpr(jsCallExprIR{Callee: target + ".isOk"}), nil
	case air.ExprResultIsErr:
		return renderJSExpr(jsCallExprIR{Callee: target + ".isErr"}), nil
	case air.ExprResultMap:
		return renderJSExpr(jsCallExprIR{Callee: target + ".map", Args: args}), nil
	case air.ExprResultMapErr:
		return renderJSExpr(jsCallExprIR{Callee: target + ".mapErr", Args: args}), nil
	case air.ExprResultAndThen:
		return renderJSExpr(jsCallExprIR{Callee: target + ".andThen", Args: args}), nil
	default:
		return "", fmt.Errorf("unsupported AIR JS result op %d", expr.Kind)
	}
}

func (l *airJSLowerer) lowerStrOp(fn air.Function, expr air.Expr) (string, error) {
	target, err := l.lowerExpr(fn, *expr.Target)
	if err != nil {
		return "", err
	}
	args, err := l.lowerArgs(fn, expr.Args)
	if err != nil {
		return "", err
	}
	switch expr.Kind {
	case air.ExprStrAt:
		return target + "[" + args[0] + "]", nil
	case air.ExprStrSize:
		return target + ".length", nil
	case air.ExprStrIsEmpty:
		return "(" + target + ".length === 0)", nil
	case air.ExprStrContains:
		return renderJSExpr(jsCallExprIR{Callee: target + ".includes", Args: args}), nil
	case air.ExprStrReplace:
		return renderJSExpr(jsCallExprIR{Callee: target + ".replace", Args: args}), nil
	case air.ExprStrReplaceAll:
		return renderJSExpr(jsCallExprIR{Callee: target + ".replaceAll", Args: args}), nil
	case air.ExprStrSplit:
		return renderJSExpr(jsCallExprIR{Callee: target + ".split", Args: args}), nil
	case air.ExprStrStartsWith:
		return renderJSExpr(jsCallExprIR{Callee: target + ".startsWith", Args: args}), nil
	case air.ExprStrTrim:
		return renderJSExpr(jsCallExprIR{Callee: target + ".trim"}), nil
	default:
		return "", fmt.Errorf("unsupported AIR JS str op %d", expr.Kind)
	}
}

func (l *airJSLowerer) lowerArgs(fn air.Function, args []air.Expr) ([]string, error) {
	out := make([]string, len(args))
	for i, arg := range args {
		value, err := l.lowerExpr(fn, arg)
		if err != nil {
			return nil, err
		}
		out[i] = value
	}
	return out, nil
}

func (l *airJSLowerer) lowerBinaryParts(fn air.Function, expr air.Expr) (string, string, error) {
	left, err := l.lowerExpr(fn, *expr.Left)
	if err != nil {
		return "", "", err
	}
	right, err := l.lowerExpr(fn, *expr.Right)
	if err != nil {
		return "", "", err
	}
	return left, right, nil
}

func (l *airJSLowerer) lowerBinaryOp(fn air.Function, expr air.Expr, op string) (string, error) {
	left, right, err := l.lowerBinaryParts(fn, expr)
	if err != nil {
		return "", err
	}
	return renderJSExpr(jsBinaryExprIR{Left: rawJSExpr(left), Op: op, Right: rawJSExpr(right)}), nil
}

func (l *airJSLowerer) lowerBinaryCall(fn air.Function, expr air.Expr, build func(string, string) string) (string, error) {
	left, right, err := l.lowerBinaryParts(fn, expr)
	if err != nil {
		return "", err
	}
	return build(left, right), nil
}

func (l *airJSLowerer) localName(fn air.Function, local air.LocalID) string {
	if int(local) < 0 || int(local) >= len(fn.Locals) {
		return "__local" + strconv.Itoa(int(local))
	}
	base := jsName(fn.Locals[local].Name)
	for id, candidate := range fn.Locals {
		if air.LocalID(id) != local && jsName(candidate.Name) == base {
			return base + "$" + strconv.Itoa(int(local))
		}
	}
	return base
}

func (l *airJSLowerer) typeModule(typeID air.TypeID) (air.ModuleID, bool) {
	for _, module := range l.program.Modules {
		for _, moduleType := range module.Types {
			if moduleType == typeID {
				return module.ID, true
			}
		}
	}
	return 0, false
}

func (l *airJSLowerer) typeRef(from air.ModuleID, typeID air.TypeID) string {
	t, ok := l.typeInfo(typeID)
	if !ok {
		return "__missing_type"
	}
	name := jsName(t.Name)
	moduleID, ok := l.typeModule(typeID)
	if !ok || moduleID == from {
		return name
	}
	if int(moduleID) >= 0 && int(moduleID) < len(l.program.Modules) {
		return moduleAlias(l.program.Modules[moduleID].Path) + "." + name
	}
	return name
}

func (l *airJSLowerer) functionName(id air.FunctionID) string {
	if id == air.NoFunction || int(id) < 0 || int(id) >= len(l.program.Functions) {
		return "__missing_function"
	}
	fn := l.program.Functions[id]
	if fn.IsScript {
		return "__ard_script"
	}
	name := jsName(fn.Name)
	if l.functionNameIsAmbiguous(fn) {
		return name + "__" + strconv.Itoa(int(id))
	}
	return name
}

func (l *airJSLowerer) functionNameIsAmbiguous(fn air.Function) bool {
	count := 0
	for _, other := range l.program.Functions {
		if other.Module == fn.Module && other.Name == fn.Name && !other.IsScript {
			count++
		}
	}
	return count > 1
}

func (l *airJSLowerer) functionRef(from air.ModuleID, id air.FunctionID) string {
	name := l.functionName(id)
	if id == air.NoFunction || int(id) < 0 || int(id) >= len(l.program.Functions) {
		return name
	}
	callee := l.program.Functions[id]
	if callee.Module == from {
		return name
	}
	if int(callee.Module) >= 0 && int(callee.Module) < len(l.program.Modules) {
		return moduleAlias(l.program.Modules[callee.Module].Path) + "." + name
	}
	return name
}

func (l *airJSLowerer) externRef(id air.ExternID) (string, error) {
	if int(id) < 0 || int(id) >= len(l.program.Externs) {
		return "", fmt.Errorf("unknown extern %d", id)
	}
	ext := l.program.Externs[id]
	binding := ext.Bindings[l.target]
	if binding == "" {
		binding = ext.Bindings["js"]
	}
	if binding == "" && len(ext.Bindings) == 1 {
		binding = ext.Bindings["go"]
	}
	if binding == "" {
		return "", fmt.Errorf("extern %s has no JavaScript binding for %s", ext.Name, l.target)
	}
	objectName := "project"
	if int(ext.Module) >= 0 && int(ext.Module) < len(l.program.Modules) && strings.HasPrefix(l.program.Modules[ext.Module].Path, "ard/") {
		objectName = "stdlib"
		if isAIRJSPreludeExtern(binding) {
			objectName = "prelude"
		}
	}
	if isJSIdentifier(binding) {
		return objectName + "." + binding, nil
	}
	return objectName + "[" + strconv.Quote(binding) + "]", nil
}

func isAIRJSPreludeExtern(binding string) bool {
	switch binding {
	case "JsonToDynamic", "DecodeString", "DecodeInt", "DecodeFloat", "DecodeBool", "IsNil",
		"DynamicToList", "DynamicToMap", "ExtractField", "IntFromStr", "FloatFromStr",
		"FloatFromInt", "FloatFloor", "StrToDynamic", "IntToDynamic",
		"FloatToDynamic", "BoolToDynamic", "VoidToDynamic", "ListToDynamic", "MapToDynamic",
		"JsonEncode", "promiseResolve", "promiseReject", "promiseMap", "promiseThen",
		"promiseRescue", "promiseInspect", "promiseInspectError", "promiseFinally",
		"promiseAll", "promiseRace", "promiseDelay", "fetchNative", "fetchResponseUrl",
		"fetchResponseStatus", "fetchResponseHeaders", "fetchResponseBody", "fetchErrorMessage":
		return true
	default:
		return false
	}
}

func (l *airJSLowerer) moduleExports(module air.Module) []string {
	exports := []string{}
	// lowerModule currently emits all struct/enum declarations into every generated
	// module so imported constructors are available even when AIR does not retain
	// a precise type-owner mapping. Keep exports aligned with the emitted
	// declarations so browser/server integration code can import enum values too.
	for _, typ := range l.program.Types {
		if typ.Kind == air.TypeStruct || typ.Kind == air.TypeEnum {
			exports = append(exports, jsName(typ.Name))
		}
	}
	for _, functionID := range module.Functions {
		fn := l.program.Functions[functionID]
		if fn.IsScript {
			continue
		}
		exports = append(exports, l.functionName(functionID))
	}
	sort.Strings(exports)
	return exports
}

func (l *airJSLowerer) temp(prefix string) string {
	name := "__" + prefix + strconv.Itoa(l.tempCounter)
	l.tempCounter++
	return name
}

func airBlockIsZero(block air.Block) bool {
	return len(block.Stmts) == 0 && block.Result == nil
}

func (l *airJSLowerer) typeInfo(id air.TypeID) (air.TypeInfo, bool) {
	if id <= 0 || int(id) > len(l.program.Types) {
		return air.TypeInfo{}, false
	}
	return l.program.Types[id-1], true
}

func (l *airJSLowerer) collectFFIArtifacts() FFIArtifacts {
	ffi := FFIArtifacts{}
	for _, ext := range l.program.Externs {
		modulePath := ""
		if int(ext.Module) >= 0 && int(ext.Module) < len(l.program.Modules) {
			modulePath = l.program.Modules[ext.Module].Path
		}
		if _, ok := ext.Bindings[l.target]; !ok {
			if _, hasJS := ext.Bindings["js"]; !hasJS {
				if _, singleGoFallback := ext.Bindings["go"]; !singleGoFallback || len(ext.Bindings) != 1 {
					continue
				}
			}
		}
		if strings.HasPrefix(modulePath, "ard/") {
			ffi.useStdlib = true
		} else {
			ffi.useProject = true
		}
	}
	return ffi
}

func airJSRootModuleFunction(program *air.Program) (air.FunctionID, bool) {
	if program == nil {
		return air.NoFunction, false
	}
	if program.Entry != air.NoFunction {
		return program.Entry, true
	}
	if program.Script != air.NoFunction {
		return program.Script, true
	}
	return air.NoFunction, false
}

func airJSRootFunction(program *air.Program) (air.FunctionID, bool) {
	if program == nil {
		return air.NoFunction, false
	}
	if program.Entry != air.NoFunction {
		return program.Entry, true
	}
	return air.NoFunction, false
}
