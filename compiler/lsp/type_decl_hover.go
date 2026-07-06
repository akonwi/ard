package lsp

import (
	"strings"

	"github.com/akonwi/ard/parse"
)

// renderTypeDeclHover renders a nominal type declaration's shape from the
// parse tree. Declaration-site hover is same-file by construction (the
// type: def record only exists in the declaring file), so the syntactic
// route is authoritative: it preserves author field order, sees private
// types, distinguishes aliases from their targets, and renders generic
// parameters verbatim.
func renderTypeDeclHover(program *parse.Program, name string) string {
	if program == nil || name == "" {
		return ""
	}
	for _, stmt := range program.Statements {
		switch decl := stmt.(type) {
		case *parse.StructDefinition:
			if decl.Name.Name == name {
				return renderStructDecl(decl)
			}
		case *parse.EnumDefinition:
			if decl.Name == name {
				return renderEnumDecl(decl)
			}
		case *parse.TraitDefinition:
			if decl.Name.Name == name {
				return renderTraitDecl(decl)
			}
		case *parse.TypeDeclaration:
			if decl.Name.Name == name {
				return renderTypeAliasDecl(decl)
			}
		}
	}
	return ""
}

func renderStructDecl(decl *parse.StructDefinition) string {
	var b strings.Builder
	b.WriteString("struct ")
	b.WriteString(decl.Name.Name)
	if len(decl.TypeParams) > 0 {
		b.WriteString("<$")
		b.WriteString(strings.Join(decl.TypeParams, ", $"))
		b.WriteString(">")
	}
	if len(decl.Fields) == 0 {
		b.WriteString(" {}")
		return b.String()
	}
	b.WriteString(" {\n")
	for _, field := range decl.Fields {
		b.WriteString("  ")
		b.WriteString(field.Name.Name)
		b.WriteString(": ")
		b.WriteString(typeDeclString(field.Type))
		b.WriteString(",\n")
	}
	b.WriteString("}")
	return b.String()
}

func renderEnumDecl(decl *parse.EnumDefinition) string {
	var b strings.Builder
	b.WriteString("enum ")
	b.WriteString(decl.Name)
	if len(decl.Variants) == 0 {
		b.WriteString(" {}")
		return b.String()
	}
	b.WriteString(" {\n")
	for _, variant := range decl.Variants {
		b.WriteString("  ")
		b.WriteString(variant.Name)
		b.WriteString(",\n")
	}
	b.WriteString("}")
	return b.String()
}

func renderTraitDecl(decl *parse.TraitDefinition) string {
	var b strings.Builder
	b.WriteString("trait ")
	b.WriteString(decl.Name.Name)
	if len(decl.Methods) == 0 {
		b.WriteString(" {}")
		return b.String()
	}
	b.WriteString(" {\n")
	for i := range decl.Methods {
		method := &decl.Methods[i]
		b.WriteString("  fn ")
		b.WriteString(method.Name)
		b.WriteString("(")
		params := make([]string, 0, len(method.Parameters))
		for _, param := range method.Parameters {
			params = append(params, param.Name+": "+typeDeclString(param.Type))
		}
		b.WriteString(strings.Join(params, ", "))
		b.WriteString(")")
		if method.ReturnType != nil {
			b.WriteString(" ")
			b.WriteString(typeDeclString(method.ReturnType))
		}
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String()
}

func renderTypeAliasDecl(decl *parse.TypeDeclaration) string {
	parts := make([]string, 0, len(decl.Type))
	for _, t := range decl.Type {
		parts = append(parts, typeDeclString(t))
	}
	return "type " + decl.Name.Name + " = " + strings.Join(parts, " | ")
}

// typeDeclString renders a parse.DeclaredType as Ard surface syntax.
func typeDeclString(t parse.DeclaredType) string {
	if t == nil {
		return "?"
	}
	var s string
	switch tt := t.(type) {
	case *parse.MutableType:
		s = "mut " + typeDeclString(tt.Inner)
	case parse.MutableType:
		s = "mut " + typeDeclString(tt.Inner)
	case *parse.List:
		s = "[" + typeDeclString(tt.Element) + "]"
	case *parse.Map:
		s = "[" + typeDeclString(tt.Key) + ": " + typeDeclString(tt.Value) + "]"
	case *parse.FunctionType:
		params := make([]string, 0, len(tt.Params))
		for i, p := range tt.Params {
			text := typeDeclString(p)
			if i < len(tt.ParamMutability) && tt.ParamMutability[i] && !strings.HasPrefix(text, "mut ") {
				text = "mut " + text
			}
			params = append(params, text)
		}
		s = "fn(" + strings.Join(params, ", ") + ")"
		if tt.Return != nil {
			s += " " + typeDeclString(tt.Return)
		}
	case *parse.ResultType:
		s = typeDeclString(tt.Val) + "!" + typeDeclString(tt.Err)
	case *parse.CustomType:
		s = t.GetName()
		if len(tt.TypeArgs) > 0 {
			args := make([]string, len(tt.TypeArgs))
			for i, arg := range tt.TypeArgs {
				args[i] = typeDeclString(arg)
			}
			s += "<" + strings.Join(args, ", ") + ">"
		}
	default:
		s = t.GetName()
		switch s {
		case "String":
			s = "Str"
		case "Boolean":
			s = "Bool"
		}
	}
	if t.IsNullable() {
		s += "?"
	}
	return s
}
