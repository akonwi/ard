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
	l := &lowerer{program: program}
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
	program *air.Program
}

func (l *lowerer) lowerProgram() (string, error) {
	var b strings.Builder
	b.WriteString("const std = @import(\"std\");\n")
	b.WriteString("const ard = @import(\"ard_runtime.zig\");\n\n")
	for _, fn := range l.program.Functions {
		if fn.IsTest {
			continue
		}
		if err := l.lowerFunction(&b, fn); err != nil {
			return "", err
		}
		b.WriteString("\n")
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

func (l *lowerer) lowerFunction(b *strings.Builder, fn air.Function) error {
	fmt.Fprintf(b, "fn %s(ctx: *ard.Context", functionName(fn))
	for i, param := range fn.Signature.Params {
		paramType, err := l.typeName(param.Type)
		if err != nil {
			return err
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
	case air.TypeTraitObject:
		if info.Trait >= 0 && int(info.Trait) < len(l.program.Traits) && l.program.Traits[info.Trait].Name == "ToString" {
			return "ard.Stringable", nil
		}
		return "", fmt.Errorf("unsupported Zig trait object type %s", info.Name)
	default:
		return "", fmt.Errorf("unsupported Zig type %s", info.Name)
	}
}

type functionLowerer struct {
	l      *lowerer
	fn     air.Function
	indent string
}

func (fl *functionLowerer) lowerBlock(b *strings.Builder, block air.Block, returnType air.TypeID) error {
	for _, stmt := range block.Stmts {
		if err := fl.lowerStmt(b, stmt); err != nil {
			return err
		}
	}
	if block.Result != nil {
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
		fmt.Fprintf(b, "%s%s = %s;\n", fl.indent, localName(fl.fn, stmt.Local), value)
	case air.StmtExpr:
		if stmt.Expr == nil {
			return fmt.Errorf("expression statement has no expression")
		}
		expr, err := fl.lowerExpr(*stmt.Expr)
		if err != nil {
			return err
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
		return localName(fl.fn, expr.Local), nil
	case air.ExprCall:
		return fl.lowerCall(expr)
	case air.ExprCallExtern:
		return fl.lowerExternCall(expr)
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

func (fl *functionLowerer) lowerCall(expr air.Expr) (string, error) {
	if int(expr.Function) < 0 || int(expr.Function) >= len(fl.l.program.Functions) {
		return "", fmt.Errorf("function %d out of range", expr.Function)
	}
	args := []string{"ctx"}
	for _, arg := range expr.Args {
		lowered, err := fl.lowerExpr(arg)
		if err != nil {
			return "", err
		}
		args = append(args, lowered)
	}
	return fmt.Sprintf("try %s(%s)", functionName(fl.l.program.Functions[expr.Function]), strings.Join(args, ", ")), nil
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
	case air.ExprCall, air.ExprCallExtern, air.ExprCallTrait, air.ExprStrConcat, air.ExprToStr:
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
`
