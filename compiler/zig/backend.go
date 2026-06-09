package zig

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
)

type Options struct {
	RootFileName string
}

func GenerateSources(program *air.Program, options Options) (map[string][]byte, error) {
	if program == nil {
		return nil, fmt.Errorf("AIR program is nil")
	}
	if err := air.Validate(program); err != nil {
		return nil, err
	}
	rootFile := options.RootFileName
	if rootFile == "" {
		rootFile = "main.zig"
	}
	l := &lowerer{
		program:          program,
		functionAdapters: map[string]functionAdapter{},
		closureAdapters:  map[air.FunctionID]closureAdapter{},
	}
	mainSource, err := l.lowerProgram()
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		rootFile:          []byte(mainSource),
		"ard_runtime.zig": []byte(runtimeSource),
	}, nil
}

func RunProgram(program *air.Program, args []string) error {
	workspaceDir, err := os.MkdirTemp("", "ard-zig-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspaceDir)
	if err := writeProgram(workspaceDir, program); err != nil {
		return err
	}
	binaryPath := filepath.Join(workspaceDir, "ard-program")
	if err := buildGeneratedProgram(workspaceDir, binaryPath); err != nil {
		return err
	}
	cmd := exec.Command(binaryPath, programArgs(args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func BuildProgram(program *air.Program, outputPath string) (string, error) {
	workspaceDir, err := os.MkdirTemp("", "ard-zig-build-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workspaceDir)
	if err := writeProgram(workspaceDir, program); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = "main"
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	if err := buildGeneratedProgram(workspaceDir, absOutput); err != nil {
		return "", err
	}
	return absOutput, nil
}

func writeProgram(dir string, program *air.Program) error {
	sources, err := GenerateSources(program, Options{})
	if err != nil {
		return err
	}
	for name, source := range sources {
		if err := os.WriteFile(filepath.Join(dir, name), source, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func buildGeneratedProgram(workspaceDir, binaryPath string) error {
	if _, err := exec.LookPath("zig"); err != nil {
		return fmt.Errorf("zig 0.16.0 is required to build zig target output: %w", err)
	}
	cmd := exec.Command("zig", "build-exe", "main.zig", "-O", "ReleaseSafe", "-femit-bin="+binaryPath)
	cmd.Dir = workspaceDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("zig build failed: %w\n%s", err, string(output))
	}
	return nil
}

func programArgs(args []string) []string {
	if len(args) <= 0 {
		return nil
	}
	if len(args) <= 3 {
		return nil
	}
	return append([]string(nil), args[3:]...)
}

type lowerer struct {
	program          *air.Program
	functionAdapters map[string]functionAdapter
	closureAdapters  map[air.FunctionID]closureAdapter
}

type functionAdapter struct {
	Function air.FunctionID
	Type     air.TypeID
}

type closureAdapter struct {
	Function air.FunctionID
}

func (l *lowerer) lowerProgram() (string, error) {
	var b strings.Builder
	b.WriteString("const std = @import(\"std\");\n")
	b.WriteString("const ard = @import(\"ard_runtime.zig\");\n\n")
	for _, typ := range l.program.Types {
		if typ.Kind != air.TypeStruct && typ.Kind != air.TypeEnum {
			continue
		}
		if err := l.lowerTypeDecl(&b, typ); err != nil {
			return "", err
		}
		b.WriteString("\n")
	}
	for _, fn := range l.program.Functions {
		if fn.IsTest {
			continue
		}
		if err := l.lowerFunction(&b, fn); err != nil {
			return "", err
		}
		b.WriteString("\n")
	}
	if err := l.lowerFunctionValueAdapters(&b); err != nil {
		return "", err
	}
	b.WriteString("pub fn main(init: std.process.Init) !void {\n")
	if l.program.Entry != air.NoFunction {
		b.WriteString("    var arena_state = std.heap.ArenaAllocator.init(std.heap.page_allocator);\n")
		b.WriteString("    defer arena_state.deinit();\n")
		b.WriteString("    var ctx = ard.Context{ .allocator = arena_state.allocator(), .io = init.io };\n")
		b.WriteString("    try ")
		b.WriteString(functionName(l.program.Functions[l.program.Entry]))
		b.WriteString("(&ctx);\n")
	} else {
		b.WriteString("    _ = init;\n")
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func (l *lowerer) lowerTypeDecl(b *strings.Builder, typ air.TypeInfo) error {
	switch typ.Kind {
	case air.TypeStruct:
		return l.lowerStructType(b, typ)
	case air.TypeEnum:
		return l.lowerEnumType(b, typ)
	default:
		return fmt.Errorf("unsupported Zig type declaration %s", typ.Name)
	}
}

func (l *lowerer) lowerStructType(b *strings.Builder, typ air.TypeInfo) error {
	fmt.Fprintf(b, "const %s = struct {\n", typeDeclName(typ))
	for _, field := range typ.Fields {
		fieldType, err := l.fieldTypeName(field)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "    %s: %s,\n", sanitizeIdentifier(field.Name), fieldType)
	}
	b.WriteString("};\n")
	return nil
}

func (l *lowerer) lowerEnumType(b *strings.Builder, typ air.TypeInfo) error {
	typeName := typeDeclName(typ)
	fmt.Fprintf(b, "const %s = i64;\n", typeName)
	for _, variant := range typ.Variants {
		fmt.Fprintf(b, "const %s_%s: %s = %d;\n", typeName, sanitizeIdentifier(variant.Name), typeName, variant.Discriminant)
	}
	return nil
}

func (l *lowerer) lowerFunctionValueAdapters(b *strings.Builder) error {
	for _, adapter := range l.closureAdapters {
		if err := l.lowerClosureAdapter(b, adapter); err != nil {
			return err
		}
		b.WriteString("\n")
	}
	for _, adapter := range l.functionAdapters {
		if err := l.lowerFunctionAdapter(b, adapter); err != nil {
			return err
		}
		b.WriteString("\n")
	}
	return nil
}

func (l *lowerer) lowerFunctionAdapter(b *strings.Builder, adapter functionAdapter) error {
	if int(adapter.Function) < 0 || int(adapter.Function) >= len(l.program.Functions) {
		return fmt.Errorf("function adapter %d out of range", adapter.Function)
	}
	fn := l.program.Functions[adapter.Function]
	typeInfo, err := l.functionTypeInfo(adapter.Type)
	if err != nil {
		return err
	}
	ret, err := l.typeName(typeInfo.Return)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "fn %s(ctx: *ard.Context, env: ?*anyopaque", functionAdapterName(adapter.Function, adapter.Type))
	if err := l.writeFunctionValueParams(b, typeInfo); err != nil {
		return err
	}
	if ret == "void" {
		b.WriteString(") !void {\n")
	} else {
		fmt.Fprintf(b, ") !%s {\n", ret)
	}
	b.WriteString("    _ = env;\n")
	args := []string{"ctx"}
	for i := range typeInfo.Params {
		args = append(args, fmt.Sprintf("a%d", i))
	}
	if ret == "void" {
		fmt.Fprintf(b, "    try %s(%s);\n", functionName(fn), strings.Join(args, ", "))
	} else {
		fmt.Fprintf(b, "    return try %s(%s);\n", functionName(fn), strings.Join(args, ", "))
	}
	b.WriteString("}\n")
	return nil
}

func (l *lowerer) lowerClosureAdapter(b *strings.Builder, adapter closureAdapter) error {
	if int(adapter.Function) < 0 || int(adapter.Function) >= len(l.program.Functions) {
		return fmt.Errorf("closure adapter %d out of range", adapter.Function)
	}
	fn := l.program.Functions[adapter.Function]
	if len(fn.Captures) > 0 {
		fmt.Fprintf(b, "const %s = struct {\n", closureEnvName(fn))
		for i, capture := range fn.Captures {
			captureType, err := l.typeName(capture.Type)
			if err != nil {
				return err
			}
			fmt.Fprintf(b, "    c%d: %s,\n", i, captureType)
		}
		b.WriteString("};\n\n")
	}
	typeInfo := air.TypeInfo{Kind: air.TypeFunction, Params: make([]air.TypeID, len(fn.Signature.Params)), Return: fn.Signature.Return}
	for i, param := range fn.Signature.Params {
		typeInfo.Params[i] = param.Type
	}
	ret, err := l.typeName(fn.Signature.Return)
	if err != nil {
		return err
	}
	fmt.Fprintf(b, "fn %s(ctx: *ard.Context, env_ptr: ?*anyopaque", closureAdapterName(fn.ID))
	if err := l.writeFunctionValueParams(b, typeInfo); err != nil {
		return err
	}
	if ret == "void" {
		b.WriteString(") !void {\n")
	} else {
		fmt.Fprintf(b, ") !%s {\n", ret)
	}
	args := []string{"ctx"}
	if len(fn.Captures) > 0 {
		fmt.Fprintf(b, "    const env: *%s = @ptrCast(@alignCast(env_ptr.?));\n", closureEnvName(fn))
		for i := range fn.Captures {
			args = append(args, fmt.Sprintf("env.c%d", i))
		}
	} else {
		b.WriteString("    _ = env_ptr;\n")
	}
	for i := range fn.Signature.Params {
		args = append(args, fmt.Sprintf("a%d", i))
	}
	if ret == "void" {
		fmt.Fprintf(b, "    try %s(%s);\n", functionName(fn), strings.Join(args, ", "))
	} else {
		fmt.Fprintf(b, "    return try %s(%s);\n", functionName(fn), strings.Join(args, ", "))
	}
	b.WriteString("}\n")
	return nil
}

func (l *lowerer) writeFunctionValueParams(b *strings.Builder, typeInfo air.TypeInfo) error {
	for i, param := range typeInfo.Params {
		paramType, err := l.typeName(param)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ", a%d: %s", i, paramType)
	}
	return nil
}

func (l *lowerer) fieldTypeName(field air.FieldInfo) (string, error) {
	if !field.RecursiveNullable {
		return l.typeName(field.Type)
	}
	if field.Type <= 0 || int(field.Type) > len(l.program.Types) {
		return "", fmt.Errorf("recursive nullable field %s has invalid type %d", field.Name, field.Type)
	}
	maybeInfo := l.program.Types[field.Type-1]
	if maybeInfo.Kind != air.TypeMaybe {
		return "", fmt.Errorf("recursive nullable field %s has non-maybe type %s", field.Name, maybeInfo.Name)
	}
	elem, err := l.typeName(maybeInfo.Elem)
	if err != nil {
		return "", err
	}
	return "ard.Maybe(*" + elem + ")", nil
}

func (l *lowerer) lowerFunction(b *strings.Builder, fn air.Function) error {
	fmt.Fprintf(b, "fn %s(ctx: *ard.Context", functionName(fn))
	for _, capture := range fn.Captures {
		captureType, err := l.typeName(capture.Type)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, ", %s: %s", localName(fn, capture.Local), captureType)
	}
	for i, param := range fn.Signature.Params {
		paramType, err := l.typeName(param.Type)
		if err != nil {
			return err
		}
		if param.Mutable && paramType != "void" {
			paramType = "*" + paramType
		}
		fmt.Fprintf(b, ", %s: %s", localName(fn, air.LocalID(i)), paramType)
	}
	ret, err := l.typeName(fn.Signature.Return)
	if err != nil {
		return err
	}
	if ret == "void" {
		b.WriteString(") !void {\n")
	} else {
		fmt.Fprintf(b, ") !%s {\n", ret)
	}
	fl := &functionLowerer{l: l, fn: fn, indent: "    "}
	if !fl.blockUsesContext(fn.Body) {
		b.WriteString("    _ = ctx;\n")
	}
	if err := fl.lowerBlock(b, fn.Body, fn.Signature.Return); err != nil {
		return fmt.Errorf("function %s: %w", fn.Name, err)
	}
	b.WriteString("}\n")
	return nil
}

func (l *lowerer) typeName(id air.TypeID) (string, error) {
	if id == air.NoType {
		return "void", nil
	}
	info := l.program.Types[id-1]
	switch info.Kind {
	case air.TypeVoid:
		return "void", nil
	case air.TypeInt:
		return "i64", nil
	case air.TypeFloat:
		return "f64", nil
	case air.TypeBool:
		return "bool", nil
	case air.TypeStr:
		return "[]const u8", nil
	case air.TypeList:
		elem, err := l.typeName(info.Elem)
		if err != nil {
			return "", err
		}
		return "ard.List(" + elem + ")", nil
	case air.TypeMap:
		key, err := l.typeName(info.Key)
		if err != nil {
			return "", err
		}
		value, err := l.typeName(info.Value)
		if err != nil {
			return "", err
		}
		return "ard.Map(" + key + ", " + value + ")", nil
	case air.TypeMaybe:
		elem, err := l.typeName(info.Elem)
		if err != nil {
			return "", err
		}
		return "ard.Maybe(" + elem + ")", nil
	case air.TypeResult:
		value, err := l.typeName(info.Value)
		if err != nil {
			return "", err
		}
		errType, err := l.typeName(info.Error)
		if err != nil {
			return "", err
		}
		return "ard.Result(" + value + ", " + errType + ")", nil
	case air.TypeFunction:
		return l.functionTypeName(info)
	case air.TypeStruct, air.TypeEnum:
		return typeDeclName(info), nil
	case air.TypeTraitObject:
		if info.Trait >= 0 && int(info.Trait) < len(l.program.Traits) && l.program.Traits[info.Trait].Name == "ToString" {
			return "ard.Stringable", nil
		}
		return "", fmt.Errorf("unsupported Zig trait object type %s", info.Name)
	default:
		return "", fmt.Errorf("unsupported Zig type %s", info.Name)
	}
}

func (l *lowerer) functionTypeName(info air.TypeInfo) (string, error) {
	if len(info.Params) > 8 {
		return "", fmt.Errorf("unsupported Zig function value arity %d; supported arity is 0..8", len(info.Params))
	}
	ret, err := l.typeName(info.Return)
	if err != nil {
		return "", err
	}
	args := make([]string, 0, len(info.Params)+1)
	for _, param := range info.Params {
		arg, err := l.typeName(param)
		if err != nil {
			return "", err
		}
		args = append(args, arg)
	}
	args = append(args, ret)
	return fmt.Sprintf("ard.Fn%d(%s)", len(info.Params), strings.Join(args, ", ")), nil
}

func (l *lowerer) functionTypeInfo(id air.TypeID) (air.TypeInfo, error) {
	if id <= 0 || int(id) > len(l.program.Types) {
		return air.TypeInfo{}, fmt.Errorf("invalid function type id %d", id)
	}
	info := l.program.Types[id-1]
	if info.Kind != air.TypeFunction {
		return air.TypeInfo{}, fmt.Errorf("type %s is not a function type", info.Name)
	}
	return info, nil
}

type functionLowerer struct {
	l           *lowerer
	fn          air.Function
	indent      string
	tempCounter int
}

func (fl *functionLowerer) lowerBlock(b *strings.Builder, block air.Block, returnType air.TypeID) error {
	for _, stmt := range block.Stmts {
		if err := fl.lowerStmt(b, stmt); err != nil {
			return err
		}
	}
	if block.Result != nil {
		if block.Result.Kind == air.ExprIf {
			if handled, err := fl.lowerIfResult(b, *block.Result, returnType); handled || err != nil {
				return err
			}
		}
		if block.Result.Kind == air.ExprMatchMaybe {
			if handled, err := fl.lowerMatchMaybeResult(b, *block.Result, returnType); handled || err != nil {
				return err
			}
		}
		if block.Result.Kind == air.ExprMatchResult {
			if handled, err := fl.lowerMatchResultResult(b, *block.Result, returnType); handled || err != nil {
				return err
			}
		}
		if block.Result.Kind == air.ExprMatchEnum {
			if handled, err := fl.lowerMatchEnumResult(b, *block.Result, returnType); handled || err != nil {
				return err
			}
		}
		if block.Result.Kind == air.ExprMatchInt {
			if handled, err := fl.lowerMatchIntResult(b, *block.Result, returnType); handled || err != nil {
				return err
			}
		}
		expr, err := fl.lowerExpr(*block.Result)
		if err != nil {
			return err
		}
		returnTypeName, err := fl.l.typeName(returnType)
		if err != nil {
			return err
		}
		if returnTypeName == "void" {
			fmt.Fprintf(b, "%s%s;\n", fl.indent, expr)
		} else {
			fmt.Fprintf(b, "%sreturn %s;\n", fl.indent, expr)
		}
		return nil
	}
	returnTypeName, err := fl.l.typeName(returnType)
	if err != nil {
		return err
	}
	if returnTypeName != "void" {
		return fmt.Errorf("non-void function has no result")
	}
	return nil
}

func (fl *functionLowerer) lowerIfResult(b *strings.Builder, expr air.Expr, returnType air.TypeID) (bool, error) {
	if expr.Condition == nil {
		return true, fmt.Errorf("if expression missing condition")
	}
	if len(expr.Then.Stmts) == 0 && expr.Then.Result == nil && len(expr.Else.Stmts) == 0 && expr.Else.Result == nil {
		return false, nil
	}
	condition, err := fl.lowerExpr(*expr.Condition)
	if err != nil {
		return true, err
	}
	fmt.Fprintf(b, "%sif (%s) {\n", fl.indent, condition)
	thenFl := *fl
	thenFl.indent += "    "
	if err := thenFl.lowerBlock(b, expr.Then, returnType); err != nil {
		return true, err
	}
	if len(expr.Else.Stmts) > 0 || expr.Else.Result != nil {
		fmt.Fprintf(b, "%s} else {\n", fl.indent)
		elseFl := *fl
		elseFl.indent += "    "
		if err := elseFl.lowerBlock(b, expr.Else, returnType); err != nil {
			return true, err
		}
	}
	fmt.Fprintf(b, "%s}\n", fl.indent)
	return true, nil
}

func (fl *functionLowerer) lowerMatchIntResult(b *strings.Builder, expr air.Expr, returnType air.TypeID) (bool, error) {
	if expr.Target == nil {
		return true, fmt.Errorf("int match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return true, err
	}
	if len(expr.IntCases) == 0 && len(expr.RangeCases) == 0 && len(expr.CatchAll.Stmts) == 0 && expr.CatchAll.Result == nil {
		return false, nil
	}
	firstBranch := true
	for _, matchCase := range expr.IntCases {
		fl.writeIntMatchBranchHeader(b, firstBranch, "%s == %d", target, matchCase.Value)
		firstBranch = false
		caseFl := *fl
		caseFl.indent += "    "
		if err := caseFl.lowerBlock(b, matchCase.Body, returnType); err != nil {
			return true, err
		}
		fmt.Fprintf(b, "%s}", fl.indent)
	}
	for _, matchCase := range expr.RangeCases {
		fl.writeIntMatchBranchHeader(b, firstBranch, "%s >= %d and %s <= %d", target, matchCase.Start, target, matchCase.End)
		firstBranch = false
		caseFl := *fl
		caseFl.indent += "    "
		if err := caseFl.lowerBlock(b, matchCase.Body, returnType); err != nil {
			return true, err
		}
		fmt.Fprintf(b, "%s}", fl.indent)
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		if firstBranch {
			fmt.Fprintf(b, "%s{\n", fl.indent)
		} else {
			b.WriteString(" else {\n")
		}
		catchAllFl := *fl
		catchAllFl.indent += "    "
		if err := catchAllFl.lowerBlock(b, expr.CatchAll, returnType); err != nil {
			return true, err
		}
		fmt.Fprintf(b, "%s}", fl.indent)
	} else {
		if firstBranch {
			fmt.Fprintf(b, "%sunreachable;\n", fl.indent)
			return true, nil
		}
		b.WriteString(" else {\n")
		fmt.Fprintf(b, "%s    unreachable;\n", fl.indent)
		fmt.Fprintf(b, "%s}", fl.indent)
	}
	b.WriteString("\n")
	return true, nil
}

func (fl *functionLowerer) writeIntMatchBranchHeader(b *strings.Builder, firstBranch bool, condition string, args ...any) {
	if firstBranch {
		fmt.Fprintf(b, "%sif (", fl.indent)
	} else {
		b.WriteString(" else if (")
	}
	fmt.Fprintf(b, condition, args...)
	b.WriteString(") {\n")
}

func (fl *functionLowerer) lowerMatchEnumResult(b *strings.Builder, expr air.Expr, returnType air.TypeID) (bool, error) {
	if expr.Target == nil {
		return true, fmt.Errorf("enum match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return true, err
	}
	if len(expr.EnumCases) == 0 && len(expr.CatchAll.Stmts) == 0 && expr.CatchAll.Result == nil {
		return false, nil
	}
	fmt.Fprintf(b, "%sswitch (%s) {\n", fl.indent, target)
	for _, matchCase := range expr.EnumCases {
		fmt.Fprintf(b, "%s    %d => {\n", fl.indent, matchCase.Discriminant)
		caseFl := *fl
		caseFl.indent += "        "
		if err := caseFl.lowerBlock(b, matchCase.Body, returnType); err != nil {
			return true, err
		}
		fmt.Fprintf(b, "%s    },\n", fl.indent)
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		fmt.Fprintf(b, "%s    else => {\n", fl.indent)
		catchAllFl := *fl
		catchAllFl.indent += "        "
		if err := catchAllFl.lowerBlock(b, expr.CatchAll, returnType); err != nil {
			return true, err
		}
		fmt.Fprintf(b, "%s    },\n", fl.indent)
	} else {
		fmt.Fprintf(b, "%s    else => unreachable,\n", fl.indent)
	}
	fmt.Fprintf(b, "%s}\n", fl.indent)
	return true, nil
}

func (fl *functionLowerer) lowerMatchMaybeResult(b *strings.Builder, expr air.Expr, returnType air.TypeID) (bool, error) {
	if expr.Target == nil {
		return true, fmt.Errorf("maybe match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return true, err
	}
	if len(expr.Some.Stmts) == 0 && expr.Some.Result == nil && len(expr.None.Stmts) == 0 && expr.None.Result == nil {
		return false, nil
	}
	fmt.Fprintf(b, "%sif ((%s).some) |%s| {\n", fl.indent, target, localName(fl.fn, expr.SomeLocal))
	someFl := *fl
	someFl.indent += "    "
	if err := someFl.lowerBlock(b, expr.Some, returnType); err != nil {
		return true, err
	}
	fmt.Fprintf(b, "%s} else {\n", fl.indent)
	noneFl := *fl
	noneFl.indent += "    "
	if err := noneFl.lowerBlock(b, expr.None, returnType); err != nil {
		return true, err
	}
	fmt.Fprintf(b, "%s}\n", fl.indent)
	return true, nil
}

func (fl *functionLowerer) lowerMatchResultResult(b *strings.Builder, expr air.Expr, returnType air.TypeID) (bool, error) {
	if expr.Target == nil {
		return true, fmt.Errorf("result match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return true, err
	}
	if len(expr.Ok.Stmts) == 0 && expr.Ok.Result == nil && len(expr.Err.Stmts) == 0 && expr.Err.Result == nil {
		return false, nil
	}
	temp := fl.nextTemp("result")
	fmt.Fprintf(b, "%sconst %s = %s;\n", fl.indent, temp, target)
	fmt.Fprintf(b, "%sif (%s.ok) {\n", fl.indent, temp)
	okFl := *fl
	okFl.indent += "    "
	fmt.Fprintf(b, "%sconst %s = %s.value;\n", okFl.indent, localName(fl.fn, expr.OkLocal), temp)
	if fl.localNeedsDiscard(expr.OkLocal) {
		fmt.Fprintf(b, "%s_ = %s;\n", okFl.indent, localName(fl.fn, expr.OkLocal))
	}
	if err := okFl.lowerBlock(b, expr.Ok, returnType); err != nil {
		return true, err
	}
	fmt.Fprintf(b, "%s} else {\n", fl.indent)
	errFl := *fl
	errFl.indent += "    "
	fmt.Fprintf(b, "%sconst %s = %s.err;\n", errFl.indent, localName(fl.fn, expr.ErrLocal), temp)
	if fl.localNeedsDiscard(expr.ErrLocal) {
		fmt.Fprintf(b, "%s_ = %s;\n", errFl.indent, localName(fl.fn, expr.ErrLocal))
	}
	if err := errFl.lowerBlock(b, expr.Err, returnType); err != nil {
		return true, err
	}
	fmt.Fprintf(b, "%s}\n", fl.indent)
	return true, nil
}

func (fl *functionLowerer) lowerStmt(b *strings.Builder, stmt air.Stmt) error {
	switch stmt.Kind {
	case air.StmtLet:
		if stmt.Value == nil {
			return fmt.Errorf("let %s has no value", stmt.Name)
		}
		value, err := fl.lowerExpr(*stmt.Value)
		if err != nil {
			return err
		}
		keyword := "const"
		if stmt.Mutable {
			keyword = "var"
		}
		stmtType, err := fl.l.typeName(stmt.Type)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s%s %s: %s = %s;\n", fl.indent, keyword, localName(fl.fn, stmt.Local), stmtType, value)
	case air.StmtAssign:
		if stmt.Value == nil {
			return fmt.Errorf("assign %s has no value", stmt.Name)
		}
		value, err := fl.lowerExpr(*stmt.Value)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s%s = %s;\n", fl.indent, fl.lowerLocalAssignTarget(stmt.Local), value)
	case air.StmtSetField:
		if stmt.Target == nil {
			return fmt.Errorf("field set statement missing target")
		}
		if stmt.Value == nil {
			return fmt.Errorf("field set statement missing value")
		}
		if stmt.Target.Type <= 0 || int(stmt.Target.Type) > len(fl.l.program.Types) {
			return fmt.Errorf("field set target has invalid type %d", stmt.Target.Type)
		}
		targetType := fl.l.program.Types[stmt.Target.Type-1]
		if targetType.Kind != air.TypeStruct {
			return fmt.Errorf("field set target must be struct, got %s", targetType.Name)
		}
		if stmt.Field < 0 || stmt.Field >= len(targetType.Fields) {
			return fmt.Errorf("field index %d out of range for %s", stmt.Field, targetType.Name)
		}
		target, err := fl.lowerPlace(*stmt.Target)
		if err != nil {
			return err
		}
		value, err := fl.lowerExpr(*stmt.Value)
		if err != nil {
			return err
		}
		if targetType.Fields[stmt.Field].RecursiveNullable {
			elem, err := fl.recursiveNullableElemName(targetType.Fields[stmt.Field])
			if err != nil {
				return err
			}
			value = fmt.Sprintf("try ard.boxMaybe(%s, ctx, %s)", elem, value)
		}
		fmt.Fprintf(b, "%s%s.%s = %s;\n", fl.indent, target, sanitizeIdentifier(targetType.Fields[stmt.Field].Name), value)
	case air.StmtExpr:
		if stmt.Expr == nil {
			return fmt.Errorf("expression statement has no expression")
		}
		if stmt.Expr.Kind == air.ExprIf {
			if handled, err := fl.lowerIfResult(b, *stmt.Expr, air.NoType); handled || err != nil {
				return err
			}
		}
		if stmt.Expr.Kind == air.ExprMatchMaybe {
			if handled, err := fl.lowerMatchMaybeResult(b, *stmt.Expr, air.NoType); handled || err != nil {
				return err
			}
		}
		if stmt.Expr.Kind == air.ExprMatchResult {
			if handled, err := fl.lowerMatchResultResult(b, *stmt.Expr, air.NoType); handled || err != nil {
				return err
			}
		}
		if stmt.Expr.Kind == air.ExprMatchEnum {
			if handled, err := fl.lowerMatchEnumResult(b, *stmt.Expr, air.NoType); handled || err != nil {
				return err
			}
		}
		if stmt.Expr.Kind == air.ExprMatchInt {
			if handled, err := fl.lowerMatchIntResult(b, *stmt.Expr, air.NoType); handled || err != nil {
				return err
			}
		}
		expr, err := fl.lowerExpr(*stmt.Expr)
		if err != nil {
			return err
		}
		if stmt.Expr.Kind == air.ExprTryResult {
			fmt.Fprintf(b, "%s_ = %s;\n", fl.indent, expr)
			return nil
		}
		fmt.Fprintf(b, "%s%s;\n", fl.indent, expr)
	case air.StmtWhile:
		if stmt.Condition == nil {
			return fmt.Errorf("while has no condition")
		}
		condition, err := fl.lowerExpr(*stmt.Condition)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%swhile (%s) {\n", fl.indent, condition)
		child := *fl
		child.indent += "    "
		if err := child.lowerBlock(b, stmt.Body, air.NoType); err != nil {
			return err
		}
		fmt.Fprintf(b, "%s}\n", fl.indent)
	case air.StmtBreak:
		fmt.Fprintf(b, "%sbreak;\n", fl.indent)
	default:
		return fmt.Errorf("unsupported statement kind %d", stmt.Kind)
	}
	return nil
}

func (fl *functionLowerer) lowerExpr(expr air.Expr) (string, error) {
	switch expr.Kind {
	case air.ExprConstVoid:
		return "{}", nil
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
		if fl.localIsMutablePointer(expr.Local) {
			return localName(fl.fn, expr.Local) + ".*", nil
		}
		return localName(fl.fn, expr.Local), nil
	case air.ExprFunctionRef:
		return fl.lowerFunctionRef(expr)
	case air.ExprCall:
		return fl.lowerCall(expr)
	case air.ExprCallExtern:
		return fl.lowerExternCall(expr)
	case air.ExprMakeClosure:
		return fl.lowerMakeClosure(expr)
	case air.ExprCallClosure:
		return fl.lowerCallClosure(expr)
	case air.ExprTraitUpcast:
		if expr.Target == nil {
			return "", fmt.Errorf("trait upcast missing target")
		}
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return fl.lowerTraitUpcast(expr, target)
	case air.ExprCallTrait:
		return fl.lowerTraitCall(expr)
	case air.ExprMakeList:
		return fl.lowerMakeList(expr)
	case air.ExprListAt:
		return fl.lowerListAt(expr)
	case air.ExprListPush:
		return fl.lowerListPush(expr)
	case air.ExprListSize:
		return fl.lowerListSize(expr)
	case air.ExprMakeStruct:
		return fl.lowerMakeStruct(expr)
	case air.ExprGetField:
		return fl.lowerGetField(expr)
	case air.ExprMakeMap:
		return fl.lowerMakeMap(expr)
	case air.ExprMakeMaybeSome:
		return fl.lowerMakeMaybeSome(expr)
	case air.ExprMakeMaybeNone:
		return fl.lowerMakeMaybeNone(expr)
	case air.ExprMaybeMap:
		return fl.lowerMaybeMap(expr)
	case air.ExprMaybeAndThen:
		return fl.lowerMaybeAndThen(expr)
	case air.ExprMakeResultOk:
		return fl.lowerMakeResultOk(expr)
	case air.ExprMakeResultErr:
		return fl.lowerMakeResultErr(expr)
	case air.ExprMatchResult:
		return fl.lowerMatchResultExpr(expr)
	case air.ExprTryResult:
		return fl.lowerTryResult(expr)
	case air.ExprResultOr:
		return fl.lowerResultOr(expr)
	case air.ExprResultExpect:
		return fl.lowerResultExpect(expr)
	case air.ExprResultIsOk:
		return fl.lowerResultIsOk(expr)
	case air.ExprResultIsErr:
		return fl.lowerResultIsErr(expr)
	case air.ExprResultMap:
		return fl.lowerResultMap(expr)
	case air.ExprResultMapErr:
		return fl.lowerResultMapErr(expr)
	case air.ExprResultAndThen:
		return fl.lowerResultAndThen(expr)
	case air.ExprMapSize:
		return fl.lowerMapSize(expr)
	case air.ExprMapSet:
		return fl.lowerMapSet(expr)
	case air.ExprMapHas:
		return fl.lowerMapHas(expr)
	case air.ExprMapDrop:
		return fl.lowerMapDrop(expr)
	case air.ExprMapGet:
		return fl.lowerMapGet(expr)
	case air.ExprMapKeyAt:
		return fl.lowerMapKeyAt(expr)
	case air.ExprMapValueAt:
		return fl.lowerMapValueAt(expr)
	case air.ExprEnumVariant:
		return fl.lowerEnumVariant(expr)
	case air.ExprMatchEnum:
		return fl.lowerMatchEnumExpr(expr)
	case air.ExprMatchInt:
		return fl.lowerMatchIntExpr(expr)
	case air.ExprMatchMaybe:
		return fl.lowerMatchMaybeExpr(expr)
	case air.ExprIntAdd, air.ExprIntSub, air.ExprIntMul, air.ExprIntDiv, air.ExprIntMod,
		air.ExprFloatAdd, air.ExprFloatSub, air.ExprFloatMul, air.ExprFloatDiv,
		air.ExprEq, air.ExprNotEq, air.ExprLt, air.ExprLte, air.ExprGt, air.ExprGte,
		air.ExprAnd, air.ExprOr:
		return fl.lowerBinary(expr)
	case air.ExprNot:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return "!" + target, nil
	case air.ExprNeg:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return "-" + target, nil
	case air.ExprFloatToInt:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("@as(i64, @intFromFloat(%s))", target), nil
	case air.ExprStrConcat:
		left, err := fl.lowerExpr(*expr.Left)
		if err != nil {
			return "", err
		}
		right, err := fl.lowerExpr(*expr.Right)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("try ard.concat(ctx, %s, %s)", left, right), nil
	case air.ExprStrAt:
		return fl.lowerStrAt(expr)
	case air.ExprStrSize:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("@as(i64, @intCast(%s.len))", target), nil
	case air.ExprStrIsEmpty:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s.len == 0)", target), nil
	case air.ExprStrContains:
		return fl.lowerStrUnaryCall("std.mem.containsAtLeast(u8, %s, 1, %s)", expr)
	case air.ExprStrReplace:
		return fl.lowerStrBinaryCall("try ard.strReplace(ctx, %s, %s, %s)", expr)
	case air.ExprStrReplaceAll:
		return fl.lowerStrBinaryCall("try ard.strReplaceAll(ctx, %s, %s, %s)", expr)
	case air.ExprStrSplit:
		return fl.lowerStrUnaryCall("try ard.strSplit(ctx, %s, %s)", expr)
	case air.ExprStrStartsWith:
		return fl.lowerStrUnaryCall("std.mem.startsWith(u8, %s, %s)", expr)
	case air.ExprStrEndsWith:
		return fl.lowerStrUnaryCall("std.mem.endsWith(u8, %s, %s)", expr)
	case air.ExprStrTrim:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("std.mem.trim(u8, %s, \" \")", target), nil
	case air.ExprToStr:
		target, err := fl.lowerExpr(*expr.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("try ard.toStr(ctx, %s)", target), nil
	case air.ExprIf:
		return fl.lowerIfExpr(expr)
	default:
		return "", fmt.Errorf("unsupported expression kind %d", expr.Kind)
	}
}

func (fl *functionLowerer) lowerStrAt(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("str at missing target")
	}
	if len(expr.Args) != 1 {
		return "", fmt.Errorf("str at expects 1 arg, got %d", len(expr.Args))
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	index, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	if expr.Type > 0 && int(expr.Type) <= len(fl.l.program.Types) && fl.l.program.Types[expr.Type-1].Kind == air.TypeMaybe {
		return fmt.Sprintf("ard.strAt(%s, %s)", target, index), nil
	}
	return fmt.Sprintf("ard.strAtByte(%s, %s)", target, index), nil
}

func (fl *functionLowerer) lowerStrUnaryCall(format string, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("str method missing target")
	}
	if len(expr.Args) != 1 {
		return "", fmt.Errorf("str method expects 1 arg, got %d", len(expr.Args))
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	arg, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(format, target, arg), nil
}

func (fl *functionLowerer) lowerStrBinaryCall(format string, expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("str method missing target")
	}
	if len(expr.Args) != 2 {
		return "", fmt.Errorf("str method expects 2 args, got %d", len(expr.Args))
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	first, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	second, err := fl.lowerExpr(expr.Args[1])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(format, target, first, second), nil
}

func (fl *functionLowerer) lowerMakeList(expr air.Expr) (string, error) {
	if expr.Type <= 0 || int(expr.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("list literal has invalid type %d", expr.Type)
	}
	listType := fl.l.program.Types[expr.Type-1]
	if listType.Kind != air.TypeList {
		return "", fmt.Errorf("list literal has non-list type %s", listType.Name)
	}
	elemType, err := fl.l.typeName(listType.Elem)
	if err != nil {
		return "", err
	}
	items := make([]string, 0, len(expr.Args))
	for _, arg := range expr.Args {
		item, err := fl.lowerExpr(arg)
		if err != nil {
			return "", err
		}
		items = append(items, item)
	}
	return fmt.Sprintf("try ard.List(%s).init(ctx.allocator, &[_]%s{%s})", elemType, elemType, strings.Join(items, ", ")), nil
}

func (fl *functionLowerer) lowerListAt(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("list at expects target and index")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	index, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.at(@intCast(%s))", target, index), nil
}

func (fl *functionLowerer) lowerListSize(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("list size missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("@as(i64, @intCast(%s.size()))", target), nil
}

func (fl *functionLowerer) lowerListPush(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("list push expects target and value")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	value, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("try %s.push(%s)", target, value), nil
}

func (fl *functionLowerer) lowerMakeStruct(expr air.Expr) (string, error) {
	if expr.Type <= 0 || int(expr.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("struct literal has invalid type %d", expr.Type)
	}
	structType := fl.l.program.Types[expr.Type-1]
	if structType.Kind != air.TypeStruct {
		return "", fmt.Errorf("struct literal has non-struct type %s", structType.Name)
	}
	fields := make([]string, 0, len(expr.Fields))
	for _, field := range expr.Fields {
		value, err := fl.lowerExpr(field.Value)
		if err != nil {
			return "", err
		}
		if field.Index >= 0 && field.Index < len(structType.Fields) && structType.Fields[field.Index].RecursiveNullable {
			elem, err := fl.recursiveNullableElemName(structType.Fields[field.Index])
			if err != nil {
				return "", err
			}
			value = fmt.Sprintf("try ard.boxMaybe(%s, ctx, %s)", elem, value)
		}
		fields = append(fields, fmt.Sprintf(".%s = %s", sanitizeIdentifier(field.Name), value))
	}
	return fmt.Sprintf("%s{ %s }", typeDeclName(structType), strings.Join(fields, ", ")), nil
}

func (fl *functionLowerer) lowerGetField(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("field access missing target")
	}
	if expr.Target.Type <= 0 || int(expr.Target.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("field access target has invalid type %d", expr.Target.Type)
	}
	targetType := fl.l.program.Types[expr.Target.Type-1]
	if targetType.Kind != air.TypeStruct {
		return "", fmt.Errorf("field access on non-struct type %s", targetType.Name)
	}
	if expr.Field < 0 || expr.Field >= len(targetType.Fields) {
		return "", fmt.Errorf("field index %d out of range for %s", expr.Field, targetType.Name)
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	field := targetType.Fields[expr.Field]
	access := fmt.Sprintf("%s.%s", target, sanitizeIdentifier(field.Name))
	if field.RecursiveNullable {
		elem, err := fl.recursiveNullableElemName(field)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ard.unboxMaybe(%s, %s)", elem, access), nil
	}
	return access, nil
}

func (fl *functionLowerer) lowerPlace(expr air.Expr) (string, error) {
	switch expr.Kind {
	case air.ExprLoadLocal:
		return localName(fl.fn, expr.Local), nil
	case air.ExprGetField:
		if expr.Target == nil {
			return "", fmt.Errorf("field place missing target")
		}
		if expr.Target.Type <= 0 || int(expr.Target.Type) > len(fl.l.program.Types) {
			return "", fmt.Errorf("field place target has invalid type %d", expr.Target.Type)
		}
		targetType := fl.l.program.Types[expr.Target.Type-1]
		if targetType.Kind != air.TypeStruct {
			return "", fmt.Errorf("field place on non-struct type %s", targetType.Name)
		}
		if expr.Field < 0 || expr.Field >= len(targetType.Fields) {
			return "", fmt.Errorf("field index %d out of range for %s", expr.Field, targetType.Name)
		}
		target, err := fl.lowerPlace(*expr.Target)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s.%s", target, sanitizeIdentifier(targetType.Fields[expr.Field].Name)), nil
	default:
		return "", fmt.Errorf("expression kind %d is not addressable", expr.Kind)
	}
}

func (fl *functionLowerer) lowerLocalAssignTarget(local air.LocalID) string {
	if fl.localIsMutablePointer(local) {
		return localName(fl.fn, local) + ".*"
	}
	return localName(fl.fn, local)
}

func (fl *functionLowerer) localIsMutablePointer(local air.LocalID) bool {
	return int(local) >= 0 && int(local) < len(fl.fn.Signature.Params) && fl.fn.Signature.Params[local].Mutable
}

func (fl *functionLowerer) localNeedsDiscard(local air.LocalID) bool {
	return int(local) < 0 || int(local) >= len(fl.fn.Locals) || fl.fn.Locals[local].Name == "" || fl.fn.Locals[local].Name == "_"
}

func (fl *functionLowerer) recursiveNullableElemName(field air.FieldInfo) (string, error) {
	if field.Type <= 0 || int(field.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("recursive nullable field %s has invalid type %d", field.Name, field.Type)
	}
	maybeInfo := fl.l.program.Types[field.Type-1]
	if maybeInfo.Kind != air.TypeMaybe {
		return "", fmt.Errorf("recursive nullable field %s has non-maybe type %s", field.Name, maybeInfo.Name)
	}
	return fl.l.typeName(maybeInfo.Elem)
}

func (fl *functionLowerer) lowerMakeMap(expr air.Expr) (string, error) {
	if expr.Type <= 0 || int(expr.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("map literal has invalid type %d", expr.Type)
	}
	mapType := fl.l.program.Types[expr.Type-1]
	if mapType.Kind != air.TypeMap {
		return "", fmt.Errorf("map literal has non-map type %s", mapType.Name)
	}
	keyType, err := fl.l.typeName(mapType.Key)
	if err != nil {
		return "", err
	}
	valueType, err := fl.l.typeName(mapType.Value)
	if err != nil {
		return "", err
	}
	entries := make([]string, 0, len(expr.Entries))
	for _, entry := range expr.Entries {
		key, err := fl.lowerExpr(entry.Key)
		if err != nil {
			return "", err
		}
		value, err := fl.lowerExpr(entry.Value)
		if err != nil {
			return "", err
		}
		entries = append(entries, fmt.Sprintf(".{ .key = %s, .value = %s }", key, value))
	}
	return fmt.Sprintf("try ard.Map(%s, %s).init(ctx.allocator, &[_]ard.Map(%s, %s).Entry{%s})", keyType, valueType, keyType, valueType, strings.Join(entries, ", ")), nil
}

func (fl *functionLowerer) lowerMakeMaybeSome(expr air.Expr) (string, error) {
	if expr.Type <= 0 || int(expr.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("maybe some has invalid type %d", expr.Type)
	}
	maybeType := fl.l.program.Types[expr.Type-1]
	if maybeType.Kind != air.TypeMaybe {
		return "", fmt.Errorf("maybe some has non-maybe type %s", maybeType.Name)
	}
	if expr.Target == nil {
		return "", fmt.Errorf("maybe some missing target")
	}
	elemType, err := fl.l.typeName(maybeType.Elem)
	if err != nil {
		return "", err
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ard.Maybe(%s){ .some = %s }", elemType, target), nil
}

func (fl *functionLowerer) lowerMakeMaybeNone(expr air.Expr) (string, error) {
	if expr.Type <= 0 || int(expr.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("maybe none has invalid type %d", expr.Type)
	}
	maybeType := fl.l.program.Types[expr.Type-1]
	if maybeType.Kind != air.TypeMaybe {
		return "", fmt.Errorf("maybe none has non-maybe type %s", maybeType.Name)
	}
	elemType, err := fl.l.typeName(maybeType.Elem)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ard.Maybe(%s){ .some = null }", elemType), nil
}

func (fl *functionLowerer) lowerMaybeMap(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("maybe map expects target and callback")
	}
	maybeType, elemType, err := fl.maybeTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, callback, err := fl.lowerMaybeCallbackOperands(expr)
	if err != nil {
		return "", err
	}
	targetTemp := fl.nextTemp("maybe")
	callbackTemp := fl.nextTemp("callback")
	mapped := fmt.Sprintf("try %s.invoke(%s.some.?)", callbackTemp, targetTemp)
	if elemType == "void" {
		mapped = "{}"
	}
	return fmt.Sprintf("blk: {\nconst %s = %s;\nconst %s = %s;\nif (%s.some != null) break :blk %s{ .some = %s };\nbreak :blk %s{ .some = null };\n}", targetTemp, target, callbackTemp, callback, targetTemp, maybeType, mapped, maybeType), nil
}

func (fl *functionLowerer) lowerMaybeAndThen(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("maybe and_then expects target and callback")
	}
	maybeType, _, err := fl.maybeTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, callback, err := fl.lowerMaybeCallbackOperands(expr)
	if err != nil {
		return "", err
	}
	targetTemp := fl.nextTemp("maybe")
	callbackTemp := fl.nextTemp("callback")
	return fmt.Sprintf("blk: {\nconst %s = %s;\nconst %s = %s;\nif (%s.some != null) break :blk try %s.invoke(%s.some.?);\nbreak :blk %s{ .some = null };\n}", targetTemp, target, callbackTemp, callback, targetTemp, callbackTemp, targetTemp, maybeType), nil
}

func (fl *functionLowerer) lowerMaybeCallbackOperands(expr air.Expr) (target string, callback string, err error) {
	target, err = fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", "", err
	}
	callback, err = fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", "", err
	}
	return target, callback, nil
}

func (fl *functionLowerer) maybeTypeNames(id air.TypeID) (maybeType, elemType string, err error) {
	if id <= 0 || int(id) > len(fl.l.program.Types) {
		err = fmt.Errorf("invalid maybe type id %d", id)
		return
	}
	info := fl.l.program.Types[id-1]
	if info.Kind != air.TypeMaybe {
		err = fmt.Errorf("maybe expression with non-maybe type %s", info.Name)
		return
	}
	maybeType, err = fl.l.typeName(id)
	if err != nil {
		return
	}
	elemType, err = fl.l.typeName(info.Elem)
	return
}

func (fl *functionLowerer) lowerMakeResultOk(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("result ok missing target")
	}
	resultType, valueType, _, err := fl.resultTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	if valueType == "void" {
		target = "{}"
	}
	return fmt.Sprintf("%s{ .value = %s, .err = undefined, .ok = true }", resultType, target), nil
}

func (fl *functionLowerer) lowerMakeResultErr(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("result err missing target")
	}
	resultType, _, errType, err := fl.resultTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	if errType == "void" {
		target = "{}"
	}
	return fmt.Sprintf("%s{ .value = undefined, .err = %s, .ok = false }", resultType, target), nil
}

func (fl *functionLowerer) lowerResultOr(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("result or expects target and one arg")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	defaultValue, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	temp := fl.nextTemp("result")
	return fmt.Sprintf("blk: {\nconst %s = %s;\nif (%s.ok) break :blk %s.value;\nbreak :blk %s;\n}", temp, target, temp, temp, defaultValue), nil
}

func (fl *functionLowerer) lowerResultExpect(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("result expect expects target and message")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	message, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	temp := fl.nextTemp("result")
	return fmt.Sprintf("blk: {\nconst %s = %s;\nif (%s.ok) break :blk %s.value;\n@panic(%s);\n}", temp, target, temp, temp, message), nil
}

func (fl *functionLowerer) lowerResultIsOk(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("result is_ok missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("((%s).ok)", target), nil
}

func (fl *functionLowerer) lowerResultIsErr(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("result is_err missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("(!(%s).ok)", target), nil
}

func (fl *functionLowerer) lowerResultMap(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("result map expects target and callback")
	}
	resultType, valueType, _, err := fl.resultTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, callback, err := fl.lowerResultCallbackOperands(expr)
	if err != nil {
		return "", err
	}
	targetTemp := fl.nextTemp("result")
	callbackTemp := fl.nextTemp("callback")
	mapped := fmt.Sprintf("try %s.invoke(%s.value)", callbackTemp, targetTemp)
	if valueType == "void" {
		mapped = "{}"
	}
	return fmt.Sprintf("blk: {\nconst %s = %s;\nconst %s = %s;\nif (%s.ok) break :blk %s{ .value = %s, .err = undefined, .ok = true };\nbreak :blk %s{ .value = undefined, .err = %s.err, .ok = false };\n}", targetTemp, target, callbackTemp, callback, targetTemp, resultType, mapped, resultType, targetTemp), nil
}

func (fl *functionLowerer) lowerResultMapErr(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("result map_err expects target and callback")
	}
	resultType, _, errType, err := fl.resultTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, callback, err := fl.lowerResultCallbackOperands(expr)
	if err != nil {
		return "", err
	}
	targetTemp := fl.nextTemp("result")
	callbackTemp := fl.nextTemp("callback")
	mappedErr := fmt.Sprintf("try %s.invoke(%s.err)", callbackTemp, targetTemp)
	if errType == "void" {
		mappedErr = "{}"
	}
	return fmt.Sprintf("blk: {\nconst %s = %s;\nconst %s = %s;\nif (%s.ok) break :blk %s{ .value = %s.value, .err = undefined, .ok = true };\nbreak :blk %s{ .value = undefined, .err = %s, .ok = false };\n}", targetTemp, target, callbackTemp, callback, targetTemp, resultType, targetTemp, resultType, mappedErr), nil
}

func (fl *functionLowerer) lowerResultAndThen(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("result and_then expects target and callback")
	}
	resultType, _, _, err := fl.resultTypeNames(expr.Type)
	if err != nil {
		return "", err
	}
	target, callback, err := fl.lowerResultCallbackOperands(expr)
	if err != nil {
		return "", err
	}
	targetTemp := fl.nextTemp("result")
	callbackTemp := fl.nextTemp("callback")
	return fmt.Sprintf("blk: {\nconst %s = %s;\nconst %s = %s;\nif (%s.ok) break :blk try %s.invoke(%s.value);\nbreak :blk %s{ .value = undefined, .err = %s.err, .ok = false };\n}", targetTemp, target, callbackTemp, callback, targetTemp, callbackTemp, targetTemp, resultType, targetTemp), nil
}

func (fl *functionLowerer) lowerResultCallbackOperands(expr air.Expr) (target string, callback string, err error) {
	target, err = fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", "", err
	}
	callback, err = fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", "", err
	}
	return target, callback, nil
}

func (fl *functionLowerer) lowerMatchResultExpr(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("result match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	okResult, err := blockResultExpr(fl, expr.Ok)
	if err != nil {
		return "", err
	}
	errResult, err := blockResultExpr(fl, expr.Err)
	if err != nil {
		return "", err
	}
	temp := fl.nextTemp("result")
	okLocal := localName(fl.fn, expr.OkLocal)
	errLocal := localName(fl.fn, expr.ErrLocal)
	okDiscard := ""
	if fl.localNeedsDiscard(expr.OkLocal) {
		okDiscard = fmt.Sprintf("_ = %s;\n", okLocal)
	}
	errDiscard := ""
	if fl.localNeedsDiscard(expr.ErrLocal) {
		errDiscard = fmt.Sprintf("_ = %s;\n", errLocal)
	}
	return fmt.Sprintf("blk: {\nconst %s = %s;\nif (%s.ok) {\nconst %s = %s.value;\n%sbreak :blk %s;\n} else {\nconst %s = %s.err;\n%sbreak :blk %s;\n}\n}", temp, target, temp, okLocal, temp, okDiscard, okResult, errLocal, temp, errDiscard, errResult), nil
}

func (fl *functionLowerer) lowerTryResult(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("try result missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	temp := fl.nextTemp("result")
	var b strings.Builder
	fmt.Fprintf(&b, "blk: {\nconst %s = %s;\nif (%s.ok) break :blk %s.value;\n", temp, target, temp, temp)
	if expr.HasCatch {
		catchExpr, err := blockResultExpr(fl, expr.Catch)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "const %s = %s.err;\nbreak :blk %s;\n", localName(fl.fn, expr.CatchLocal), temp, catchExpr)
	} else {
		returnType, err := fl.l.typeName(fl.fn.Signature.Return)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "return %s{ .value = undefined, .err = %s.err, .ok = false };\n", returnType, temp)
	}
	b.WriteString("}")
	return b.String(), nil
}

func (fl *functionLowerer) resultTypeNames(id air.TypeID) (resultType, valueType, errType string, err error) {
	if id <= 0 || int(id) > len(fl.l.program.Types) {
		err = fmt.Errorf("invalid result type id %d", id)
		return
	}
	info := fl.l.program.Types[id-1]
	if info.Kind != air.TypeResult {
		err = fmt.Errorf("result expression with non-result type %s", info.Name)
		return
	}
	resultType, err = fl.l.typeName(id)
	if err != nil {
		return
	}
	valueType, err = fl.l.typeName(info.Value)
	if err != nil {
		return
	}
	errType, err = fl.l.typeName(info.Error)
	return
}

func (fl *functionLowerer) lowerMapSize(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("map size missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("@as(i64, @intCast(%s.size()))", target), nil
}

func (fl *functionLowerer) lowerMapSet(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 2 {
		return "", fmt.Errorf("map set expects target and two args")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	key, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	value, err := fl.lowerExpr(expr.Args[1])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("try %s.set(%s, %s)", target, key, value), nil
}

func (fl *functionLowerer) lowerMapHas(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("map has expects target and one arg")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	key, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.has(%s)", target, key), nil
}

func (fl *functionLowerer) lowerMapDrop(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("map drop expects target and one arg")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	key, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.drop(%s)", target, key), nil
}

func (fl *functionLowerer) lowerMapGet(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("map get expects target and one arg")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	key, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.get(%s)", target, key), nil
}

func (fl *functionLowerer) lowerMapKeyAt(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("map key_at expects target and index")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	index, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.keyAt(@intCast(%s))", target, index), nil
}

func (fl *functionLowerer) lowerMapValueAt(expr air.Expr) (string, error) {
	if expr.Target == nil || len(expr.Args) != 1 {
		return "", fmt.Errorf("map value_at expects target and index")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	index, err := fl.lowerExpr(expr.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.valueAt(@intCast(%s))", target, index), nil
}

func (fl *functionLowerer) lowerEnumVariant(expr air.Expr) (string, error) {
	if expr.Type <= 0 || int(expr.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("invalid enum type id %d", expr.Type)
	}
	typ := fl.l.program.Types[expr.Type-1]
	if typ.Kind != air.TypeEnum {
		return "", fmt.Errorf("enum variant with non-enum type %s", typ.Name)
	}
	if expr.Variant < 0 || expr.Variant >= len(typ.Variants) {
		return "", fmt.Errorf("enum variant index %d out of range for %s", expr.Variant, typ.Name)
	}
	return enumVariantName(typ, typ.Variants[expr.Variant]), nil
}

func (fl *functionLowerer) lowerMatchIntExpr(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("int match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("blk: {\n")
	firstBranch := true
	for _, matchCase := range expr.IntCases {
		result, err := blockResultExpr(fl, matchCase.Body)
		if err != nil {
			return "", err
		}
		writeIntMatchExprBranchHeader(&b, firstBranch, "%s == %d", target, matchCase.Value)
		firstBranch = false
		fmt.Fprintf(&b, "break :blk %s;\n", result)
		b.WriteString("}")
	}
	for _, matchCase := range expr.RangeCases {
		result, err := blockResultExpr(fl, matchCase.Body)
		if err != nil {
			return "", err
		}
		writeIntMatchExprBranchHeader(&b, firstBranch, "%s >= %d and %s <= %d", target, matchCase.Start, target, matchCase.End)
		firstBranch = false
		fmt.Fprintf(&b, "break :blk %s;\n", result)
		b.WriteString("}")
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		result, err := blockResultExpr(fl, expr.CatchAll)
		if err != nil {
			return "", err
		}
		if firstBranch {
			fmt.Fprintf(&b, "break :blk %s;\n", result)
		} else {
			fmt.Fprintf(&b, " else {\nbreak :blk %s;\n}", result)
		}
	} else {
		if firstBranch {
			b.WriteString("unreachable;\n")
		} else {
			b.WriteString(" else {\nunreachable;\n}")
		}
	}
	b.WriteString("\n")
	b.WriteString("}")
	return b.String(), nil
}

func writeIntMatchExprBranchHeader(b *strings.Builder, firstBranch bool, condition string, args ...any) {
	if firstBranch {
		b.WriteString("if (")
	} else {
		b.WriteString(" else if (")
	}
	fmt.Fprintf(b, condition, args...)
	b.WriteString(") {\n")
}

func (fl *functionLowerer) lowerMatchEnumExpr(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("enum match missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("blk: {\n")
	b.WriteString("switch (")
	b.WriteString(target)
	b.WriteString(") {\n")
	for _, matchCase := range expr.EnumCases {
		result, err := blockResultExpr(fl, matchCase.Body)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%d => break :blk %s,\n", matchCase.Discriminant, result)
	}
	if len(expr.CatchAll.Stmts) > 0 || expr.CatchAll.Result != nil {
		result, err := blockResultExpr(fl, expr.CatchAll)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "else => break :blk %s,\n", result)
	} else {
		b.WriteString("else => unreachable,\n")
	}
	b.WriteString("}\n")
	b.WriteString("}")
	return b.String(), nil
}

func (fl *functionLowerer) lowerMatchMaybeExpr(expr air.Expr) (string, error) {
	return "", fmt.Errorf("unsupported expression-form maybe match")
}

func (fl *functionLowerer) lowerFunctionRef(expr air.Expr) (string, error) {
	if int(expr.Function) < 0 || int(expr.Function) >= len(fl.l.program.Functions) {
		return "", fmt.Errorf("function ref %d out of range", expr.Function)
	}
	fnType, err := fl.l.typeName(expr.Type)
	if err != nil {
		return "", err
	}
	adapter := fl.l.ensureFunctionAdapter(expr.Function, expr.Type)
	return fmt.Sprintf("%s{ .ctx = ctx, .env = null, .call = %s }", fnType, adapter), nil
}

func (fl *functionLowerer) lowerMakeClosure(expr air.Expr) (string, error) {
	if int(expr.Function) < 0 || int(expr.Function) >= len(fl.l.program.Functions) {
		return "", fmt.Errorf("closure function %d out of range", expr.Function)
	}
	fn := fl.l.program.Functions[expr.Function]
	fnType, err := fl.l.typeName(expr.Type)
	if err != nil {
		return "", err
	}
	adapter := fl.l.ensureClosureAdapter(expr.Function)
	if len(fn.Captures) == 0 {
		return fmt.Sprintf("%s{ .ctx = ctx, .env = null, .call = %s }", fnType, adapter), nil
	}
	envName := closureEnvName(fn)
	values := make([]string, 0, len(fn.Captures))
	for i, capture := range fn.Captures {
		if i >= len(expr.CaptureLocals) {
			return "", fmt.Errorf("closure %s expects capture %d", fn.Name, i)
		}
		captured, err := fl.lowerExpr(air.Expr{Kind: air.ExprLoadLocal, Type: capture.Type, Local: expr.CaptureLocals[i]})
		if err != nil {
			return "", err
		}
		values = append(values, fmt.Sprintf(".c%d = %s", i, captured))
	}
	return fmt.Sprintf("blk: {\nconst env = try ctx.allocator.create(%s);\nenv.* = .{ %s };\nbreak :blk %s{ .ctx = ctx, .env = env, .call = %s };\n}", envName, strings.Join(values, ", "), fnType, adapter), nil
}

func (fl *functionLowerer) lowerCallClosure(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("call closure missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	args := make([]string, 0, len(expr.Args))
	for _, arg := range expr.Args {
		lowered, err := fl.lowerExpr(arg)
		if err != nil {
			return "", err
		}
		args = append(args, lowered)
	}
	return fmt.Sprintf("try %s.invoke(%s)", target, strings.Join(args, ", ")), nil
}

func (fl *functionLowerer) lowerCall(expr air.Expr) (string, error) {
	if int(expr.Function) < 0 || int(expr.Function) >= len(fl.l.program.Functions) {
		return "", fmt.Errorf("function %d out of range", expr.Function)
	}
	fn := fl.l.program.Functions[expr.Function]
	args := []string{"ctx"}
	for i, arg := range expr.Args {
		var lowered string
		var err error
		if i < len(fn.Signature.Params) && fn.Signature.Params[i].Mutable {
			lowered, err = fl.lowerMutableArg(arg)
		} else {
			lowered, err = fl.lowerExpr(arg)
		}
		if err != nil {
			return "", err
		}
		args = append(args, lowered)
	}
	return fmt.Sprintf("try %s(%s)", functionName(fn), strings.Join(args, ", ")), nil
}

func (fl *functionLowerer) lowerMutableArg(expr air.Expr) (string, error) {
	if expr.Kind == air.ExprLoadLocal && fl.localIsMutablePointer(expr.Local) {
		return localName(fl.fn, expr.Local), nil
	}
	place, err := fl.lowerPlace(expr)
	if err != nil {
		return "", err
	}
	return "&" + place, nil
}

func (fl *functionLowerer) lowerExternCall(expr air.Expr) (string, error) {
	if int(expr.Extern) < 0 || int(expr.Extern) >= len(fl.l.program.Externs) {
		return "", fmt.Errorf("extern %d out of range", expr.Extern)
	}
	ext := fl.l.program.Externs[expr.Extern]
	binding := externBinding(ext)
	args := make([]string, 0, len(expr.Args))
	for _, arg := range expr.Args {
		lowered, err := fl.lowerExpr(arg)
		if err != nil {
			return "", err
		}
		args = append(args, lowered)
	}
	switch binding {
	case "print", "Print":
		if len(args) != 1 {
			return "", fmt.Errorf("print extern expects 1 arg, got %d", len(args))
		}
		return fmt.Sprintf("try ard.print(ctx, %s)", args[0]), nil
	case "FloatFromInt":
		if len(args) != 1 {
			return "", fmt.Errorf("FloatFromInt extern expects 1 arg, got %d", len(args))
		}
		return fmt.Sprintf("@as(f64, @floatFromInt(%s))", args[0]), nil
	case "FloatFromStr":
		if len(args) != 1 {
			return "", fmt.Errorf("FloatFromStr extern expects 1 arg, got %d", len(args))
		}
		return fmt.Sprintf("ard.floatFromStr(%s)", args[0]), nil
	case "FloatFloor":
		if len(args) != 1 {
			return "", fmt.Errorf("FloatFloor extern expects 1 arg, got %d", len(args))
		}
		return fmt.Sprintf("@floor(%s)", args[0]), nil
	case "IntFromStr":
		if len(args) != 1 {
			return "", fmt.Errorf("IntFromStr extern expects 1 arg, got %d", len(args))
		}
		return fmt.Sprintf("ard.intFromStr(%s)", args[0]), nil
	default:
		return "", fmt.Errorf("unsupported zig extern binding %q", binding)
	}
}

func (fl *functionLowerer) lowerTraitUpcast(expr air.Expr, target string) (string, error) {
	if expr.Trait < 0 || int(expr.Trait) >= len(fl.l.program.Traits) {
		return "", fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := fl.l.program.Traits[expr.Trait]
	if trait.Name != "ToString" {
		return "", fmt.Errorf("unsupported zig trait upcast to %s", trait.Name)
	}
	if expr.Target == nil || expr.Target.Type <= 0 || int(expr.Target.Type) > len(fl.l.program.Types) {
		return "", fmt.Errorf("trait upcast target has invalid type %d", expr.Target.Type)
	}
	targetType := fl.l.program.Types[expr.Target.Type-1]
	switch targetType.Kind {
	case air.TypeStr:
		return fmt.Sprintf("ard.Stringable{ .str = %s }", target), nil
	case air.TypeInt:
		return fmt.Sprintf("ard.Stringable{ .int = %s }", target), nil
	case air.TypeFloat:
		return fmt.Sprintf("ard.Stringable{ .float = %s }", target), nil
	case air.TypeBool:
		return fmt.Sprintf("ard.Stringable{ .bool = %s }", target), nil
	default:
		return "", fmt.Errorf("unsupported ToString upcast from %s", targetType.Name)
	}
}

func (fl *functionLowerer) lowerTraitCall(expr air.Expr) (string, error) {
	if expr.Target == nil {
		return "", fmt.Errorf("trait call missing target")
	}
	target, err := fl.lowerExpr(*expr.Target)
	if err != nil {
		return "", err
	}
	if expr.Trait < 0 || int(expr.Trait) >= len(fl.l.program.Traits) {
		return "", fmt.Errorf("invalid trait id %d", expr.Trait)
	}
	trait := fl.l.program.Traits[expr.Trait]
	if expr.Method < 0 || expr.Method >= len(trait.Methods) {
		return "", fmt.Errorf("invalid trait method %d for %s", expr.Method, trait.Name)
	}
	method := trait.Methods[expr.Method]
	if trait.Name == "ToString" && method.Name == "to_str" {
		if expr.Target.Type > 0 && int(expr.Target.Type) <= len(fl.l.program.Types) {
			targetType := fl.l.program.Types[expr.Target.Type-1]
			if targetType.Kind == air.TypeTraitObject {
				return fmt.Sprintf("try ard.stringableToStr(ctx, %s)", target), nil
			}
		}
		return fmt.Sprintf("try ard.toStr(ctx, %s)", target), nil
	}
	return "", fmt.Errorf("unsupported zig trait call %s.%s", trait.Name, method.Name)
}

func (fl *functionLowerer) lowerBinary(expr air.Expr) (string, error) {
	left, err := fl.lowerExpr(*expr.Left)
	if err != nil {
		return "", err
	}
	right, err := fl.lowerExpr(*expr.Right)
	if err != nil {
		return "", err
	}
	op := map[air.ExprKind]string{
		air.ExprIntAdd: "+", air.ExprIntSub: "-", air.ExprIntMul: "*", air.ExprIntDiv: "@divTrunc", air.ExprIntMod: "@mod",
		air.ExprFloatAdd: "+", air.ExprFloatSub: "-", air.ExprFloatMul: "*", air.ExprFloatDiv: "/",
		air.ExprEq: "==", air.ExprNotEq: "!=", air.ExprLt: "<", air.ExprLte: "<=", air.ExprGt: ">", air.ExprGte: ">=",
		air.ExprAnd: "and", air.ExprOr: "or",
	}[expr.Kind]
	switch expr.Kind {
	case air.ExprIntDiv, air.ExprIntMod:
		return fmt.Sprintf("%s(%s, %s)", op, left, right), nil
	default:
		return fmt.Sprintf("(%s %s %s)", left, op, right), nil
	}
}

func (fl *functionLowerer) lowerIfExpr(expr air.Expr) (string, error) {
	condition, err := fl.lowerExpr(*expr.Condition)
	if err != nil {
		return "", err
	}
	thenExpr, err := blockResultExpr(fl, expr.Then)
	if err != nil {
		return "", err
	}
	elseExpr, err := blockResultExpr(fl, expr.Else)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("if (%s) %s else %s", condition, thenExpr, elseExpr), nil
}

func blockResultExpr(fl *functionLowerer, block air.Block) (string, error) {
	if len(block.Stmts) != 0 || block.Result == nil {
		return "", fmt.Errorf("unsupported non-expression if branch")
	}
	return fl.lowerExpr(*block.Result)
}

func (fl *functionLowerer) blockUsesContext(block air.Block) bool {
	for _, stmt := range block.Stmts {
		if stmt.Value != nil && fl.exprUsesContext(*stmt.Value) {
			return true
		}
		if stmt.Expr != nil && fl.exprUsesContext(*stmt.Expr) {
			return true
		}
		if stmt.Target != nil && fl.exprUsesContext(*stmt.Target) {
			return true
		}
		if stmt.Condition != nil && fl.exprUsesContext(*stmt.Condition) {
			return true
		}
		if fl.blockUsesContext(stmt.Body) {
			return true
		}
	}
	return block.Result != nil && fl.exprUsesContext(*block.Result)
}

func (fl *functionLowerer) exprUsesContext(expr air.Expr) bool {
	switch expr.Kind {
	case air.ExprCall, air.ExprCallExtern, air.ExprCallTrait, air.ExprFunctionRef, air.ExprMakeClosure,
		air.ExprMakeList, air.ExprMakeMap, air.ExprStrConcat, air.ExprStrSplit, air.ExprStrReplace,
		air.ExprStrReplaceAll, air.ExprToStr:
		return true
	}
	for _, arg := range expr.Args {
		if fl.exprUsesContext(arg) {
			return true
		}
	}
	for _, entry := range expr.Entries {
		if fl.exprUsesContext(entry.Key) || fl.exprUsesContext(entry.Value) {
			return true
		}
	}
	for _, field := range expr.Fields {
		if fl.exprUsesContext(field.Value) {
			return true
		}
	}
	if expr.Target != nil && fl.exprUsesContext(*expr.Target) {
		return true
	}
	if expr.Left != nil && fl.exprUsesContext(*expr.Left) {
		return true
	}
	if expr.Right != nil && fl.exprUsesContext(*expr.Right) {
		return true
	}
	if expr.Condition != nil && fl.exprUsesContext(*expr.Condition) {
		return true
	}
	return fl.blockUsesContext(expr.Body) ||
		fl.blockUsesContext(expr.Then) ||
		fl.blockUsesContext(expr.Else) ||
		fl.blockUsesContext(expr.CatchAll) ||
		fl.blockUsesContext(expr.Some) ||
		fl.blockUsesContext(expr.None) ||
		fl.blockUsesContext(expr.Ok) ||
		fl.blockUsesContext(expr.Err) ||
		fl.blockUsesContext(expr.Catch)
}

func functionName(fn air.Function) string {
	return fmt.Sprintf("ard_fn_%d_%s", fn.ID, sanitizeIdentifier(fn.Name))
}

func typeDeclName(typ air.TypeInfo) string {
	return fmt.Sprintf("ArdType_%d_%s", typ.ID, sanitizeIdentifier(typ.Name))
}

func enumVariantName(typ air.TypeInfo, variant air.VariantInfo) string {
	return fmt.Sprintf("%s_%s", typeDeclName(typ), sanitizeIdentifier(variant.Name))
}

func (l *lowerer) ensureFunctionAdapter(fn air.FunctionID, typ air.TypeID) string {
	name := functionAdapterName(fn, typ)
	l.functionAdapters[name] = functionAdapter{Function: fn, Type: typ}
	return name
}

func (l *lowerer) ensureClosureAdapter(fn air.FunctionID) string {
	l.closureAdapters[fn] = closureAdapter{Function: fn}
	return closureAdapterName(fn)
}

func functionAdapterName(fn air.FunctionID, typ air.TypeID) string {
	return fmt.Sprintf("ard_fn_adapter_%d_%d", fn, typ)
}

func closureAdapterName(fn air.FunctionID) string {
	return fmt.Sprintf("ard_closure_adapter_%d", fn)
}

func closureEnvName(fn air.Function) string {
	return fmt.Sprintf("ArdClosureEnv_%d_%s", fn.ID, sanitizeIdentifier(fn.Name))
}

func (fl *functionLowerer) nextTemp(prefix string) string {
	name := fmt.Sprintf("tmp_%s_%d", sanitizeIdentifier(prefix), fl.tempCounter)
	fl.tempCounter++
	return name
}

func localName(fn air.Function, id air.LocalID) string {
	name := "local"
	if int(id) >= 0 && int(id) < len(fn.Locals) && fn.Locals[id].Name != "" {
		name = fn.Locals[id].Name
	}
	return fmt.Sprintf("l%d_%s", id, sanitizeIdentifier(name))
}

func externBinding(ext air.Extern) string {
	if binding := ext.Bindings[backend.TargetZig]; binding != "" {
		return binding
	}
	if binding := ext.Bindings[backend.TargetGo]; binding != "" {
		return binding
	}
	for _, binding := range ext.Bindings {
		if binding != "" {
			return binding
		}
	}
	return ext.Name
}

func sanitizeIdentifier(name string) string {
	if name == "" {
		return "unnamed"
	}
	var b strings.Builder
	for i, r := range name {
		if (i == 0 && (unicode.IsLetter(r) || r == '_')) || (i > 0 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_')) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || out == "_" {
		return "unnamed"
	}
	return out
}

const runtimeSource = `const std = @import("std");

pub const Context = struct {
    allocator: std.mem.Allocator,
    io: std.Io,
};

pub const Stringable = union(enum) {
    str: []const u8,
    int: i64,
    float: f64,
    bool: bool,
};

pub fn Maybe(comptime T: type) type {
    return struct {
        some: ?T,
    };
}

pub fn Result(comptime Value: type, comptime Error: type) type {
    return struct {
        value: Value = undefined,
        err: Error = undefined,
        ok: bool = false,
    };
}

pub fn Fn0(comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self) !R {
            return self.call(self.ctx, self.env);
        }
    };
}

pub fn Fn1(comptime A0: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0) !R {
            return self.call(self.ctx, self.env, a0);
        }
    };
}

pub fn Fn2(comptime A0: type, comptime A1: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1) !R {
            return self.call(self.ctx, self.env, a0, a1);
        }
    };
}

pub fn Fn3(comptime A0: type, comptime A1: type, comptime A2: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1, A2) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1, a2: A2) !R {
            return self.call(self.ctx, self.env, a0, a1, a2);
        }
    };
}

pub fn Fn4(comptime A0: type, comptime A1: type, comptime A2: type, comptime A3: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1, A2, A3) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1, a2: A2, a3: A3) !R {
            return self.call(self.ctx, self.env, a0, a1, a2, a3);
        }
    };
}

pub fn Fn5(comptime A0: type, comptime A1: type, comptime A2: type, comptime A3: type, comptime A4: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1, A2, A3, A4) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1, a2: A2, a3: A3, a4: A4) !R {
            return self.call(self.ctx, self.env, a0, a1, a2, a3, a4);
        }
    };
}

pub fn Fn6(comptime A0: type, comptime A1: type, comptime A2: type, comptime A3: type, comptime A4: type, comptime A5: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1, A2, A3, A4, A5) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1, a2: A2, a3: A3, a4: A4, a5: A5) !R {
            return self.call(self.ctx, self.env, a0, a1, a2, a3, a4, a5);
        }
    };
}

pub fn Fn7(comptime A0: type, comptime A1: type, comptime A2: type, comptime A3: type, comptime A4: type, comptime A5: type, comptime A6: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1, A2, A3, A4, A5, A6) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1, a2: A2, a3: A3, a4: A4, a5: A5, a6: A6) !R {
            return self.call(self.ctx, self.env, a0, a1, a2, a3, a4, a5, a6);
        }
    };
}

pub fn Fn8(comptime A0: type, comptime A1: type, comptime A2: type, comptime A3: type, comptime A4: type, comptime A5: type, comptime A6: type, comptime A7: type, comptime R: type) type {
    return struct {
        ctx: *Context,
        env: ?*anyopaque,
        call: *const fn (*Context, ?*anyopaque, A0, A1, A2, A3, A4, A5, A6, A7) anyerror!R,

        const Self = @This();

        pub fn invoke(self: Self, a0: A0, a1: A1, a2: A2, a3: A3, a4: A4, a5: A5, a6: A6, a7: A7) !R {
            return self.call(self.ctx, self.env, a0, a1, a2, a3, a4, a5, a6, a7);
        }
    };
}

pub fn boxMaybe(comptime T: type, ctx: *Context, value: Maybe(T)) !Maybe(*T) {
    if (value.some) |unboxed| {
        const boxed = try ctx.allocator.create(T);
        boxed.* = unboxed;
        return .{ .some = boxed };
    }
    return .{ .some = null };
}

pub fn unboxMaybe(comptime T: type, value: Maybe(*T)) Maybe(T) {
    if (value.some) |boxed| {
        return .{ .some = boxed.* };
    }
    return .{ .some = null };
}

pub fn List(comptime T: type) type {
    return struct {
        allocator: std.mem.Allocator,
        items: []T,

        const Self = @This();

        pub fn init(allocator: std.mem.Allocator, initial: []const T) !Self {
            const items = try allocator.alloc(T, initial.len);
            @memcpy(items, initial);
            return .{ .allocator = allocator, .items = items };
        }

        pub fn size(self: Self) usize {
            return self.items.len;
        }

        pub fn at(self: Self, index: usize) T {
            return self.items[index];
        }

        pub fn push(self: *Self, value: T) !void {
            const next = try self.allocator.alloc(T, self.items.len + 1);
            @memcpy(next[0..self.items.len], self.items);
            next[self.items.len] = value;
            self.items = next;
        }
    };
}

pub fn Map(comptime K: type, comptime V: type) type {
    return struct {
        allocator: std.mem.Allocator,
        entries: []Entry,

        const Self = @This();

        pub const Entry = struct {
            key: K,
            value: V,
        };

        pub fn init(allocator: std.mem.Allocator, initial: []const Entry) !Self {
            const entries = try allocator.alloc(Entry, initial.len);
            @memcpy(entries, initial);
            return .{ .allocator = allocator, .entries = entries };
        }

        pub fn size(self: Self) usize {
            return self.entries.len;
        }

        pub fn has(self: Self, key: K) bool {
            return self.indexOf(key) != null;
        }

        pub fn get(self: Self, key: K) Maybe(V) {
            if (self.indexOf(key)) |index| {
                return .{ .some = self.entries[index].value };
            }
            return .{ .some = null };
        }

        pub fn set(self: *Self, key: K, value: V) !void {
            if (self.indexOf(key)) |index| {
                self.entries[index].value = value;
                return;
            }
            const next = try self.allocator.alloc(Entry, self.entries.len + 1);
            @memcpy(next[0..self.entries.len], self.entries);
            next[self.entries.len] = .{ .key = key, .value = value };
            self.entries = next;
        }

        pub fn drop(self: *Self, key: K) void {
            const index = self.indexOf(key) orelse return;
            var i = index;
            while (i + 1 < self.entries.len) : (i += 1) {
                self.entries[i] = self.entries[i + 1];
            }
            self.entries = self.entries[0 .. self.entries.len - 1];
        }

        pub fn keyAt(self: Self, index: usize) K {
            return self.entries[index].key;
        }

        pub fn valueAt(self: Self, index: usize) V {
            return self.entries[index].value;
        }

        fn indexOf(self: Self, key: K) ?usize {
            for (self.entries, 0..) |entry, index| {
                if (eql(K, entry.key, key)) return index;
            }
            return null;
        }
    };
}

fn eql(comptime T: type, left: T, right: T) bool {
    return switch (T) {
        []const u8 => std.mem.eql(u8, left, right),
        else => left == right,
    };
}

pub fn strAt(value: []const u8, index: i64) Maybe([]const u8) {
    if (index < 0) return .{ .some = null };
    var view = std.unicode.Utf8View.init(value) catch return .{ .some = null };
    var iter = view.iterator();
    var current: i64 = 0;
    while (iter.nextCodepointSlice()) |slice| {
        if (current == index) return .{ .some = slice };
        current += 1;
    }
    return .{ .some = null };
}

pub fn strAtByte(value: []const u8, index: i64) []const u8 {
    const byte_index: usize = @intCast(index);
    return value[byte_index .. byte_index + 1];
}

pub fn strReplace(ctx: *Context, value: []const u8, old: []const u8, new: []const u8) ![]const u8 {
    const index = std.mem.indexOf(u8, value, old) orelse return value;
    const size = value.len - old.len + new.len;
    const out = try ctx.allocator.alloc(u8, size);
    @memcpy(out[0..index], value[0..index]);
    @memcpy(out[index..][0..new.len], new);
    @memcpy(out[index + new.len ..], value[index + old.len ..]);
    return out;
}

pub fn strReplaceAll(ctx: *Context, value: []const u8, old: []const u8, new: []const u8) ![]const u8 {
    return try std.mem.replaceOwned(u8, ctx.allocator, value, old, new);
}

pub fn strSplit(ctx: *Context, value: []const u8, delimiter: []const u8) !List([]const u8) {
    if (delimiter.len == 0) return try strSplitCodepoints(ctx, value);

    const count = std.mem.count(u8, value, delimiter) + 1;
    var parts = try ctx.allocator.alloc([]const u8, count);
    var iter = std.mem.splitSequence(u8, value, delimiter);
    var index: usize = 0;
    while (iter.next()) |part| {
        parts[index] = part;
        index += 1;
    }
    return try List([]const u8).init(ctx.allocator, parts);
}

fn strSplitCodepoints(ctx: *Context, value: []const u8) !List([]const u8) {
    const count = std.unicode.utf8CountCodepoints(value) catch value.len;
    var parts = try ctx.allocator.alloc([]const u8, count);
    var view = std.unicode.Utf8View.init(value) catch {
        for (value, 0..) |_, index| {
            parts[index] = value[index .. index + 1];
        }
        return try List([]const u8).init(ctx.allocator, parts);
    };
    var iter = view.iterator();
    var index: usize = 0;
    while (iter.nextCodepointSlice()) |slice| {
        parts[index] = slice;
        index += 1;
    }
    return try List([]const u8).init(ctx.allocator, parts);
}

pub fn print(ctx: *Context, value: []const u8) !void {
    var stdout_buffer: [1024]u8 = undefined;
    var stdout_writer = std.Io.File.stdout().writer(ctx.io, &stdout_buffer);
    const stdout = &stdout_writer.interface;
    try stdout.print("{s}\n", .{value});
    try stdout.flush();
}

pub fn concat(ctx: *Context, left: []const u8, right: []const u8) ![]const u8 {
    return try std.fmt.allocPrint(ctx.allocator, "{s}{s}", .{ left, right });
}

pub fn toStr(ctx: *Context, value: anytype) ![]const u8 {
    return switch (@TypeOf(value)) {
        []const u8 => value,
        f64 => try std.fmt.allocPrint(ctx.allocator, "{d:.2}", .{value}),
        else => try std.fmt.allocPrint(ctx.allocator, "{}", .{value}),
    };
}

pub fn stringableToStr(ctx: *Context, value: Stringable) ![]const u8 {
    return switch (value) {
        .str => |v| v,
        .int => |v| try toStr(ctx, v),
        .float => |v| try toStr(ctx, v),
        .bool => |v| try toStr(ctx, v),
    };
}

pub fn intFromStr(value: []const u8) Maybe(i64) {
    const parsed = std.fmt.parseInt(i64, value, 10) catch return .{ .some = null };
    return .{ .some = parsed };
}

pub fn floatFromStr(value: []const u8) Maybe(f64) {
    const parsed = std.fmt.parseFloat(f64, value) catch return .{ .some = null };
    return .{ .some = parsed };
}
`
