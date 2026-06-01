package formatter

import (
	"strings"

	"github.com/akonwi/ard/parse"
)

func removeUnusedImports(program *parse.Program) {
	if program == nil || len(program.Imports) == 0 {
		return
	}
	used := map[string]bool{}
	for _, stmt := range program.Statements {
		collectImportUsesInStatement(stmt, used)
	}
	imports := program.Imports[:0]
	for _, imp := range program.Imports {
		name := imp.Name
		if name == "" {
			name = defaultImportName(imp.Path)
		}
		if used[name] {
			imports = append(imports, imp)
		}
	}
	program.Imports = imports
}

func collectImportUsesInType(t parse.DeclaredType, used map[string]bool) {
	switch v := t.(type) {
	case parse.CustomType:
		if v.Type.Target != nil {
			if name := simpleImportUseName(v.Type.Target); name != "" {
				used[name] = true
			}
		}
		for _, arg := range v.TypeArgs {
			collectImportUsesInType(arg, used)
		}
	case parse.List:
		collectImportUsesInType(v.Element, used)
	case parse.Map:
		collectImportUsesInType(v.Key, used)
		collectImportUsesInType(v.Value, used)
	case parse.ResultType:
		collectImportUsesInType(v.Val, used)
		collectImportUsesInType(v.Err, used)
	case parse.FunctionType:
		for _, p := range v.Params {
			collectImportUsesInType(p, used)
		}
		collectImportUsesInType(v.Return, used)
	}
}

func collectImportUsesInStatement(stmt parse.Statement, used map[string]bool) {
	switch s := stmt.(type) {
	case *parse.VariableDeclaration:
		if s.Type != nil {
			collectImportUsesInType(s.Type, used)
		}
		collectImportUsesInExpression(s.Value, used)
	case *parse.VariableAssignment:
		collectImportUsesInExpression(s.Target, used)
		collectImportUsesInExpression(s.Value, used)
	case *parse.FunctionDeclaration:
		for _, p := range s.Parameters {
			if p.Type != nil {
				collectImportUsesInType(p.Type, used)
			}
		}
		if s.ReturnType != nil {
			collectImportUsesInType(s.ReturnType, used)
		}
		for _, body := range s.Body {
			collectImportUsesInStatement(body, used)
		}
	case *parse.StaticFunctionDeclaration:
		collectImportUsesInExpression(&s.Path, used)
		collectImportUsesInStatement(&s.FunctionDeclaration, used)
	case *parse.ExternalFunction:
		for _, p := range s.Parameters {
			if p.Type != nil {
				collectImportUsesInType(p.Type, used)
			}
		}
		if s.ReturnType != nil {
			collectImportUsesInType(s.ReturnType, used)
		}
	case *parse.TypeDeclaration:
		for _, t := range s.Type {
			collectImportUsesInType(t, used)
		}
	case *parse.StructDefinition:
		for _, field := range s.Fields {
			collectImportUsesInType(field.Type, used)
		}
	case *parse.ImplBlock:
		collectImportUsesInExpression(s.Target, used)
		for i := range s.Methods {
			collectImportUsesInStatement(&s.Methods[i], used)
		}
	case *parse.TraitDefinition:
		for i := range s.Methods {
			collectImportUsesInStatement(&s.Methods[i], used)
		}
	case *parse.TraitImplementation:
		collectImportUsesInExpression(s.ForType, used)
		for i := range s.Methods {
			collectImportUsesInStatement(&s.Methods[i], used)
		}
	case *parse.StructInstance:
		collectImportUsesInExpression(s, used)
	case *parse.EnumDefinition:
		for _, variant := range s.Variants {
			collectImportUsesInExpression(variant.Value, used)
		}
	case *parse.WhileLoop:
		collectImportUsesInExpression(s.Condition, used)
		for _, body := range s.Body {
			collectImportUsesInStatement(body, used)
		}
	case *parse.RangeLoop:
		collectImportUsesInExpression(s.Start, used)
		collectImportUsesInExpression(s.End, used)
		for _, body := range s.Body {
			collectImportUsesInStatement(body, used)
		}
	case *parse.ForInLoop:
		collectImportUsesInExpression(s.Iterable, used)
		for _, body := range s.Body {
			collectImportUsesInStatement(body, used)
		}
	case *parse.ForLoop:
		collectImportUsesInStatement(s.Init, used)
		collectImportUsesInExpression(s.Condition, used)
		collectImportUsesInStatement(s.Incrementer, used)
		for _, body := range s.Body {
			collectImportUsesInStatement(body, used)
		}
	case *parse.IfStatement:
		collectImportUsesInExpression(s.Condition, used)
		for _, body := range s.Body {
			collectImportUsesInStatement(body, used)
		}
		collectImportUsesInStatement(s.Else, used)
	case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.Try, *parse.BlockExpression:
		collectImportUsesInExpression(s, used)
	}
}

func collectImportUsesInExpression(expr parse.Expression, used map[string]bool) {
	switch e := expr.(type) {
	case nil:
		return
	case *parse.StaticProperty:
		if name := simpleImportUseName(e.Target); name != "" {
			used[name] = true
		}
		collectImportUsesInExpression(e.Target, used)
		collectImportUsesInExpression(e.Property, used)
	case *parse.StaticFunction:
		if name := simpleImportUseName(e.Target); name != "" {
			used[name] = true
		}
		collectImportUsesInExpression(e.Target, used)
		for _, arg := range e.Function.Args {
			collectImportUsesInExpression(arg.Value, used)
		}
	case *parse.FunctionCall:
		for _, arg := range e.Args {
			collectImportUsesInExpression(arg.Value, used)
		}
	case *parse.InstanceProperty:
		collectImportUsesInExpression(e.Target, used)
		collectImportUsesInExpression(e.Property, used)
	case *parse.InstanceMethod:
		collectImportUsesInExpression(e.Target, used)
		for _, arg := range e.Method.Args {
			collectImportUsesInExpression(arg.Value, used)
		}
	case *parse.StructInstance:
		if strings.Contains(e.Name.Name, "::") {
			used[strings.SplitN(e.Name.Name, "::", 2)[0]] = true
		}
		for _, prop := range e.Properties {
			collectImportUsesInExpression(prop.Value, used)
		}
	case *parse.AnonymousFunction:
		for _, p := range e.Parameters {
			if p.Type != nil {
				collectImportUsesInType(p.Type, used)
			}
		}
		if e.ReturnType != nil {
			collectImportUsesInType(e.ReturnType, used)
		}
		for _, body := range e.Body {
			collectImportUsesInStatement(body, used)
		}
	case *parse.BinaryExpression:
		collectImportUsesInExpression(e.Left, used)
		collectImportUsesInExpression(e.Right, used)
	case *parse.UnaryExpression:
		collectImportUsesInExpression(e.Operand, used)
	case *parse.ChainedComparison:
		for _, operand := range e.Operands {
			collectImportUsesInExpression(operand, used)
		}
	case *parse.RangeExpression:
		collectImportUsesInExpression(e.Start, used)
		collectImportUsesInExpression(e.End, used)
	case *parse.InterpolatedStr:
		for _, chunk := range e.Chunks {
			collectImportUsesInExpression(chunk, used)
		}
	case *parse.ListLiteral:
		for _, item := range e.Items {
			collectImportUsesInExpression(item, used)
		}
	case *parse.MapLiteral:
		for _, entry := range e.Entries {
			collectImportUsesInExpression(entry.Key, used)
			collectImportUsesInExpression(entry.Value, used)
		}
	case *parse.MatchExpression:
		collectImportUsesInExpression(e.Subject, used)
		for _, c := range e.Cases {
			collectImportUsesInExpression(c.Pattern, used)
			for _, body := range c.Body {
				collectImportUsesInStatement(body, used)
			}
		}
	case *parse.ConditionalMatchExpression:
		for _, c := range e.Cases {
			collectImportUsesInExpression(c.Condition, used)
			for _, body := range c.Body {
				collectImportUsesInStatement(body, used)
			}
		}
	case *parse.Try:
		collectImportUsesInExpression(e.Expression, used)
		for _, body := range e.CatchBlock {
			collectImportUsesInStatement(body, used)
		}
	case *parse.BlockExpression:
		for _, body := range e.Statements {
			collectImportUsesInStatement(body, used)
		}
	case *parse.IfStatement:
		collectImportUsesInStatement(e, used)
	}
}

func simpleImportUseName(expr parse.Expression) string {
	id, ok := expr.(*parse.Identifier)
	if !ok {
		return ""
	}
	return id.Name
}
