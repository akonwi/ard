package formatter

import (
	"fmt"
	"sort"
	"strings"

	"github.com/akonwi/ard/compiler/parse"
)

// DeclarationSignature renders the canonical form of the part of a top-level
// declaration that its platform-specific siblings must share (ADR 0082):
//
//   - a function's header, without its body;
//   - a `let` or `mut` binding's name and declared type, without its value;
//   - a type declaration's visibility and name, without the aliased type;
//   - an impl block's header and its method headers, in name order.
//
// It reports false for structs, enums, and traits, which cannot have
// platform-specific siblings, and for statements that are not declarations.
func DeclarationSignature(statement parse.Statement) (string, bool) {
	p := newPrinter(1 << 30)
	switch node := statement.(type) {
	case *parse.FunctionDeclaration:
		return p.functionSignature(*node), true
	case *parse.StaticFunctionDeclaration:
		copy := *node
		copy.Comments = nil
		return p.staticFunctionHeader(&copy), true
	case *parse.VariableDeclaration:
		binding := "let"
		if node.Mutable {
			binding = "mut"
		}
		signature := binding + " " + node.Name
		if node.Type != nil {
			signature += ": " + p.renderType(node.Type)
		}
		return signature, true
	case *parse.TypeDeclaration:
		prefix := ""
		if node.Private {
			prefix = "private "
		}
		return prefix + "type " + node.Name.Name, true
	case *parse.ImplBlock:
		return p.methodSetSignature("impl "+node.Target.Name, node.Methods), true
	case *parse.TraitImplementation:
		return p.methodSetSignature(fmt.Sprintf("impl %s for %s", p.renderExpression(node.Trait, 0), node.ForType.Name), node.Methods), true
	}
	return "", false
}

func (p printer) functionSignature(node parse.FunctionDeclaration) string {
	node.Comments = nil
	return p.printDocAtColumn(p.renderFunctionDeclarationDoc(&node, true), 0)
}

func (p printer) methodSetSignature(header string, methods []parse.FunctionDeclaration) string {
	signatures := make([]string, 0, len(methods))
	for _, method := range methods {
		signatures = append(signatures, p.functionSignature(method))
	}
	sort.Strings(signatures)
	return strings.Join(append([]string{header}, signatures...), "\n")
}
