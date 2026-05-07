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
	t := l.program.Types[typeID]
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
		return renderJSExpr(jsNewExprIR{Ctor: jsName(l.program.Types[expr.Type].Name), Args: args}), nil
	case air.ExprGetField:
		target, err := l.lowerExpr(fn, *expr.Target)
		if err != nil {
			return "", err
		}
		field := l.program.Types[expr.Target.Type].Fields[expr.Field]
		return target + "." + jsName(field.Name), nil
	case air.ExprEnumVariant:
		t := l.program.Types[expr.Type]
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
		t := l.program.Types[typeID]
		if t.Kind == air.TypeStruct || t.Kind == air.TypeEnum {
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
