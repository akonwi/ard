package transpile

import (
	"fmt"
	gofmt "go/format"
	"os"
	"os/exec"
	"path/filepath"
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
	functionNames map[string]string
	builder       strings.Builder
	indent        int
}

func EmitEntrypoint(module checker.Module) ([]byte, error) {
	if module == nil || module.Program() == nil {
		return nil, fmt.Errorf("module has no program")
	}

	e := &emitter{
		module:        module,
		packageName:   "main",
		functionNames: make(map[string]string),
	}
	e.indexFunctions()
	e.line("package main")
	e.line("")

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

	e.line("func main() {")
	e.indent++
	if err := e.emitStatements(topLevelExecutableStatements(module.Program().Statements), nil); err != nil {
		return nil, err
	}
	e.indent--
	e.line("}")

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

	source, err := EmitEntrypoint(module)
	if err != nil {
		return "", err
	}

	generatedDir := filepath.Join(project.RootPath, "generated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "go.mod"), []byte(fmt.Sprintf("module %s\n\ngo %s\n", project.ProjectName, generatedGoVersion)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "main.go"), source, 0o644); err != nil {
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

	source, err := EmitEntrypoint(module)
	if err != nil {
		return err
	}

	generatedDir := filepath.Join(project.RootPath, "generated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "go.mod"), []byte(fmt.Sprintf("module %s\n\ngo %s\n", project.ProjectName, generatedGoVersion)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(generatedDir, "main.go"), source, 0o644); err != nil {
		return err
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = generatedDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
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
		filtered = append(filtered, stmt)
	}
	return filtered
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
	for i, stmt := range stmts {
		isLastExpr := i == len(stmts)-1 && stmt.Expr != nil
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
		target, ok := s.Target.(*checker.Identifier)
		if !ok {
			return fmt.Errorf("unsupported reassignment target: %T", s.Target)
		}
		value, err := e.emitExpr(s.Value)
		if err != nil {
			return err
		}
		e.line(fmt.Sprintf("%s = %s", goName(target.Name, false), value))
		return nil
	default:
		return fmt.Errorf("unsupported statement: %T", stmt)
	}
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
		return usesNameInExpr(s.Value, name)
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
	case *checker.ModuleFunctionCall:
		for _, arg := range v.Call.Args {
			if usesNameInExpr(arg, name) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (e *emitter) emitExpressionStatement(expr checker.Expression, returnType checker.Type, isLast bool) error {
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
	default:
		return "", fmt.Errorf("unsupported expression: %T", expr)
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
