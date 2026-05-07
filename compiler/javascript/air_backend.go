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
	if rootID, ok := airJSRootFunction(l.program); ok {
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
	imports := []string{
		jsNamedImportLine([]string{"Maybe", "Result", "ardEq", "ardToString", "makeArdError", "makeEnum", "isEnumOf"}, relativeJSImport(outputPath, "ard.prelude.mjs")),
	}
	for _, importedID := range module.Imports {
		if importPath, ok := l.moduleFiles[importedID]; ok {
			imports = append(imports, jsNamespaceImportLine(moduleAlias(l.program.Modules[importedID].Path), relativeJSImport(outputPath, importPath)))
		}
	}
	b.WriteString(renderJSDoc(jsModulePreambleDoc(imports, l.target)))
	b.WriteByte('\n')
	for _, typeID := range module.Types {
		decl, err := l.lowerTypeDecl(typeID)
		if err != nil {
			return "", fmt.Errorf("module %s type %d: %w", module.Path, typeID, err)
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
	if invokeRoot {
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
	params := make([]string, len(fn.Signature.Params))
	for i, param := range fn.Signature.Params {
		params[i] = jsName(param.Name)
	}
	body, err := l.lowerBlock(fn, fn.Body, true)
	if err != nil {
		return "", err
	}
	return renderJSDoc(jsBlockDoc("function "+l.functionName(fn.ID)+"("+strings.Join(params, ", ")+")", body)), nil
}

func (l *airJSLowerer) lowerScriptFunction(fn air.Function) (string, error) {
	body, err := l.lowerBlock(fn, fn.Body, true)
	if err != nil {
		return "", err
	}
	return renderJSDoc(jsBlockDoc("function "+l.functionName(fn.ID)+"()", body)), nil
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
	case air.StmtExpr:
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
	case air.ExprCall:
		args, err := l.lowerArgs(fn, expr.Args)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsCallExprIR{Callee: l.functionName(expr.Function), Args: args}), nil
	case air.ExprMakeStruct:
		args := make([]string, len(expr.Fields))
		for i, field := range expr.Fields {
			value, err := l.lowerExpr(fn, field.Value)
			if err != nil {
				return "", err
			}
			args[i] = value
		}
		t, ok := l.typeInfo(expr.Type)
		if !ok {
			return "", fmt.Errorf("unknown struct type %d", expr.Type)
		}
		return renderJSExpr(jsNewExprIR{Ctor: jsName(t.Name), Args: args}), nil
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
		return jsName(t.Name) + "." + jsName(t.Variants[expr.Variant].Name), nil
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
	case air.ExprMapKeys, air.ExprMapSize, air.ExprMapGet, air.ExprMapSet, air.ExprMapDrop, air.ExprMapHas:
		return l.lowerMapOp(fn, expr)
	case air.ExprMakeMaybeSome, air.ExprMakeMaybeNone, air.ExprMaybeExpect, air.ExprMaybeIsNone, air.ExprMaybeIsSome, air.ExprMaybeOr, air.ExprMaybeMap, air.ExprMaybeAndThen:
		return l.lowerMaybeOp(fn, expr)
	case air.ExprMakeResultOk, air.ExprMakeResultErr, air.ExprResultExpect, air.ExprResultOr, air.ExprResultIsOk, air.ExprResultIsErr, air.ExprResultMap, air.ExprResultMapErr, air.ExprResultAndThen:
		return l.lowerResultOp(fn, expr)
	case air.ExprStrAt, air.ExprStrSize, air.ExprStrIsEmpty, air.ExprStrContains, air.ExprStrReplace, air.ExprStrReplaceAll, air.ExprStrSplit, air.ExprStrStartsWith, air.ExprStrTrim:
		return l.lowerStrOp(fn, expr)
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
	case air.ExprToStr:
		value, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		return renderJSExpr(jsCallExprIR{Callee: "ardToString", Args: []string{value}}), nil
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
	if int(local) >= 0 && int(local) < len(fn.Locals) {
		return jsName(fn.Locals[local].Name)
	}
	return "__local" + strconv.Itoa(int(local))
}

func (l *airJSLowerer) functionName(id air.FunctionID) string {
	if id == air.NoFunction || int(id) < 0 || int(id) >= len(l.program.Functions) {
		return "__missing_function"
	}
	fn := l.program.Functions[id]
	if fn.IsScript {
		return "__ard_script"
	}
	return jsName(fn.Name)
}

func (l *airJSLowerer) moduleExports(module air.Module) []string {
	exports := []string{}
	for _, typeID := range module.Types {
		t, ok := l.typeInfo(typeID)
		if ok && (t.Kind == air.TypeStruct || t.Kind == air.TypeEnum) {
			exports = append(exports, jsName(t.Name))
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
				continue
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

func airJSRootFunction(program *air.Program) (air.FunctionID, bool) {
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
